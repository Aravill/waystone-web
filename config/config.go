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

var InitialUsers = []models.User{
	{
		ID:        "550e8400-e29b-41d4-a716-446655440000",
		Email:     "mozikmichal@gmail.com",
		Name:      "Admin",
		Roles:     []string{"admin"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	},
}
