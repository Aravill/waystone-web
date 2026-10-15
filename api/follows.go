package api

import (
	"net/http"
	"strings"
	"waystone-web/db"
)

// HandleCampaignFollow handles /api/campaigns/{id}/follow (PUT follow, DELETE unfollow).
func HandleCampaignFollow(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) != 2 || parts[1] != "follow" {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	campaign, userID, ok := loadCampaignAndUser(w, r, parts[0])
	if !ok {
		return
	}

	if r.Method == http.MethodPut {
		if campaign.DM == userID {
			writeJSONError(w, http.StatusConflict, "you run this campaign")
			return
		}
		if err := db.FollowCampaign(campaign.ID, userID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to update follow")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "success", "message": "following", "following": true})
		return
	}

	if err := db.UnfollowCampaign(campaign.ID, userID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update follow")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "success", "message": "unfollowed", "following": false})
}
