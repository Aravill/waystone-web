package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
	"waystone-web/db"
	"waystone-web/models"
)

// characterRequest is the body for creating and updating a character.
type characterRequest struct {
	Name        string                 `json:"name"`
	SystemID    string                 `json:"system_id"`
	Description string                 `json:"description"`
	Background  string                 `json:"background"`
	Status      models.CharacterStatus `json:"status"`
}

// normalize trims the text fields and returns a validation message, or "" if valid.
func (req *characterRequest) normalize() string {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.Background = strings.TrimSpace(req.Background)
	req.SystemID = strings.TrimSpace(req.SystemID)
	if req.Status == "" {
		req.Status = models.CharacterAlive
	}
	switch {
	case req.Name == "":
		return "name is required"
	case utf8.RuneCountInString(req.Name) > 64:
		return "name must be at most 64 characters"
	case utf8.RuneCountInString(req.Description) > 10000:
		return "description must be at most 10000 characters"
	case utf8.RuneCountInString(req.Background) > 10000:
		return "background must be at most 10000 characters"
	case !req.Status.IsValid():
		return "invalid status"
	}
	return ""
}

// characterViewer is the lookup data for rendering characters as seen by one viewer.
type characterViewer struct {
	userID    string
	isAdmin   bool
	users     map[string]*models.User
	systems   map[string]models.System
	campaigns map[string]models.Campaign
}

func loadCharacterViewer(r *http.Request) (*characterViewer, string) {
	v := &characterViewer{users: map[string]*models.User{}, campaigns: map[string]models.Campaign{}}
	v.userID, _ = r.Context().Value("user_id").(string)
	users, err := db.GetAllUsers()
	if err != nil {
		return nil, "failed to retrieve users"
	}
	for i := range users {
		v.users[users[i].ID] = &users[i]
	}
	if u := v.users[v.userID]; u != nil {
		v.isAdmin = u.IsAdmin()
	}
	if v.systems, err = systemsByID(); err != nil {
		return nil, "failed to retrieve systems"
	}
	campaigns, err := db.GetAllCampaigns()
	if err != nil {
		return nil, "failed to retrieve games"
	}
	for _, c := range campaigns {
		v.campaigns[c.ID] = c
	}
	return v, ""
}

// characterGames reduces a character's assignments to one badge per game: the active one if
// any, else the latest ended one. Assignments must be ordered oldest first.
func (v *characterViewer) characterGames(assignments []models.CharacterAssignment) []map[string]interface{} {
	byCampaign := map[string]models.CharacterAssignment{}
	order := []string{}
	for _, a := range assignments {
		cur, seen := byCampaign[a.CampaignID]
		if !seen {
			order = append(order, a.CampaignID)
		}
		if !seen || cur.EndedAt != "" {
			byCampaign[a.CampaignID] = a
		}
	}
	games := []map[string]interface{}{}
	for _, id := range order {
		c, ok := v.campaigns[id]
		if !ok {
			continue
		}
		a := byCampaign[id]
		games = append(games, map[string]interface{}{
			"campaign_id":     c.ID,
			"title":           c.Title,
			"campaign_status": c.Status,
			"system":          c.System,
			"active":          a.EndedAt == "",
			"started_at":      a.StartedAt,
			"ended_at":        a.EndedAt,
		})
	}
	return games
}

// canSetStatus: the owner, an admin, or the DM of a game the character is actively assigned to.
func (v *characterViewer) canSetStatus(c models.Character, assignments []models.CharacterAssignment) bool {
	if v.isAdmin || c.OwnerID == v.userID {
		return true
	}
	for _, a := range assignments {
		if a.EndedAt == "" && v.campaigns[a.CampaignID].DM == v.userID && v.userID != "" {
			return true
		}
	}
	return false
}

func (v *characterViewer) characterJSON(c models.Character, assignments []models.CharacterAssignment) map[string]interface{} {
	obj := map[string]interface{}{
		"id":              c.ID,
		"owner_id":        c.OwnerID,
		"system_id":       c.SystemID,
		"system":          nil,
		"name":            c.Name,
		"description":     c.Description,
		"background":      c.Background,
		"status":          c.Status,
		"created_at":      c.CreatedAt,
		"updated_at":      c.UpdatedAt,
		"games":           v.characterGames(assignments),
		"can_edit":        c.OwnerID == v.userID,
		"can_set_status":  v.canSetStatus(c, assignments),
	}
	if sys, ok := v.systems[c.SystemID]; ok {
		obj["system"] = systemToJSON(sys)
	}
	if owner := v.users[c.OwnerID]; owner != nil {
		obj["owner_user"] = userDisplayObject(owner)
	}
	return obj
}

var characterStatusOrder = map[models.CharacterStatus]int{models.CharacterAlive: 0, models.CharacterRetired: 1, models.CharacterDead: 2}

// sortCharacters orders alive first, then retired, then dead, each by name.
func sortCharacters(chars []models.Character) {
	sort.SliceStable(chars, func(a, b int) bool {
		sa, sb := characterStatusOrder[chars[a].Status], characterStatusOrder[chars[b].Status]
		if sa != sb {
			return sa < sb
		}
		return strings.ToLower(chars[a].Name) < strings.ToLower(chars[b].Name)
	})
}

// HandleCharacters handles /api/characters (GET list, POST create).
func HandleCharacters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleListCharacters(w, r)
	case http.MethodPost:
		handleCreateCharacter(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func handleListCharacters(w http.ResponseWriter, r *http.Request) {
	v, msg := loadCharacterViewer(r)
	if msg != "" {
		writeJSONError(w, http.StatusInternalServerError, msg)
		return
	}
	ownerID := r.URL.Query().Get("user")
	if ownerID == "" {
		ownerID = v.userID
	}
	owner := v.users[ownerID]
	if owner == nil {
		writeJSONError(w, http.StatusNotFound, "user not found")
		return
	}
	chars, err := db.GetCharactersByOwner(ownerID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve characters")
		return
	}
	sortCharacters(chars)
	ids := make([]string, 0, len(chars))
	for _, c := range chars {
		ids = append(ids, c.ID)
	}
	assignments, err := db.GetAssignmentsByCharacterIDs(ids)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve characters")
		return
	}
	list := make([]map[string]interface{}, 0, len(chars))
	for _, c := range chars {
		list = append(list, v.characterJSON(c, assignments[c.ID]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"owner_user": userDisplayObject(owner), "characters": list})
}

func handleCreateCharacter(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value("user_id").(string)
	var req characterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := req.normalize(); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	systems, err := systemsByID()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve systems")
		return
	}
	if _, ok := systems[req.SystemID]; !ok {
		writeJSONError(w, http.StatusBadRequest, "choose a known system")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	c := models.Character{
		ID: db.GenerateUUID(), OwnerID: userID, SystemID: req.SystemID, Name: req.Name,
		Description: req.Description, Background: req.Background, Status: req.Status,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateCharacter(c); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create character")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"status": "success", "message": "character created", "character_id": c.ID})
}

// HandleCharacterRoutes handles /api/characters/{id} (GET, PUT, DELETE) and /api/characters/{id}/status (PUT).
func HandleCharacterRoutes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/characters/"), "/")
	if parts[0] == "" || len(parts) > 2 || (len(parts) == 2 && parts[1] != "status") {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	id := parts[0]
	if len(parts) == 2 {
		if r.Method != http.MethodPut {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		handleCharacterStatus(w, r, id)
		return
	}
	switch r.Method {
	case http.MethodGet:
		handleGetCharacter(w, r, id)
	case http.MethodPut:
		handleUpdateCharacter(w, r, id)
	case http.MethodDelete:
		handleDeleteCharacter(w, r, id)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// loadCharacter loads the character and writes the error response when it is missing.
func loadCharacter(w http.ResponseWriter, id string) (*models.Character, bool) {
	c, err := db.GetCharacter(id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve character")
		return nil, false
	}
	if c == nil {
		writeJSONError(w, http.StatusNotFound, "character not found")
		return nil, false
	}
	return c, true
}

func handleGetCharacter(w http.ResponseWriter, r *http.Request, id string) {
	c, ok := loadCharacter(w, id)
	if !ok {
		return
	}
	v, msg := loadCharacterViewer(r)
	if msg != "" {
		writeJSONError(w, http.StatusInternalServerError, msg)
		return
	}
	assignments, err := db.GetAssignmentsByCharacterIDs([]string{id})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve character")
		return
	}
	writeJSON(w, http.StatusOK, v.characterJSON(*c, assignments[id]))
}

// activeAssignmentConflict is the 409 text for a character that is still playing somewhere.
func (v *characterViewer) activeAssignmentConflict(c models.Character, assignments []models.CharacterAssignment, tail string) string {
	for _, a := range assignments {
		if a.EndedAt == "" {
			camp := v.campaigns[a.CampaignID]
			if tail == "system" {
				return fmt.Sprintf("%s is playing in %s (%s). Remove it there first.", c.Name, camp.Title, camp.System)
			}
			return fmt.Sprintf("Unassign it from %s first.", camp.Title)
		}
	}
	return "character has an active assignment"
}

func handleUpdateCharacter(w http.ResponseWriter, r *http.Request, id string) {
	c, ok := loadCharacter(w, id)
	if !ok {
		return
	}
	v, msg := loadCharacterViewer(r)
	if msg != "" {
		writeJSONError(w, http.StatusInternalServerError, msg)
		return
	}
	if c.OwnerID != v.userID {
		writeJSONError(w, http.StatusForbidden, "only the owner can edit a character")
		return
	}
	var req characterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Status = c.Status // status changes go through /status so a concurrent change isn't overwritten
	if msg := req.normalize(); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	if _, ok := v.systems[req.SystemID]; !ok {
		writeJSONError(w, http.StatusBadRequest, "choose a known system")
		return
	}
	updated := *c
	updated.SystemID, updated.Name, updated.Description, updated.Background, updated.Status =
		req.SystemID, req.Name, req.Description, req.Background, req.Status
	updated.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := db.UpdateCharacter(updated); err != nil {
		switch {
		case errors.Is(err, db.ErrCharacterNotFound):
			writeJSONError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, db.ErrCharacterAssigned):
			assignments, _ := db.GetAssignmentsByCharacterIDs([]string{id})
			writeJSONError(w, http.StatusConflict, v.activeAssignmentConflict(*c, assignments[id], "system"))
		default:
			writeJSONError(w, http.StatusInternalServerError, "failed to update character")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "character updated"})
}

func handleDeleteCharacter(w http.ResponseWriter, r *http.Request, id string) {
	c, ok := loadCharacter(w, id)
	if !ok {
		return
	}
	v, msg := loadCharacterViewer(r)
	if msg != "" {
		writeJSONError(w, http.StatusInternalServerError, msg)
		return
	}
	if c.OwnerID != v.userID {
		writeJSONError(w, http.StatusForbidden, "only the owner can delete a character")
		return
	}
	if err := db.DeleteCharacter(id); err != nil {
		switch {
		case errors.Is(err, db.ErrCharacterNotFound):
			writeJSONError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, db.ErrCharacterAssigned):
			assignments, _ := db.GetAssignmentsByCharacterIDs([]string{id})
			writeJSONError(w, http.StatusConflict, v.activeAssignmentConflict(*c, assignments[id], "delete"))
		default:
			writeJSONError(w, http.StatusInternalServerError, "failed to delete character")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "character deleted"})
}

func handleCharacterStatus(w http.ResponseWriter, r *http.Request, id string) {
	c, ok := loadCharacter(w, id)
	if !ok {
		return
	}
	v, msg := loadCharacterViewer(r)
	if msg != "" {
		writeJSONError(w, http.StatusInternalServerError, msg)
		return
	}
	assignments, err := db.GetAssignmentsByCharacterIDs([]string{id})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve character")
		return
	}
	if !v.canSetStatus(*c, assignments[id]) {
		writeJSONError(w, http.StatusForbidden, "you cannot change this character's status")
		return
	}
	var req struct {
		Status models.CharacterStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !req.Status.IsValid() {
		writeJSONError(w, http.StatusBadRequest, "invalid status")
		return
	}
	if err := db.SetCharacterStatus(id, req.Status); err != nil {
		if errors.Is(err, db.ErrCharacterNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to update status")
		return
	}
	if c.OwnerID != v.userID && c.Status != req.Status {
		// Tell the owner which game's DM changed it; an admin's change belongs to no game.
		var campaign *models.Campaign
		for _, a := range assignments[id] {
			if camp, ok := v.campaigns[a.CampaignID]; ok && a.EndedAt == "" && camp.DM == v.userID {
				campaign = &camp
				break
			}
		}
		notify(v.userID, []string{c.OwnerID}, models.NotificationCharacterStatus, campaign, "", map[string]string{
			"character_id":   c.ID,
			"character_name": c.Name,
			"from_status":    string(c.Status),
			"to_status":      string(req.Status),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "success", "message": "status updated", "character_status": req.Status})
}

// HandleCampaignCharacter handles /api/campaigns/{id}/character (PUT assign or swap, DELETE clear)
// for the calling player. The DM never has a character.
func HandleCampaignCharacter(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/")
	if len(parts) != 2 || parts[1] != "character" {
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
	if !isCampaignPlayer(campaign, userID) || campaign.DM == userID {
		writeJSONError(w, http.StatusForbidden, "only players of this game can choose a character")
		return
	}
	byName, err := systemsByName()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve systems")
		return
	}
	systemID := campaignSystemID(byName, campaign)
	if systemID == "" && r.Method == http.MethodPut {
		writeJSONError(w, http.StatusConflict, "Characters need a known system. Ask an admin to add it.")
		return
	}

	if r.Method == http.MethodDelete {
		ended, err := db.EndCharacterAssignment(campaign.ID, userID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to clear character")
			return
		}
		if ended != nil {
			notify(userID, []string{campaign.DM}, models.NotificationCharacterCleared, campaign, "", map[string]string{
				"character_id":   ended.CharacterID,
				"character_name": ended.CharacterName,
			})
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "character cleared"})
		return
	}

	var req struct {
		CharacterID string `json:"character_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CharacterID == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	c, ok := loadCharacter(w, req.CharacterID)
	if !ok {
		return
	}
	if c.OwnerID != userID {
		writeJSONError(w, http.StatusForbidden, "you can only choose your own characters")
		return
	}
	if c.SystemID != systemID {
		writeJSONError(w, http.StatusConflict, "That character is in another system.")
		return
	}
	changed, err := db.AssignCharacter(campaign.ID, userID, c.ID, systemID)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrCharacterMismatch):
			writeJSONError(w, http.StatusConflict, "That character is in another system.")
		case errors.Is(err, db.ErrNotPlayer):
			writeJSONError(w, http.StatusForbidden, "only players of this game can choose a character")
		default:
			writeJSONError(w, http.StatusInternalServerError, "failed to choose character")
		}
		return
	}
	if changed {
		notify(userID, []string{campaign.DM}, models.NotificationCharacterAssigned, campaign, "", map[string]string{
			"character_id":   c.ID,
			"character_name": c.Name,
		})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "character chosen"})
}

// acceptedNotificationData marks a signup_accepted notification so the inbox can add
// "Choose your character" when the game's system is a known one.
func acceptedNotificationData(campaign *models.Campaign) map[string]string {
	byName, err := systemsByName()
	if err != nil || campaignSystemID(byName, campaign) == "" {
		return nil
	}
	return map[string]string{"characters": "1"}
}
