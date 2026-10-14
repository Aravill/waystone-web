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

var InitialSystems = []models.System{
	{ID: "3f1c5a52-6b0e-4f0b-9a57-1d2e8c7a0b11", Name: "D&D 5e"},
	{ID: "8a2d4e90-7c3b-4d1e-b6f4-5e9a1c0d2f22", Name: "D&D 2024"},
	{ID: "c4b7e1d3-2a98-4f65-8e0c-9b3d6a1f7e33", Name: "Cyberpunk Red"},
}

var InitialCampaigns = []models.Campaign{
	{
		ID:          "ea213cb4-9ab9-49ff-a29f-3fafb7b4799c",
		Title:       "Age of Sojourn",
		Status:      models.Ongoing,
		Type:        models.LongCampaign,
		System:      "D&D 5e",
		Summary:     "An epic campaign of discovery and adventure",
		Description: "A long-running D&D campaign where players explore a mysterious world filled with ancient ruins and untold secrets.",
		Players:     []string{"7609bdaf-cc43-4131-b188-098aa07ba6dc"},
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
		ID:        "7609bdaf-cc43-4131-b188-098aa07ba6dc",
		Username:  "testy",
		Name:      "Test Testington",
		Nickname:  "Testy",
		Roles:     []string{"user"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
}
