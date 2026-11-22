package discord

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// RunGateway keeps one gateway connection open so the bot shows as online with a custom
// status (Discord also requires a bot to have connected once before it may post). It asks for
// no intents and ignores every event; it reconnects with backoff until ctx is done, and stops
// for good when Discord rejects the token. Messages are still sent over REST.
func RunGateway(ctx context.Context, base, token, status string) {
	backoff := 5 * time.Second
	for ctx.Err() == nil {
		start := time.Now()
		err := runGatewayOnce(ctx, base, token, status)
		if ctx.Err() != nil {
			return
		}
		var ce websocket.CloseError
		if errors.As(err, &ce) && (ce.Code == 4004 || ce.Code == 4013 || ce.Code == 4014) {
			log.Printf("discord gateway: stopped: %v", err)
			return
		}
		if time.Since(start) > 5*time.Minute {
			backoff = 5 * time.Second
		}
		log.Printf("discord gateway: %v (reconnecting in %s)", err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 5*time.Minute {
			backoff *= 2
		}
	}
}

type gwPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	S  *int64          `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

func gatewayURL(ctx context.Context, base string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/gateway", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		URL string `json:"url"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&out) != nil || out.URL == "" {
		return "", errors.New("no gateway url (" + resp.Status + ")")
	}
	return strings.TrimRight(out.URL, "/") + "/?v=10&encoding=json", nil
}

func runGatewayOnce(ctx context.Context, base, token, status string) error {
	url, err := gatewayURL(ctx, base)
	if err != nil {
		return err
	}
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(8 << 20)

	var hello gwPayload
	if err := wsjson.Read(ctx, conn, &hello); err != nil {
		return err
	}
	var h struct {
		HeartbeatInterval int64 `json:"heartbeat_interval"`
	}
	if hello.Op != 10 || json.Unmarshal(hello.D, &h) != nil || h.HeartbeatInterval <= 0 {
		return errors.New("bad hello")
	}

	var mu sync.Mutex // one writer at a time
	var seq *int64
	send := func(v interface{}) error {
		mu.Lock()
		defer mu.Unlock()
		wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return wsjson.Write(wctx, conn, v)
	}
	heartbeat := func() error {
		mu.Lock()
		s := seq
		mu.Unlock()
		return send(map[string]interface{}{"op": 1, "d": s})
	}

	if err := send(map[string]interface{}{"op": 2, "d": map[string]interface{}{
		"token":      token,
		"intents":    0,
		"properties": map[string]string{"os": "linux", "browser": "waystone", "device": "waystone"},
		"presence": map[string]interface{}{
			"since": nil, "afk": false, "status": "online",
			"activities": []map[string]interface{}{{"type": 4, "name": "Custom Status", "state": status}},
		},
	}}); err != nil {
		return err
	}

	hbCtx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		t := time.NewTicker(time.Duration(h.HeartbeatInterval) * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-t.C:
				if heartbeat() != nil {
					conn.CloseNow()
					return
				}
			}
		}
	}()

	for {
		var p gwPayload
		if err := wsjson.Read(ctx, conn, &p); err != nil {
			return err
		}
		if p.S != nil {
			mu.Lock()
			seq = p.S
			mu.Unlock()
		}
		switch p.Op {
		case 1: // the gateway asks for a heartbeat now
			if err := heartbeat(); err != nil {
				return err
			}
		case 7, 9: // reconnect / invalid session: start over with a fresh identify
			return errors.New("gateway asked to reconnect")
		case 0:
			if p.T == "READY" {
				log.Printf("discord gateway: online")
			}
		}
	}
}
