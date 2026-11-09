package api

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// codexAdminOnly limits the Codex to admins while it is being tried out in production
// (CODEX_ADMIN_ONLY; docker-compose turns it on, a plain `go run` leaves it off).
// Any non-empty value that isn't a valid boolean counts as on (fail closed) and is logged once.
var codexAdminOnly = parseCodexAdminOnly(os.Getenv("CODEX_ADMIN_ONLY"))

func parseCodexAdminOnly(v string) bool {
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		log.Printf("CODEX_ADMIN_ONLY=%q is not a boolean (true/false/1/0): treating it as on", v)
		return true
	}
	return b
}

// codexAllowedFor reports whether the user may use the Codex at all.
func codexAllowedFor(userID string) bool {
	if !codexAdminOnly {
		return true
	}
	ok, _ := codexIsAdmin(userID)
	return ok
}

// codexGate hides the Codex pages and API from users who can't use it, as if it didn't exist:
// API calls get 404, page loads go to Home. Wrap it inside AuthMiddleware so user_id is set.
func codexGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value("user_id").(string)
		if !codexAllowedFor(userID) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeJSONError(w, http.StatusNotFound, "not found")
			} else {
				http.Redirect(w, r, "/", http.StatusSeeOther)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}
