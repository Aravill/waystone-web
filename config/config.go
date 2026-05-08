package config

import (
	"time"
	"waystone-web/models"
)

const (
	DBPath      = "./data/leveldb"
	DefaultPort = "8080"
)

var InitialEvents = []models.Event{
	{ID: 1, Name: "Age of Sojourn", Date: "2024-05-10"},
}

var InitialCampaigns = []models.Campaign{
	{
		ID:          1,
		Title:       "Age of Sojourn",
		Status:      models.Ongoing,
		Summary:     "An epic campaign of discovery and adventure",
		Description: "A long-running D&D campaign where players explore a mysterious world filled with ancient ruins and untold secrets.",
		Players:     []string{},
		DM:          "550e8400-e29b-41d4-a716-446655440000",
		SignUpsOpen: true,
	},
}

var InitialUsers = []models.User{
	{
		ID:        "550e8400-e29b-41d4-a716-446655440000",
		Email:     "mozikmichal@gmail.com",
		Name:      "Admin",
		Nickname:  "Michi",
		Roles:     []string{"admin"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
}
