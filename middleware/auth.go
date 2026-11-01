package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/gorilla/sessions"
	"waystone-web/db"
)

var sessionStore sessions.Store

const (
	SessionName         = "auth_session"
	minSessionSecretLen = 32
)

// Placeholder secrets that shipped in earlier code/config; anyone could forge cookies with them.
var knownPlaceholderSecrets = []string{
	"default-insecure-secret-change-this",
	"your-secret-key-change-me",
	"change-me-to-random-secret-in-production",
}

// InitAuth exits the process if SESSION_SECRET is missing, a known placeholder, or too short.
func InitAuth() {
	sessionSecret := os.Getenv("SESSION_SECRET")
	const hint = "generate one with: openssl rand -base64 32"
	switch {
	case sessionSecret == "":
		log.Fatalf("SESSION_SECRET is not set; %s", hint)
	case slices.Contains(knownPlaceholderSecrets, sessionSecret):
		log.Fatalf("SESSION_SECRET is a known placeholder value; %s", hint)
	case len(sessionSecret) < minSessionSecretLen:
		log.Fatalf("SESSION_SECRET must be at least %d characters (got %d); %s", minSessionSecretLen, len(sessionSecret), hint)
	}

	store := sessions.NewCookieStore([]byte(sessionSecret))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	sessionStore = store
}

// SetSession stores only the user ID; everything else is read from the DB per request.
func SetSession(w http.ResponseWriter, r *http.Request, userID string) error {
	session, err := sessionStore.Get(r, SessionName)
	if err != nil {
		return err
	}

	session.Values["user_id"] = userID

	return session.Save(r, w)
}

func GetSession(r *http.Request) (map[interface{}]interface{}, error) {
	session, err := sessionStore.Get(r, SessionName)
	if err != nil {
		return nil, err
	}

	if session.IsNew {
		return nil, fmt.Errorf("not authenticated")
	}

	return session.Values, nil
}

func ClearSession(w http.ResponseWriter, r *http.Request) error {
	session, err := sessionStore.Get(r, SessionName)
	if err != nil {
		return err
	}

	session.Options.MaxAge = -1
	return session.Save(r, w)
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := GetSession(r)
		if err != nil || session["user_id"] == nil {
			http.Redirect(w, r, "/login.html", http.StatusSeeOther)
			return
		}

		// DB check: handle deleted users and blocked users
		userID, ok := session["user_id"].(string)
		if !ok || userID == "" {
			ClearSession(w, r)
			http.Redirect(w, r, "/login.html", http.StatusSeeOther)
			return
		}
		dbUser, dbErr := db.GetUserByID(userID)
		if dbErr != nil {
			log.Printf("AuthMiddleware: DB error for user %s: %v", userID, dbErr)
			// fail open on transient DB errors
		} else if dbUser == nil || dbUser.Deleted {
			// stale cookie — user deleted
			ClearSession(w, r)
			http.Redirect(w, r, "/login.html", http.StatusSeeOther)
			return
		} else if dbUser.Blocked {
			ClearSession(w, r)
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{"error": "account blocked"})
			} else {
				http.Redirect(w, r, "/blocked.html", http.StatusSeeOther)
			}
			return
		} else if dbUser.MustChangePassword {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{"error": "password change required"})
				return
			}
			if r.URL.Path != "/change-password" {
				http.Redirect(w, r, "/change-password", http.StatusSeeOther)
				return
			}
		}

		ctx := context.WithValue(r.Context(), "user_id", session["user_id"])

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
