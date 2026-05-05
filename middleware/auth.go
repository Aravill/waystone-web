package middleware

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	sessionStore sessions.Store
	oauth2Config *oauth2.Config
	verifier     *oidc.IDTokenVerifier
)

const SessionName = "auth_session"

type Claims struct {
	GoogleID string `json:"sub"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Picture  string `json:"picture"`
}

func InitAuth(callbackURL string) error {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	sessionSecret := os.Getenv("SESSION_SECRET")

	if sessionSecret == "" {
		sessionSecret = "default-insecure-secret-change-this"
	}

	// Always initialize session store
	sessionStore = sessions.NewCookieStore([]byte(sessionSecret))

	// OAuth is optional for development
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET environment variables required")
	}

	oauth2Config = &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  callbackURL,
		Scopes: []string{
			oidc.ScopeOpenID,
			"email",
			"profile",
		},
		Endpoint: google.Endpoint,
	}

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	verifier = provider.Verifier(&oidc.Config{ClientID: clientID})
	return nil
}

func GetOAuth2Config() *oauth2.Config {
	if oauth2Config == nil {
		panic("auth not initialized")
	}
	return oauth2Config
}

func GetVerifier() *oidc.IDTokenVerifier {
	if verifier == nil {
		panic("auth not initialized")
	}
	return verifier
}

func SetSession(w http.ResponseWriter, r *http.Request, userID, googleID, email, name, picture string, roles []string) error {
	session, err := sessionStore.Get(r, SessionName)
	if err != nil {
		return err
	}

	session.Values["user_id"] = userID
	session.Values["google_id"] = googleID
	session.Values["email"] = email
	session.Values["name"] = name
	session.Values["picture"] = picture
	session.Values["roles"] = roles

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

		ctx := context.WithValue(r.Context(), "user_id", session["user_id"])
		ctx = context.WithValue(ctx, "user_email", session["email"])
		ctx = context.WithValue(ctx, "user_name", session["name"])
		ctx = context.WithValue(ctx, "google_id", session["google_id"])

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
