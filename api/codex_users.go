package api

import (
	"net/http"
	"strings"
	"waystone-web/db"
	"waystone-web/models"
)

// mentionLabel is the display name as written inside a mention, @[label](id): one line, no brackets.
func mentionLabel(u *models.User) string {
	s := strings.Join(strings.Fields(strings.NewReplacer("[", " ", "]", " ").Replace(u.DisplayName())), " ")
	s = strings.TrimSpace(firstRunes(s, 100))
	if s == "" {
		return u.Username
	}
	return s
}

// handleCodexUsers lists everyone who can be mentioned on a page: every account that isn't deleted.
func handleCodexUsers(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := codexRightsFor(w, r); !ok {
		return
	}
	users, err := db.GetAllUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return
	}
	out := []map[string]string{}
	for i := range users {
		u := &users[i]
		if u.Deleted {
			continue
		}
		out = append(out, map[string]string{
			"id": u.ID, "username": u.Username, "display_name": u.DisplayName(), "label": mentionLabel(u),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// linkUserMentions turns every @username typed in body into @[Name](id); ok is false (with the
// response written) when the users can't be read.
func linkUserMentions(w http.ResponseWriter, body string) (string, bool) {
	if !strings.Contains(body, "@") {
		return body, true
	}
	users, err := db.GetAllUsers()
	if err != nil {
		codexServerError(w, "retrieve users")
		return "", false
	}
	links := make(map[string]string, len(users))
	for i := range users {
		if u := &users[i]; !u.Deleted {
			links[strings.ToLower(u.Username)] = "@[" + mentionLabel(u) + "](" + u.ID + ")"
		}
	}
	return db.LinkUsernames(body, links), true
}
