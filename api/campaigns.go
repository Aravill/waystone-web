package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"waystone-web/db"
	"waystone-web/models"
)

func HandleCampaigns(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		HandleGetCampaigns(w, r)
	} else if r.Method == http.MethodPost {
		HandleCreateCampaign(w, r)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, `{"error": "method not allowed"}`)
	}
}

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
			"id":                   campaign.ID,
			"title":                campaign.Title,
			"status":               campaign.Status,
			"summary":              campaign.Summary,
			"description":          campaign.Description,
			"dm":                   campaign.DM,
			"players":              campaign.Players,
			"sign_ups_open":        campaign.SignUpsOpen,
			"desired_player_count": campaign.DesiredPlayerCount,
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

func HandleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, `{"error": "method not allowed"}`)
		return
	}

	// Get authenticated user from context
	userID, ok := r.Context().Value("user_id").(string)
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error": "not authenticated"}`)
		return
	}

	// Parse request body
	type CreateCampaignRequest struct {
		Title              string `json:"title"`
		Summary            string `json:"summary"`
		Description        string `json:"description"`
		DesiredPlayerCount int    `json:"desired_player_count"`
	}

	var req CreateCampaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "invalid request body"}`)
		return
	}

	// Validate required fields
	if strings.TrimSpace(req.Title) == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "title is required"}`)
		return
	}
	if strings.TrimSpace(req.Summary) == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "summary is required"}`)
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "description is required"}`)
		return
	}
	if req.DesiredPlayerCount <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "desired player count must be a positive number"}`)
		return
	}

	// Create campaign with DM set to authenticated user
	campaign := models.Campaign{
		ID:                 db.GetNextCampaignID(),
		Title:              strings.TrimSpace(req.Title),
		Summary:            strings.TrimSpace(req.Summary),
		Description:        strings.TrimSpace(req.Description),
		DesiredPlayerCount: strconv.Itoa(req.DesiredPlayerCount),
		DM:                 userID,
		Players:            []string{},
		Status:             models.Pitch,
		SignUpsOpen:        true,
	}

	// Save campaign to database
	if err := db.SaveCampaign(campaign); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to create campaign"}`)
		return
	}

	w.WriteHeader(http.StatusCreated)
	data, _ := json.Marshal(map[string]interface{}{
		"status":      "success",
		"message":     "campaign created successfully",
		"campaign_id": campaign.ID,
	})
	w.Write(data)
}
