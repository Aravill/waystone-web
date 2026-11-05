package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
	"waystone-web/auth"
	"waystone-web/db"
	"waystone-web/middleware"
	"waystone-web/models"
)

const inviteTTL = 7 * 24 * time.Hour

// newToken returns 32 random bytes as unpadded base64url.
func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashInviteToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// issueInvite creates a one-time password invite for the user, replacing any earlier
// one and clearing the current password. Only the token's hash is stored.
func issueInvite(userID string) (string, time.Time, error) {
	token, err := newToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().UTC().Add(inviteTTL)
	if err := db.IssuePasswordInvite(userID, hashInviteToken(token), expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// inviteHeaders keeps tokens out of caches and Referer headers.
func inviteHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// resolveInvite validates an invite token and returns its user and expiry. On
// failure it writes the error response and returns false. It never touches the
// session or cookies.
func resolveInvite(w http.ResponseWriter, token string) (*models.User, time.Time, bool) {
	userID, expiresAt, err := db.GetPasswordInvite(hashInviteToken(token))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return nil, time.Time{}, false
	}
	if userID == "" {
		writeJSONError(w, http.StatusNotFound, "link is not valid")
		return nil, time.Time{}, false
	}
	user, err := db.GetUserByID(userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return nil, time.Time{}, false
	}
	if user == nil || user.Deleted || user.Blocked || auth.IsEnvAdmin(user) {
		writeJSONError(w, http.StatusNotFound, "link is not valid")
		return nil, time.Time{}, false
	}
	if !expiresAt.After(time.Now()) {
		writeJSONError(w, http.StatusGone, "link has expired")
		return nil, time.Time{}, false
	}
	return user, expiresAt, true
}

// HandleInvitePage serves the invite page at /invite/{token}. The token is
// validated by the page's API calls.
func HandleInvitePage(w http.ResponseWriter, r *http.Request) {
	inviteHeaders(w)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "./static/invite.html")
}

// HandleInviteAPI handles GET /api/invite/{token} and
// POST /api/invite/{token}/password.
func HandleInviteAPI(w http.ResponseWriter, r *http.Request) {
	inviteHeaders(w)
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/invite/"), "/")
	token := parts[0]

	switch {
	case len(parts) == 1:
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		user, expiresAt, ok := resolveInvite(w, token)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"user":       map[string]string{"username": user.Username, "display_name": user.DisplayName()},
			"expires_at": expiresAt.UTC().Format(time.RFC3339),
		})
	case len(parts) == 2 && parts[1] == "password":
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		handleInviteSetPassword(w, r, token)
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

func handleInviteSetPassword(w http.ResponseWriter, r *http.Request, token string) {
	user, _, ok := resolveInvite(w, token)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var body struct {
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if v := auth.PasswordPolicyViolations(body.NewPassword, user.Username); len(v) > 0 {
		writeJSONError(w, http.StatusBadRequest, "new "+auth.PasswordPolicyError(v))
		return
	}
	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	userID, err := db.RedeemPasswordInvite(hashInviteToken(token), hash)
	if err != nil {
		log.Printf("invite: failed to set password: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to save password")
		return
	}
	if userID == "" {
		writeJSONError(w, http.StatusNotFound, "link is not valid")
		return
	}

	// Replace any other session in this browser with the new user's.
	updated, err := db.GetUserByID(userID)
	if err != nil || updated == nil {
		log.Printf("invite: failed to reload user: %v", err)
		updated = nil
	} else if err := middleware.SetSession(w, r, updated); err != nil {
		log.Printf("invite: failed to start session: %v", err)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":              "password set",
		"needs_profile_setup": updated != nil && updated.NeedsProfileSetup,
	})
}
