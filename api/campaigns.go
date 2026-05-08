package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"waystone-web/db"
	"waystone-web/models"
)

func HandleGetCampaigns(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	campaigns, err := db.GetAllCampaigns()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to retrieve campaigns"}`)
		return
	}

	// Fetch all users once to avoid N+1 queries
	allUsers, err := db.GetAllUsers()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to retrieve users"}`)
		return
	}

	// Create a map for O(1) user lookup
	userMap := make(map[string]*models.User)
	for i := range allUsers {
		userMap[allUsers[i].ID] = &allUsers[i]
	}

	// Build enriched campaign objects
	enriched := []map[string]interface{}{}
	for _, campaign := range campaigns {
		campaignObj := map[string]interface{}{
			"id":              campaign.ID,
			"title":           campaign.Title,
			"status":          campaign.Status,
			"summary":         campaign.Summary,
			"description":     campaign.Description,
			"dm":              campaign.DM,
			"players":         campaign.Players,
			"sign_ups_open":   campaign.SignUpsOpen,
		}

		// Add DM user display object if DM exists
		if campaign.DM != "" {
			if dmUser, ok := userMap[campaign.DM]; ok && dmUser != nil {
				displayName := dmUser.Nickname
				if displayName == "" {
					displayName = dmUser.Name
				}
				if displayName == "" {
					displayName = dmUser.Email
				}

				campaignObj["dm_user"] = map[string]interface{}{
					"id":           dmUser.ID,
					"name":         dmUser.Name,
					"nickname":     dmUser.Nickname,
					"display_name": displayName,
					"picture":      dmUser.Picture,
					"profile_url":  fmt.Sprintf("/profile?user_id=%s", dmUser.ID),
				}
			}
		}

		// Add player user display objects
		playerUsers := []map[string]interface{}{}
		for _, playerID := range campaign.Players {
			if playerUser, ok := userMap[playerID]; ok && playerUser != nil {
				displayName := playerUser.Nickname
				if displayName == "" {
					displayName = playerUser.Name
				}
				if displayName == "" {
					displayName = playerUser.Email
				}

				playerUsers = append(playerUsers, map[string]interface{}{
					"id":           playerUser.ID,
					"name":         playerUser.Name,
					"nickname":     playerUser.Nickname,
					"display_name": displayName,
					"picture":      playerUser.Picture,
					"profile_url":  fmt.Sprintf("/profile?user_id=%s", playerUser.ID),
				})
			}
		}
		campaignObj["player_users"] = playerUsers

		enriched = append(enriched, campaignObj)
	}

	w.WriteHeader(http.StatusOK)
	data, _ := json.Marshal(enriched)
	w.Write(data)
}
