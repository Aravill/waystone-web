package config

import (
	"os"
	"strings"
	"time"
	"waystone-web/auth"
	"waystone-web/models"
)

const (
	DBPath      = "./data/waystone.db"
	DefaultPort = "8080"
)

// IsDevelopment reports whether ENVIRONMENT is "development" (case-insensitive).
// Only development seeds the demo data below: its users share a known password,
// so any other value - including unset - must never load it.
func IsDevelopment() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ENVIRONMENT")), "development")
}

const (
	sojournID = "ea213cb4-9ab9-49ff-a29f-3fafb7b4799c"
	testyID   = "7609bdaf-cc43-4131-b188-098aa07ba6dc"
	royID     = "2b6f0c1e-4d8a-4f3b-9e27-81c5a6d0f401"
	adamID    = "9d3e7a52-1f6c-4b80-a4d9-3c2e8f1b7a02"
	simonID   = "e5c1b8f3-7a2d-4e69-8b04-6f9d2c3a1e03"
	miraID    = "4a8e2c71-3b5d-4f9a-9c16-7e2b0d8f5a04"
	corvinID  = "b2d94f06-8e1a-4c7b-a35f-1c6e9a2d7b05"
	adminID   = "550e8400-e29b-41d4-a716-446655440000"

	arcanistID  = "6e05d2bc-efc8-4464-b3c9-c6fff907940f"
	lanternID   = "0c7a3e19-5d2b-4a8f-b641-9e3d2f7c1a06"
	saltmarshID = "d81f5b2a-4c9e-4e37-8a05-2b7c6e1d9f07"
	gooseID     = "73e6a0d4-1b8c-4f52-9d27-5a4c3b8e2f08"
	thornsID    = "f4b29c85-6a1d-4b03-8e7f-3d9a5c2b1e09"
)

// InitialPasswords are preset passwords for seeded users, keyed by user ID.
var InitialPasswords = map[string]string{
	testyID:  "ohgoditsacat",
	royID:    "ohgoditsacat",
	adamID:   "ohgoditsacat",
	simonID:  "ohgoditsacat",
	miraID:   "ohgoditsacat",
	corvinID: "ohgoditsacat",
}

var InitialSystems = []models.System{
	{ID: "3f1c5a52-6b0e-4f0b-9a57-1d2e8c7a0b11", Name: "D&D 5e"},
	{ID: "8a2d4e90-7c3b-4d1e-b6f4-5e9a1c0d2f22", Name: "D&D 2024"},
	{ID: "c4b7e1d3-2a98-4f65-8e0c-9b3d6a1f7e33", Name: "Cyberpunk Red"},
}

var InitialCampaigns = []models.Campaign{
	{
		ID:          sojournID,
		Title:       "Age of Sojourn",
		Status:      models.Ongoing,
		Type:        models.LongCampaign,
		System:      "D&D 5e",
		Summary:     "An epic campaign of discovery and adventure",
		Description: "A long-running D&D campaign where players explore a mysterious world filled with ancient ruins and untold secrets.",
		Players:     []string{testyID, royID, adamID, simonID},
		DM:          "550e8400-e29b-41d4-a716-446655440000",
		SignUpsOpen: true,
	},
	{
		ID:          arcanistID,
		Title:       "Embittered Arcanist",
		Status:      models.Pitch,
		Type:        models.OneShot,
		Summary:     "Figure out who turned the Archamge into a sheep",
		Description: "Lorem ipsum dolor sit amet something something bla bla bla",
		Players:     []string{adminID},
		DM:          testyID,
		SignUpsOpen: true,
	},
	{
		ID:                 lanternID,
		Title:              "The Lantern Road",
		Status:             models.Pitch,
		Type:               models.Adventure,
		System:             "Cyberpunk Red",
		Summary:            "Escort a convoy of lantern-bearers through the dead zone",
		Description:        "Four sessions on the road between two arcologies. Nobody has made the run in a decade.",
		Players:            []string{adamID},
		DM:                 royID,
		SignUpsOpen:        true,
		DesiredPlayerCount: "4",
	},
	{
		ID:          saltmarshID,
		Title:       "Saltmarsh Tides",
		Status:      models.OnHiatus,
		Type:        models.LongCampaign,
		System:      "D&D 2024",
		Summary:     "Smugglers, sea hags and a town that keeps its secrets",
		Description: "Resting until spring while the DM moves house.",
		Players:     []string{adminID, miraID, corvinID},
		DM:          simonID,
	},
	{
		ID:          gooseID,
		Title:       "Night at the Gilded Goose",
		Status:      models.Finished,
		Type:        models.OneShot,
		Summary:     "A murder at the inn, solved before the candles burned out",
		Description: "A one-night mystery. Played and done.",
		Players:     []string{royID, miraID, adminID},
		DM:          adamID,
	},
	{
		ID:          thornsID,
		Title:       "Thorns of Anevion",
		Status:      models.Cancelled,
		Type:        models.Adventure,
		Summary:     "A hedge-maze that grows back every night",
		Description: "Cancelled before the first session.",
		Players:     []string{},
		DM:          miraID,
	},
}

// InitialSignups are pending sign-ups awaiting the DM's decision.
var InitialSignups = []models.CampaignSignup{
	{ID: "a1c3e5f7-2b4d-4f6a-8c0e-1d3f5a7c9e10", CampaignID: sojournID, UserID: miraID, Status: models.SignupPending},
	{ID: "b2d4f6a8-3c5e-4a7b-9d1f-2e4a6b8d0f11", CampaignID: sojournID, UserID: corvinID, Status: models.SignupPending},
}

var InitialUsers = []models.User{
	{
		ID:        adminID,
		Username:  auth.AdminUsername(),
		Name:      "Michal Mozik",
		Nickname:  "Aravill",
		Roles:     []string{"admin", "user"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	{
		ID:        testyID,
		Username:  "testy",
		Name:      "Test Testington",
		Nickname:  "Testy",
		Roles:     []string{"user"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	seedUser(royID, "roy", "Roy Rogers", "Roy"),
	seedUser(adamID, "adam", "Adam Archer", "Adam"),
	seedUser(simonID, "simon", "Simon Sparrow", "Simon"),
	seedUser(miraID, "mira", "Mira Moss", "Mira"),
	seedUser(corvinID, "corvin", "Corvin Crane", ""),
}

func seedUser(id, username, name, nickname string) models.User {
	return models.User{
		ID:        id,
		Username:  username,
		Name:      name,
		Nickname:  nickname,
		Roles:     []string{"user"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// SeedSession is a seeded session plus its players' responses (user ID → participation).
type SeedSession struct {
	Session   models.CampaignSession
	Responses map[string]models.ParticipationStatus
}

// InitialSessions returns Age of Sojourn's weekly history (twelve past weeks,
// one of them cancelled), a confirmed session next week and a suggestion after it,
// plus an unanswered suggestion for Embittered Arcanist (the admin's only reply pending).
func InitialSessions() []SeedSession {
	all := []string{testyID, royID, adamID, simonID}
	responses := func(status models.ParticipationStatus, ids ...string) map[string]models.ParticipationStatus {
		m := map[string]models.ParticipationStatus{}
		for _, id := range ids {
			m[id] = status
		}
		return m
	}

	var seeded []SeedSession
	addOn := func(campaignID string, days int, status models.SessionStatus, r map[string]models.ParticipationStatus) {
		seeded = append(seeded, SeedSession{
			Session: models.CampaignSession{
				CampaignID: campaignID,
				Date:       time.Now().AddDate(0, 0, days).Format("2006-01-02"),
				Time:       "18:00",
				Duration:   240,
				Status:     status,
				Timezone:   "Europe/Prague",
			},
			Responses: r,
		})
	}
	add := func(weeks int, status models.SessionStatus, r map[string]models.ParticipationStatus) {
		addOn(sojournID, 7*weeks, status, r)
	}

	for weeks := -12; weeks <= -1; weeks++ {
		r := responses(models.Accepted, all...)
		status := models.SessionConfirmed
		switch weeks {
		case -9:
			r[simonID] = models.Declined
		case -7:
			status = models.SessionCancelled
			r[adamID], r[royID] = models.Declined, models.Declined
		case -3:
			r[adamID] = models.Tentative
		}
		add(weeks, status, r)
	}
	add(1, models.SessionConfirmed, responses(models.Accepted, royID, adamID, simonID))
	add(2, models.SessionSuggested, responses(models.Accepted, royID))
	addOn(arcanistID, 3, models.SessionSuggested, nil)
	return seeded
}
