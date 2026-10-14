package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"
)

const maxLogoBytes = 256 << 10

var allowedLogoTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

type systemJSON struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	LogoURL string `json:"logo_url"`
}

func systemToJSON(s models.System) systemJSON {
	return systemJSON{ID: s.ID, Name: s.Name, LogoURL: s.LogoURL()}
}

func systemListJSON(systems []models.System) []systemJSON {
	out := make([]systemJSON, 0, len(systems))
	for _, s := range systems {
		out = append(out, systemToJSON(s))
	}
	return out
}

// systemsByName returns all known systems keyed by lower-cased name.
func systemsByName() (map[string]models.System, error) {
	systems, err := db.GetAllSystems()
	if err != nil {
		return nil, err
	}
	m := make(map[string]models.System, len(systems))
	for _, s := range systems {
		m[strings.ToLower(s.Name)] = s
	}
	return m, nil
}

// canonicalSystemName returns the known system's spelling for a case-insensitive match, else name unchanged.
func canonicalSystemName(name string) (string, error) {
	if name == "" {
		return "", nil
	}
	known, err := systemsByName()
	if err != nil {
		return "", err
	}
	if s, ok := known[strings.ToLower(name)]; ok {
		return s.Name, nil
	}
	return name, nil
}

// normalizeSystemName trims name and returns a validation message, or "" if valid.
func normalizeSystemName(name string) (string, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return name, "name is required"
	}
	if utf8.RuneCountInString(name) > 64 {
		return name, "name must be at most 64 characters"
	}
	return name, ""
}

// systemNameTaken reports whether another system already uses name (case-insensitive).
func systemNameTaken(name, exceptID string) (bool, error) {
	systems, err := db.GetAllSystems()
	if err != nil {
		return false, err
	}
	for _, s := range systems {
		if s.ID != exceptID && strings.EqualFold(s.Name, name) {
			return true, nil
		}
	}
	return false, nil
}

// HandleSystems handles GET /api/systems (any authenticated user).
func HandleSystems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	systems, err := db.GetAllSystems()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve systems")
		return
	}
	writeJSON(w, http.StatusOK, systemListJSON(systems))
}

// HandleSystemRoutes handles GET /api/systems/{id}/logo.
func HandleSystemRoutes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/systems/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "logo" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	data, contentType, err := db.GetSystemLogo(parts[0])
	if errors.Is(err, db.ErrSystemNotFound) || (err == nil && len(data) == 0) {
		writeJSONError(w, http.StatusNotFound, "logo not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve logo")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// HandleAdminSystems handles /api/admin/systems (GET list, POST create).
func HandleAdminSystems(w http.ResponseWriter, r *http.Request) {
	caller, err := getAdminCaller(r)
	if err != nil || caller == nil {
		adminForbidden(w)
		return
	}

	switch r.Method {
	case http.MethodGet:
		systems, err := db.GetAllSystems()
		if err != nil {
			adminError(w, http.StatusInternalServerError, "failed to retrieve systems")
			return
		}
		writeJSON(w, http.StatusOK, systemListJSON(systems))
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			adminError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		name, msg := normalizeSystemName(req.Name)
		if msg != "" {
			adminError(w, http.StatusBadRequest, msg)
			return
		}
		taken, err := systemNameTaken(name, "")
		if err != nil {
			adminError(w, http.StatusInternalServerError, "failed to create system")
			return
		}
		if taken {
			adminError(w, http.StatusConflict, "system already exists")
			return
		}
		sys := models.System{ID: db.GenerateUUID(), Name: name}
		if err := db.CreateSystem(sys); err != nil {
			adminError(w, http.StatusInternalServerError, "failed to create system")
			return
		}
		writeJSON(w, http.StatusCreated, systemToJSON(sys))
	default:
		adminError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleAdminSystemActions handles /api/admin/systems/{id} (PUT rename, DELETE) and /api/admin/systems/{id}/logo (PUT, DELETE).
func HandleAdminSystemActions(w http.ResponseWriter, r *http.Request) {
	caller, err := getAdminCaller(r)
	if err != nil || caller == nil {
		adminForbidden(w)
		return
	}

	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/admin/systems/"), "/", 2)
	id := parts[0]
	if id == "" {
		adminError(w, http.StatusBadRequest, "system id required")
		return
	}
	if len(parts) == 2 {
		if parts[1] != "logo" {
			adminError(w, http.StatusNotFound, "not found")
			return
		}
		handleAdminSystemLogo(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			adminError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		name, msg := normalizeSystemName(req.Name)
		if msg != "" {
			adminError(w, http.StatusBadRequest, msg)
			return
		}
		taken, err := systemNameTaken(name, id)
		if err != nil {
			adminError(w, http.StatusInternalServerError, "failed to rename system")
			return
		}
		if taken {
			adminError(w, http.StatusConflict, "system already exists")
			return
		}
		if err := db.RenameSystem(id, name); err != nil {
			systemWriteError(w, err, "failed to rename system")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "system renamed"})
	case http.MethodDelete:
		if err := db.DeleteSystem(id); err != nil {
			systemWriteError(w, err, "failed to delete system")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "system deleted"})
	default:
		adminError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func handleAdminSystemLogo(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, maxLogoBytes)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				adminError(w, http.StatusRequestEntityTooLarge, "logo must be 256 KB or smaller")
				return
			}
			adminError(w, http.StatusBadRequest, "failed to read logo")
			return
		}
		if len(data) == 0 {
			adminError(w, http.StatusBadRequest, "logo file is empty")
			return
		}
		contentType := http.DetectContentType(data)
		if !allowedLogoTypes[contentType] {
			adminError(w, http.StatusBadRequest, "logo must be a PNG, JPEG, WebP or GIF image")
			return
		}
		if err := db.SetSystemLogo(id, data, contentType); err != nil {
			systemWriteError(w, err, "failed to save logo")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "logo updated"})
	case http.MethodDelete:
		if err := db.SetSystemLogo(id, nil, ""); err != nil {
			systemWriteError(w, err, "failed to remove logo")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "logo removed"})
	default:
		adminError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func systemWriteError(w http.ResponseWriter, err error, fallback string) {
	if errors.Is(err, db.ErrSystemNotFound) {
		adminError(w, http.StatusNotFound, err.Error())
		return
	}
	adminError(w, http.StatusInternalServerError, fallback)
}
