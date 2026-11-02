package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"waystone-web/db"
	"waystone-web/models"
)

const (
	defaultGuestTTLHours = 24
	maxGuestTTLHours     = 24 * 365
)

// newToken returns 32 random bytes as unpadded base64url.
func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HandleAdminCampaignActions handles POST /api/admin/campaigns/{cid}/players and
// DELETE /api/admin/campaigns/{cid}/players/{uid}.
func HandleAdminCampaignActions(w http.ResponseWriter, r *http.Request) {
	caller, err := getAdminCaller(r)
	if err != nil || caller == nil {
		adminForbidden(w)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/admin/campaigns/"), "/")
	campaignID := parts[0]
	rest := parts[1:]
	if campaignID == "" || len(rest) == 0 {
		adminError(w, http.StatusNotFound, "not found")
		return
	}
	campaign, err := db.GetCampaignByID(campaignID)
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to retrieve game")
		return
	}
	if campaign == nil {
		adminError(w, http.StatusNotFound, "game not found")
		return
	}

	switch {
	case r.Method == http.MethodPost && len(rest) == 1 && rest[0] == "players":
		adminAddPlayer(w, r, caller, campaign)

	case r.Method == http.MethodDelete && len(rest) == 2 && rest[0] == "players":
		uid := rest[1]
		if !isCampaignPlayer(campaign, uid) {
			adminError(w, http.StatusNotFound, "player not found")
			return
		}
		if applySignupStatus(w, campaignID, uid, models.SignupKicked, "player removed") != nil {
			_ = db.DeleteGuestToken(campaignID, uid)
			notify(caller.ID, []string{uid}, models.NotificationPlayerKicked, campaign, "", nil)
		}

	default:
		adminError(w, http.StatusNotFound, "not found")
	}
}

func adminAddPlayer(w http.ResponseWriter, r *http.Request, caller *models.User, campaign *models.Campaign) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		adminError(w, http.StatusBadRequest, "user_id required")
		return
	}
	target, err := db.GetUserByID(req.UserID)
	if err != nil || target == nil || target.Deleted {
		adminError(w, http.StatusNotFound, "user not found")
		return
	}
	if campaign.DM == target.ID {
		adminError(w, http.StatusConflict, "the DM cannot be a player")
		return
	}
	if isCampaignPlayer(campaign, target.ID) {
		adminError(w, http.StatusConflict, "already a player")
		return
	}

	existing, err := db.GetCampaignSignup(campaign.ID, target.ID)
	if err != nil {
		adminError(w, http.StatusInternalServerError, "failed to check sign-up")
		return
	}
	if existing == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := db.CreateCampaignSignup(models.CampaignSignup{
			ID:         db.GenerateUUID(),
			CampaignID: campaign.ID,
			UserID:     target.ID,
			Status:     models.SignupPending,
			CreatedAt:  now,
			UpdatedAt:  now,
		}); err != nil {
			adminError(w, http.StatusInternalServerError, "failed to create sign-up")
			return
		}
	}

	updated := applySignupStatus(w, campaign.ID, target.ID, models.SignupAccepted, "player added")
	if updated == nil {
		return
	}
	notify(caller.ID, []string{target.ID}, models.NotificationSignupAccepted, campaign, "", nil)
	if campaign.SignUpsOpen && !updated.SignUpsOpen {
		notify("", []string{campaign.DM}, models.NotificationCampaignFull, campaign, "", map[string]string{
			"player_count": strconv.Itoa(len(updated.Players)),
		})
	}
}

// HandleCampaignGuestLinks handles the DM/admin guest-link routes:
// GET /api/campaigns/{id}/guest-links and
// POST|DELETE /api/campaigns/{id}/players/{uid}/guest-link.
func HandleCampaignGuestLinks(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	campaign, ok := loadManagedCampaign(w, r, parts[0])
	if !ok {
		return
	}

	switch {
	case len(parts) == 2 && parts[1] == "guest-links":
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		links, err := db.GetGuestTokens(campaign.ID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to load guest links")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"links": links})
	case len(parts) == 4 && parts[1] == "players" && parts[3] == "guest-link":
		switch r.Method {
		case http.MethodPost:
			createGuestLink(w, r, campaign, parts[2])
		case http.MethodDelete:
			if err := db.DeleteGuestToken(campaign.ID, parts[2]); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to revoke guest link")
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
		default:
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

func createGuestLink(w http.ResponseWriter, r *http.Request, campaign *models.Campaign, uid string) {
	var req struct {
		TTLHours int `json:"ttl_hours"`
	}
	if r.Body != nil {
		// An empty body means the default TTL.
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if req.TTLHours == 0 {
		req.TTLHours = defaultGuestTTLHours
	}
	if req.TTLHours < 1 || req.TTLHours > maxGuestTTLHours {
		writeJSONError(w, http.StatusBadRequest, "ttl_hours must be between 1 and "+strconv.Itoa(maxGuestTTLHours))
		return
	}

	user, err := db.GetUserByID(uid)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	if user == nil || user.Deleted || user.Blocked || !isCampaignPlayer(campaign, uid) {
		writeJSONError(w, http.StatusConflict, "user is not an active player of this game")
		return
	}

	token, err := newToken()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	expiresAt := time.Now().UTC().Add(time.Duration(req.TTLHours) * time.Hour)
	if err := db.SetGuestToken(campaign.ID, uid, token, expiresAt); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save guest link")
		return
	}
	writeJSON(w, http.StatusOK, models.GuestToken{Token: token, ExpiresAt: expiresAt.Format(time.RFC3339)})
}

// guestHeaders keeps tokens out of caches and Referer headers.
func guestHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// resolveGuest validates a guest token and returns its campaign, user and expiry. On
// failure it writes the error response and returns false. It never touches
// the session or cookies.
func resolveGuest(w http.ResponseWriter, token string) (*models.Campaign, *models.User, time.Time, bool) {
	campaignID, userID, expiresAt, err := db.GetGuestToken(token)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return nil, nil, time.Time{}, false
	}
	if campaignID == "" {
		writeJSONError(w, http.StatusNotFound, "link is not valid")
		return nil, nil, time.Time{}, false
	}
	if !expiresAt.After(time.Now()) {
		writeJSONError(w, http.StatusGone, "link has expired")
		return nil, nil, time.Time{}, false
	}
	user, err := db.GetUserByID(userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return nil, nil, time.Time{}, false
	}
	campaign, err := db.GetCampaignByID(campaignID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return nil, nil, time.Time{}, false
	}
	if user == nil || user.Deleted || user.Blocked || campaign == nil || !isCampaignPlayer(campaign, userID) {
		writeJSONError(w, http.StatusNotFound, "link is not valid")
		return nil, nil, time.Time{}, false
	}
	return campaign, user, expiresAt, true
}

// HandleGuestPage serves the guest reply page at /r/{token}. The token is
// validated by the page's API calls.
func HandleGuestPage(w http.ResponseWriter, r *http.Request) {
	guestHeaders(w)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "./static/guest.html")
}

// HandleGuestAPI handles GET /api/guest/{token} and
// POST /api/guest/{token}/sessions/{sid}/responses.
func HandleGuestAPI(w http.ResponseWriter, r *http.Request) {
	guestHeaders(w)
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/guest/"), "/")
	token := parts[0]

	switch {
	case len(parts) == 1:
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		handleGuestOverview(w, token)
	case len(parts) == 4 && parts[1] == "sessions" && parts[3] == "responses":
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		handleGuestRespond(w, r, token, parts[2])
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

func handleGuestOverview(w http.ResponseWriter, token string) {
	campaign, user, expiresAt, ok := resolveGuest(w, token)
	if !ok {
		return
	}

	today := time.Now().UTC().Format("2006-01-02")
	sessions, err := db.GetSessionsByCampaignIDs([]string{campaign.ID}, today)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load sessions")
		return
	}
	ids := make([]string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.ID)
	}
	responses, err := db.GetSessionResponsesBySessionIDs(ids)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load responses")
		return
	}
	for i := range sessions {
		applySessionCounts(&sessions[i], campaign, responses[sessions[i].ID], user.ID)
	}
	if sessions == nil {
		sessions = []models.CampaignSession{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"campaign":   map[string]string{"id": campaign.ID, "title": campaign.Title},
		"user":       map[string]string{"display_name": user.DisplayName()},
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
		"sessions":   sessions,
	})
}

func handleGuestRespond(w http.ResponseWriter, r *http.Request, token, sessionID string) {
	campaign, user, _, ok := resolveGuest(w, token)
	if !ok {
		return
	}
	_, session, err := validateSessionCampaign(campaign.ID, sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve session")
		return
	}
	if session == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}
	if session.Status == models.SessionCancelled {
		writeJSONError(w, http.StatusConflict, "session is cancelled")
		return
	}
	writeSessionResponse(w, r, sessionID, user.ID)
}
