package config

import (
	"time"
	"waystone-web/models"
)

const (
	DBPath      = "./data/leveldb"
	DefaultPort = "8080"
)

var InitialCampaigns = []models.Campaign{
	{
		ID:          "ea213cb4-9ab9-49ff-a29f-3fafb7b4799c",
		Title:       "Age of Sojourn",
		Status:      models.Ongoing,
		Summary:     "An epic campaign of discovery and adventure",
		Description: "A long-running D&D campaign where players explore a mysterious world filled with ancient ruins and untold secrets.",
		Players:     []string{},
		DM:          "550e8400-e29b-41d4-a716-446655440000",
		SignUpsOpen: true,
	},
	{
		ID:          "ea213cb4-9ab9-49ff-a29f-3fafb7b4799c",
		Title:       "Embittered Arcanist",
		Status:      models.Pitch,
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
		Email:     "mozikmichal@gmail.com",
		Name:      "Michal Mozik",
		Nickname:  "Aravill",
		Roles:     []string{"admin", "user"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
	{
		ID:        "7609bdaf-cc43-4131-b188-098aa07ba6dc",
		Email:     "test.user@gmail.com",
		Name:      "Test Testington",
		Nickname:  "Testy",
		Roles:     []string{"user"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
}
