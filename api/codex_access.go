package api

import (
	"net/http"
	"strings"
	"waystone-web/db"
	"waystone-web/models"
)

// codexRights is one user's Codices rights, loaded once per request. This file is the one place
// for the rules: admins can do everything; anyone else needs the See Codices right to reach
// anything, and then a per-codex level (see, or edit which implies see) for each codex.
// Edit Codices (canManage) lets a user create codices and rename or delete the ones they can see.
type codexRights struct {
	admin, see, edit bool
	levels           map[string]string // codex id -> "see" | "edit"
}

func loadCodexRights(userID string) (*codexRights, error) {
	u, err := db.GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	rt := &codexRights{}
	if u == nil {
		return rt, nil
	}
	if u.IsAdmin() {
		rt.admin = true
		return rt, nil
	}
	actions, err := db.GetUserRights(userID)
	if err != nil {
		return nil, err
	}
	for _, a := range actions {
		switch a {
		case models.RightCodicesSee:
			rt.see = true
		case models.RightCodicesEdit:
			rt.edit = true
		}
	}
	if rt.see {
		if rt.levels, err = db.GetCodexAccessForUser(userID); err != nil {
			return nil, err
		}
	}
	return rt, nil
}

// canUse reports whether the user may reach the Codices module at all.
func (c *codexRights) canUse() bool { return c.admin || c.see }

func (c *codexRights) canSee(codexID string) bool {
	return c.admin || (c.see && c.levels[codexID] != "")
}

func (c *codexRights) canEdit(codexID string) bool {
	return c.admin || (c.see && c.levels[codexID] == models.CodexLevelEdit)
}

// canManage reports whether the user may create codices and rename or delete visible ones.
func (c *codexRights) canManage() bool { return c.admin || (c.see && c.edit) }

// codexRightsFor returns the request user and their rights, or writes the error response.
func codexRightsFor(w http.ResponseWriter, r *http.Request) (string, *codexRights, bool) {
	userID, ok := codexUserID(w, r)
	if !ok {
		return "", nil, false
	}
	rt, err := loadCodexRights(userID)
	if err != nil {
		codexServerError(w, "retrieve rights")
		return "", nil, false
	}
	return userID, rt, true
}

// checkCodex answers 404 when the user can't see the codex (as if it didn't exist) and, for edit,
// 403 when they can see it but not change it. It reports whether the request may go on.
func (c *codexRights) checkCodex(w http.ResponseWriter, codexID string, edit bool) bool {
	switch {
	case !c.canSee(codexID):
		writeJSONError(w, http.StatusNotFound, db.ErrCodexNotFound.Error())
	case edit && !c.canEdit(codexID):
		writeJSONError(w, http.StatusForbidden, "you can't edit this codex")
	default:
		return true
	}
	return false
}

// checkManage answers 403 unless the user has the Edit Codices right.
func (c *codexRights) checkManage(w http.ResponseWriter) bool {
	if !c.canManage() {
		writeJSONError(w, http.StatusForbidden, "you can't create, rename or delete codices")
		return false
	}
	return true
}

// checkPage is checkCodex for the codex that holds a page; an unknown or invisible page is the
// same page_deleted 404.
func (c *codexRights) checkPage(w http.ResponseWriter, pageID string, edit bool) bool {
	codexID, err := db.GetPageCodexID(pageID)
	if err != nil {
		codexServerError(w, "retrieve page")
		return false
	}
	if codexID == "" || !c.canSee(codexID) {
		codexStoreError(w, db.ErrPageNotFound, "")
		return false
	}
	if edit && !c.canEdit(codexID) {
		writeJSONError(w, http.StatusForbidden, "you can't edit this codex")
		return false
	}
	return true
}

// checkFolder is checkCodex for the codex that holds a folder.
func (c *codexRights) checkFolder(w http.ResponseWriter, folderID string, edit bool) bool {
	f, err := db.GetFolder(folderID)
	if err != nil {
		codexServerError(w, "retrieve folder")
		return false
	}
	if f == nil || !c.canSee(f.CodexID) {
		codexFolderError(w, db.ErrFolderNotFound, "")
		return false
	}
	if edit && !c.canEdit(f.CodexID) {
		writeJSONError(w, http.StatusForbidden, "you can't edit this codex")
		return false
	}
	return true
}

// codexAllowedFor reports whether the user may use the Codices module at all (the See Codices
// right, or admin). Errors count as no.
func codexAllowedFor(userID string) bool {
	rt, err := loadCodexRights(userID)
	return err == nil && rt.canUse()
}

// codexGate hides the Codices pages and API from users who can't use them, as if they didn't exist:
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
