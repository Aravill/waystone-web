package config

import (
	"time"
	"waystone-web/auth"
	"waystone-web/models"
)

const (
	DBPath      = "./data/waystone.db"
	DefaultPort = "8080"
)

const (
	sojournID = "ea213cb4-9ab9-49ff-a29f-3fafb7b4799c"
	testyID   = "7609bdaf-cc43-4131-b188-098aa07ba6dc"
	royID     = "2b6f0c1e-4d8a-4f3b-9e27-81c5a6d0f401"
	adamID    = "9d3e7a52-1f6c-4b80-a4d9-3c2e8f1b7a02"
	simonID   = "e5c1b8f3-7a2d-4e69-8b04-6f9d2c3a1e03"
)

// InitialPasswords are preset passwords for the demo users, keyed by user ID.
// Demo data (users, campaigns, sessions) is seeded only with SEED_DEMO_DATA on.
var InitialPasswords = map[string]string{
	testyID: "ohgoditsacat",
	royID:   "ohgoditsacat",
	adamID:  "ohgoditsacat",
	simonID: "ohgoditsacat",
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
		ID:          "6e05d2bc-efc8-4464-b3c9-c6fff907940f",
		Title:       "Embittered Arcanist",
		Status:      models.Pitch,
		Type:        models.OneShot,
		Summary:     "Figure out who turned the Archamge into a sheep",
		Description: "Lorem ipsum dolor sit amet something something bla bla bla",
		Players:     []string{"550e8400-e29b-41d4-a716-446655440000"},
		DM:          "7609bdaf-cc43-4131-b188-098aa07ba6dc",
		SignUpsOpen: true,
	},
}

var InitialUsers = []models.User{
	{
		ID:        "550e8400-e29b-41d4-a716-446655440000",
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
// one of them cancelled), a confirmed session next week and a suggestion after it.
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
	add := func(weeks int, status models.SessionStatus, r map[string]models.ParticipationStatus) {
		seeded = append(seeded, SeedSession{
			Session: models.CampaignSession{
				CampaignID: sojournID,
				Date:       time.Now().AddDate(0, 0, 7*weeks).Format("2006-01-02"),
				Time:       "18:00",
				Duration:   240,
				Status:     status,
				Timezone:   "Europe/Prague",
			},
			Responses: r,
		})
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
	return seeded
}
