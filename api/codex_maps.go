package api

import (
	"net/http"
	"waystone-web/models"
)

// Map pages: filled in with the map image and pin endpoints.

func handleCodexMapRoutes(w http.ResponseWriter, r *http.Request, id string, rest []string, method func(...string) bool) bool {
	return false
}

func codexAddMapPayload(payload map[string]interface{}, page *models.Page) error { return nil }

func codexAddOnMaps(payload map[string]interface{}, backs []models.PageBacklink) error { return nil }

func removeCodexFiles(files []string) {}

func handleCodexFile(w http.ResponseWriter, r *http.Request, name string) {
	writeJSONError(w, http.StatusNotFound, "not found")
}
