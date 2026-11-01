package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"waystone-web/auth"
	"waystone-web/db"
	"waystone-web/middleware"
)

var (
	dummyHashOnce sync.Once
	dummyHash     string
)

const (
	authFailureWindow = 15 * time.Minute
	authMaxKeys       = 10000
)

var (
	// Keyed on ip|username so a stranger can't lock the real user out.
	loginAccountLimiter = auth.NewFailureLimiter(5, authFailureWindow, authMaxKeys)
	// Keyed on IP to cap guesses spread across many usernames.
	loginIPLimiter = auth.NewFailureLimiter(20, authFailureWindow, authMaxKeys)
	// Keyed on user ID to stop current-password guessing with a stolen session.
	changePasswordLimiter = auth.NewFailureLimiter(5, authFailureWindow, authMaxKeys)
)

// clientIP returns the last X-Forwarded-For entry (the one appended by the
// nearest proxy), else the host part of RemoteAddr. A client reaching the
// port directly can forge X-Forwarded-For; that only weakens the per-IP
// limit, not the per-account one.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// writeTooManyAttempts responds 429 with a Retry-After header in seconds.
func writeTooManyAttempts(w http.ResponseWriter, retry time.Duration) {
	secs := int((retry + time.Second - 1) / time.Second)
	mins := (secs + 59) / 60
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{
		"error": fmt.Sprintf("too many failed attempts, try again in %d min", mins),
	})
}

// getDummyHash returns a valid bcrypt hash used to equalise login timing when
// the user does not exist, is blocked or has no password.
func getDummyHash() string {
	dummyHashOnce.Do(func() {
		dummyHash, _ = auth.HashPassword("dummy-password-for-timing")
	})
	return dummyHash
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	ip := clientIP(r)
	accountKey := ip + "|" + strings.ToLower(strings.TrimSpace(body.Username))
	// Check before bcrypt so blocked clients don't cost CPU.
	if retry := max(loginAccountLimiter.RetryAfter(accountKey), loginIPLimiter.RetryAfter(ip)); retry > 0 {
		writeTooManyAttempts(w, retry)
		return
	}

	invalid := func() {
		loginAccountLimiter.RecordFailure(accountKey)
		loginIPLimiter.RecordFailure(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
	}

	username, err := auth.ParseUsername(body.Username)
	if err != nil {
		auth.CheckPassword(getDummyHash(), body.Password)
		invalid()
		return
	}

	user, err := db.GetUserByUsername(username)
	if err != nil {
		log.Printf("login: DB error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "login failed"})
		return
	}

	if user == nil || user.Blocked || user.PasswordHash == "" {
		auth.CheckPassword(getDummyHash(), body.Password)
		invalid()
		return
	}
	if !auth.CheckPassword(user.PasswordHash, body.Password) {
		invalid()
		return
	}

	loginAccountLimiter.Reset(accountKey)

	if err := middleware.SetSession(w, r, user.ID); err != nil {
		log.Printf("failed to set session: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set session"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":               "ok",
		"must_change_password": user.MustChangePassword,
		"needs_profile_setup":  user.NeedsProfileSetup,
	})
}

func HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	session, err := middleware.GetSession(r)
	userID, _ := session["user_id"].(string)
	if err != nil || userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	user, err := db.GetUserByID(userID)
	if err != nil {
		log.Printf("change-password: DB error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch user"})
		return
	}
	if user == nil || user.Blocked {
		middleware.ClearSession(w, r)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	if auth.IsEnvAdmin(user) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "this account's password is set via ADMIN_PASSWORD"})
		return
	}

	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if len(body.NewPassword) < auth.MinPasswordLength {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "new password must be at least 8 characters"})
		return
	}
	if body.NewPassword == body.CurrentPassword {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "new password must differ from the current one"})
		return
	}
	if retry := changePasswordLimiter.RetryAfter(user.ID); retry > 0 {
		writeTooManyAttempts(w, retry)
		return
	}
	if user.PasswordHash == "" || !auth.CheckPassword(user.PasswordHash, body.CurrentPassword) {
		changePasswordLimiter.RecordFailure(user.ID)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "current password is incorrect"})
		return
	}
	changePasswordLimiter.Reset(user.ID)

	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}
	if err := db.SetUserPassword(user.ID, hash, false); err != nil {
		log.Printf("change-password: failed to save: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save password"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}

func HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if err := middleware.ClearSession(w, r); err != nil {
		log.Printf("failed to clear session: %v", err)
		http.Error(w, "failed to logout", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}

func HandleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	session, err := middleware.GetSession(r)
	if err != nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	userID, ok := session["user_id"].(string)
	if !ok {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	// Reload fresh user data from DB
	user, err := db.GetUserByID(userID)
	if err != nil {
		log.Printf("error fetching user: %v", err)
		http.Error(w, "failed to fetch user", http.StatusInternalServerError)
		return
	}

	if user == nil {
		// User was deleted - clear session and return 401
		middleware.ClearSession(w, r)
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}

	if user.Blocked {
		middleware.ClearSession(w, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "account blocked"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":              user.ID,
		"username":             user.Username,
		"name":                 user.Name,
		"nickname":             user.Nickname,
		"display_name":         user.DisplayName(),
		"roles":                user.Roles,
		"must_change_password": user.MustChangePassword,
		"needs_profile_setup":  user.NeedsProfileSetup,
		"is_env_admin":         auth.IsEnvAdmin(user),
	})
}
