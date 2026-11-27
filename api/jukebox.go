package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"waystone-web/db"
	"waystone-web/models"
)

// The jukebox: a game's DM broadcasts audio over a WebSocket and the game's players and followers
// listen over theirs. Nothing is stored; a room lives in memory only while someone is connected.
// Design: docs/design/jukebox.md.

type jbMsg struct {
	typ       websocket.MessageType
	data      []byte
	closeCode websocket.StatusCode // non-zero: close the socket after the earlier messages
	reason    string
}

type jukeboxConn struct {
	campaignID string
	userID     string
	user       map[string]interface{}
	dm         bool
	ws         *websocket.Conn
	out        chan jbMsg
	cancel     context.CancelFunc
}

type jukeboxRoom struct {
	broadcaster *jukeboxConn
	init        []byte
	paused      bool
	listeners   map[*jukeboxConn]bool
}

var jukebox = struct {
	sync.Mutex
	rooms map[string]*jukeboxRoom
}{rooms: map[string]*jukeboxRoom{}}

func (c *jukeboxConn) send(m jbMsg) bool {
	select {
	case c.out <- m:
		return true
	default:
		return false
	}
}

func jbText(v interface{}) jbMsg {
	data, _ := json.Marshal(v)
	return jbMsg{typ: websocket.MessageText, data: data}
}

// jukeboxRole says what a user may do with a game's jukebox: "dm", "listener" or "" (nothing).
func jukeboxRole(c *models.Campaign, userID string, following bool) string {
	if c == nil || c.Status != models.Ongoing {
		return ""
	}
	if c.DM == userID {
		return "dm"
	}
	if isCampaignPlayer(c, userID) || following {
		return "listener"
	}
	return ""
}

// jukeboxGameState is the game payload's jukebox object.
func jukeboxGameState(c *models.Campaign, userID string, following bool) map[string]bool {
	state := map[string]bool{"live": false, "paused": false}
	if jukeboxRole(c, userID, following) == "" {
		return state
	}
	jukebox.Lock()
	defer jukebox.Unlock()
	if room := jukebox.rooms[c.ID]; room != nil && room.broadcaster != nil && room.init != nil {
		state["live"], state["paused"] = true, room.paused
	}
	return state
}

// --- locked helpers (the caller holds jukebox) ---

func jbRoomLocked(id string) *jukeboxRoom {
	room := jukebox.rooms[id]
	if room == nil {
		room = &jukeboxRoom{listeners: map[*jukeboxConn]bool{}}
		jukebox.rooms[id] = room
	}
	return room
}

func jbGCLocked(id string) {
	if room := jukebox.rooms[id]; room != nil && room.broadcaster == nil && len(room.listeners) == 0 {
		delete(jukebox.rooms, id)
	}
}

func jbStateLocked(room *jukeboxRoom) jbMsg {
	state := "live"
	switch {
	case room.broadcaster == nil || room.init == nil:
		state = "off"
	case room.paused:
		state = "paused"
	}
	return jbText(map[string]string{"type": "state", "state": state})
}

// jbToListenersLocked queues msgs for every listener; listeners whose queue is full are returned to be dropped.
func jbToListenersLocked(room *jukeboxRoom, msgs ...jbMsg) (slow []*jukeboxConn) {
	for c := range room.listeners {
		for _, m := range msgs {
			if !c.send(m) {
				slow = append(slow, c)
				break
			}
		}
	}
	return slow
}

// jbDropSlow removes listeners that couldn't keep up and closes them; their pop-up reconnects and gets a fresh init.
func jbDropSlow(campaignID string, slow []*jukeboxConn) {
	if len(slow) == 0 {
		return
	}
	jukebox.Lock()
	if room := jukebox.rooms[campaignID]; room != nil {
		for _, c := range slow {
			delete(room.listeners, c)
		}
		jbGCLocked(campaignID)
	}
	jukebox.Unlock()
	for _, c := range slow {
		c.ws.CloseNow()
		c.cancel()
	}
	jbBroadcastList(campaignID)
}

// jbBroadcastList sends the who's-listening list to the listeners and the broadcaster.
func jbBroadcastList(campaignID string) {
	jukebox.Lock()
	room := jukebox.rooms[campaignID]
	if room == nil {
		jukebox.Unlock()
		return
	}
	var dm interface{}
	if room.broadcaster != nil && room.init != nil {
		dm = room.broadcaster.user
	}
	seen := map[string]bool{}
	list := []map[string]interface{}{}
	for c := range room.listeners {
		if !seen[c.userID] {
			seen[c.userID] = true
			list = append(list, c.user)
		}
	}
	sort.Slice(list, func(a, b int) bool {
		na, _ := list[a]["display_name"].(string)
		nb, _ := list[b]["display_name"].(string)
		if na != nb {
			return na < nb
		}
		return list[a]["id"].(string) < list[b]["id"].(string)
	})
	msg := jbText(map[string]interface{}{"type": "listeners", "dm": dm, "listeners": list})
	slow := jbToListenersLocked(room, msg)
	if room.broadcaster != nil {
		room.broadcaster.send(msg)
	}
	jukebox.Unlock()
	jbDropSlow(campaignID, slow)
}

// jbDetach removes a connection from its room; if it was the broadcaster the room goes off air.
func jbDetach(c *jukeboxConn) {
	jukebox.Lock()
	room := jukebox.rooms[c.campaignID]
	if room == nil {
		jukebox.Unlock()
		return
	}
	var slow []*jukeboxConn
	if room.broadcaster == c {
		room.broadcaster, room.init, room.paused = nil, nil, false
		slow = jbToListenersLocked(room, jbStateLocked(room))
	}
	delete(room.listeners, c)
	jbGCLocked(c.campaignID)
	jukebox.Unlock()
	jbDropSlow(c.campaignID, slow)
	jbBroadcastList(c.campaignID)
}

// jbEnd tells a connection why it is over and closes it.
func jbEnd(c *jukeboxConn, reason string) {
	jbDetach(c)
	ok := c.send(jbText(map[string]string{"type": "ended", "reason": reason})) &&
		c.send(jbMsg{closeCode: websocket.StatusNormalClosure, reason: reason})
	if !ok {
		c.ws.CloseNow()
		c.cancel()
	}
}

// startWriter runs the one goroutine that writes to the socket (keeps frames in order) and the pinger.
func (c *jukeboxConn) startWriter(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case m := <-c.out:
				if m.closeCode != 0 {
					c.ws.Close(m.closeCode, m.reason)
					c.cancel()
					return
				}
				wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := c.ws.Write(wctx, m.typ, m.data)
				cancel()
				if err != nil {
					c.ws.CloseNow()
					c.cancel()
					return
				}
			}
		}
	}()
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := c.ws.Ping(pctx)
				cancel()
				if err != nil {
					c.ws.CloseNow()
					c.cancel()
					return
				}
			}
		}
	}()
}

// HandleJukeboxWS handles GET /api/campaigns/{id}/jukebox/ws?role=dm|listen.
func HandleJukeboxWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	campaign, userID, ok := loadCampaignAndUser(w, r, parts[0])
	if !ok {
		return
	}
	want := r.URL.Query().Get("role")
	if want != "dm" && want != "listen" {
		writeJSONError(w, http.StatusBadRequest, "role must be dm or listen")
		return
	}
	user, err := db.GetUserByID(userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve user")
		return
	}
	followed, err := db.GetFollowedCampaignIDs(userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve follows")
		return
	}
	role := jukeboxRole(campaign, userID, followed[campaign.ID])
	if user == nil || role == "" || (want == "dm") != (role == "dm") {
		writeJSONError(w, http.StatusNotFound, "game not found")
		return
	}

	// The server-wide deadlines would cut a long stream; clear them before the upgrade.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &jukeboxConn{
		campaignID: campaign.ID, userID: userID, user: userDisplayObject(user), dm: role == "dm",
		ws: ws, out: make(chan jbMsg, 64), cancel: cancel,
	}
	c.startWriter(ctx)
	defer ws.CloseNow()

	if c.dm {
		runJukeboxDM(ctx, c)
	} else {
		runJukeboxListener(ctx, c)
	}
	jbDetach(c)
}

func runJukeboxListener(ctx context.Context, c *jukeboxConn) {
	jukebox.Lock()
	room := jbRoomLocked(c.campaignID)
	room.listeners[c] = true
	// Queued under the lock so every later broadcast arrives after these.
	c.send(jbStateLocked(room))
	if room.init != nil {
		c.send(jbText(map[string]string{"type": "stream"}))
		c.send(jbMsg{typ: websocket.MessageBinary, data: room.init})
	}
	jukebox.Unlock()
	jbBroadcastList(c.campaignID)
	<-c.ws.CloseRead(ctx).Done()
}

func runJukeboxDM(ctx context.Context, c *jukeboxConn) {
	c.ws.SetReadLimit(256 << 10)
	jukebox.Lock()
	room := jbRoomLocked(c.campaignID)
	old := room.broadcaster
	room.broadcaster, room.init, room.paused = c, nil, false
	slow := jbToListenersLocked(room, jbStateLocked(room))
	jukebox.Unlock()
	jbDropSlow(c.campaignID, slow)
	if old != nil {
		jbEnd(old, "taken_over")
	}
	jbBroadcastList(c.campaignID)

	var parser webmReader
	for {
		typ, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		if typ == websocket.MessageText {
			var msg struct {
				Muted *bool `json:"muted"`
			}
			if json.Unmarshal(data, &msg) != nil || msg.Muted == nil {
				continue
			}
			jukebox.Lock()
			room := jukebox.rooms[c.campaignID]
			if room == nil || room.broadcaster != c {
				jukebox.Unlock()
				return
			}
			room.paused = *msg.Muted
			slow := jbToListenersLocked(room, jbStateLocked(room))
			jukebox.Unlock()
			jbDropSlow(c.campaignID, slow)
			continue
		}
		init, frames, err := parser.Feed(data)
		if err != nil {
			c.ws.Close(websocket.StatusUnsupportedData, "bad stream")
			return
		}
		jukebox.Lock()
		room := jukebox.rooms[c.campaignID]
		if room == nil || room.broadcaster != c {
			jukebox.Unlock()
			return
		}
		var slow []*jukeboxConn
		if init != nil {
			room.init = init
			slow = jbToListenersLocked(room,
				jbText(map[string]string{"type": "stream"}),
				jbMsg{typ: websocket.MessageBinary, data: init},
				jbStateLocked(room))
		}
		for _, f := range frames {
			slow = append(slow, jbToListenersLocked(room, jbMsg{typ: websocket.MessageBinary, data: f})...)
		}
		jukebox.Unlock()
		jbDropSlow(c.campaignID, slow)
		if init != nil {
			jbBroadcastList(c.campaignID)
		}
	}
}

// HandleJukeboxLive handles GET /api/jukebox/live: the live games the user can broadcast to or listen to.
func HandleJukeboxLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID, _ := r.Context().Value("user_id").(string)
	type liveRoom struct {
		id        string
		paused    bool
		listeners int
	}
	var rooms []liveRoom
	jukebox.Lock()
	for id, room := range jukebox.rooms {
		if room.broadcaster == nil || room.init == nil {
			continue
		}
		users := map[string]bool{}
		for c := range room.listeners {
			users[c.userID] = true
		}
		rooms = append(rooms, liveRoom{id, room.paused, len(users)})
	}
	jukebox.Unlock()

	live := []map[string]interface{}{}
	if len(rooms) > 0 {
		followed, err := db.GetFollowedCampaignIDs(userID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to retrieve follows")
			return
		}
		for _, lr := range rooms {
			campaign, err := db.GetCampaignByID(lr.id)
			if err != nil || campaign == nil {
				continue
			}
			role := jukeboxRole(campaign, userID, followed[lr.id])
			if role == "" {
				continue
			}
			live = append(live, map[string]interface{}{
				"campaign_id": lr.id, "title": campaign.Title, "role": role,
				"paused": lr.paused, "listeners": lr.listeners,
			})
		}
		sort.Slice(live, func(a, b int) bool { return live[a]["title"].(string) < live[b]["title"].(string) })
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"live": live})
}

// jukeboxConns snapshots the connections of a room (campaignID) or of every room (empty id).
func jukeboxConns(campaignID string, userID string) []*jukeboxConn {
	var conns []*jukeboxConn
	jukebox.Lock()
	defer jukebox.Unlock()
	for id, room := range jukebox.rooms {
		if campaignID != "" && id != campaignID {
			continue
		}
		if room.broadcaster != nil && (userID == "" || room.broadcaster.userID == userID) {
			conns = append(conns, room.broadcaster)
		}
		for c := range room.listeners {
			if userID == "" || c.userID == userID {
				conns = append(conns, c)
			}
		}
	}
	return conns
}

// jukeboxRecheck re-checks everyone connected to a game after something changed (a player left or was
// removed, someone unfollowed, the status changed, the game was deleted). Cheap when nobody is connected.
func jukeboxRecheck(campaignID string) {
	conns := jukeboxConns(campaignID, "")
	if len(conns) == 0 {
		return
	}
	campaign, err := db.GetCampaignByID(campaignID)
	if err != nil {
		return
	}
	reason := ""
	switch {
	case campaign == nil:
		reason = "deleted"
	case campaign.Status != models.Ongoing:
		reason = "not_ongoing"
	}
	if reason != "" {
		for _, c := range conns {
			jbEnd(c, reason)
		}
		return
	}
	followers := map[string]bool{}
	ids, _ := db.GetCampaignFollowerIDs(campaignID)
	for _, id := range ids {
		followers[id] = true
	}
	for _, c := range conns {
		role := jukeboxRole(campaign, c.userID, followers[c.userID])
		if (role == "dm") != c.dm || role == "" {
			jbEnd(c, "no_access")
		}
	}
}

// jukeboxRecheckUser ends every connection of a user (blocked, deleted or anonymized).
func jukeboxRecheckUser(userID string) {
	for _, c := range jukeboxConns("", userID) {
		jbEnd(c, "no_access")
	}
}

// CloseJukebox closes every connection with "going away" (server shutdown); clients reconnect.
func CloseJukebox() {
	for _, c := range jukeboxConns("", "") {
		if !c.send(jbMsg{closeCode: websocket.StatusGoingAway, reason: "server shutting down"}) {
			c.ws.CloseNow()
		}
	}
}
