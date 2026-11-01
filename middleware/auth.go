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
	"waystone-web/models"
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

// SetSession stores the user ID and session version; everything else is read from the DB per request.
func SetSession(w http.ResponseWriter, r *http.Request, user *models.User) error {
	session, err := sessionStore.Get(r, SessionName)
	if err != nil {
		return err
	}

	session.Values["user_id"] = user.ID
	session.Values["session_version"] = user.SessionVersion

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

// SessionUser loads the session's user from the DB. It returns (nil, nil) and
// clears the cookie when the session is missing, the user is gone, or the
// cookie's session_version no longer matches (password changed, blocked, ...).
// Blocked users are returned as-is so callers can respond accordingly.
func SessionUser(w http.ResponseWriter, r *http.Request) (*models.User, error) {
	session, err := GetSession(r)
	if err != nil {
		return nil, nil
	}
	userID, _ := session["user_id"].(string)
	if userID == "" {
		ClearSession(w, r)
		return nil, nil
	}
	user, err := db.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Deleted {
		ClearSession(w, r)
		return nil, nil
	}
	if user.Blocked {
		return user, nil
	}
	if version, ok := session["session_version"].(int); !ok || version != user.SessionVersion {
		ClearSession(w, r)
		return nil, nil
	}
	return user, nil
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAPI := strings.HasPrefix(r.URL.Path, "/api/")
		dbUser, err := SessionUser(w, r)
		if err != nil {
			// Fail closed on DB errors: we can't verify the session.
			log.Printf("AuthMiddleware: DB error: %v", err)
			if isAPI {
				writeJSONError(w, http.StatusInternalServerError, "failed to verify session")
			} else {
				http.Error(w, "failed to verify session", http.StatusInternalServerError)
			}
			return
		}
		if dbUser == nil {
			if isAPI {
				writeJSONError(w, http.StatusUnauthorized, "session expired")
			} else {
				http.Redirect(w, r, "/login.html", http.StatusSeeOther)
			}
			return
		}
		if dbUser.Blocked {
			ClearSession(w, r)
			if isAPI {
				writeJSONError(w, http.StatusForbidden, "account blocked")
			} else {
				http.Redirect(w, r, "/blocked.html", http.StatusSeeOther)
			}
			return
		}
		if dbUser.MustChangePassword {
			if isAPI {
				writeJSONError(w, http.StatusForbidden, "password change required")
				return
			}
			if r.URL.Path != "/change-password" {
				http.Redirect(w, r, "/change-password", http.StatusSeeOther)
				return
			}
		}

		ctx := context.WithValue(r.Context(), "user_id", dbUser.ID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
