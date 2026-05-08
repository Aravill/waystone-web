package db

import (
	"github.com/google/uuid"
)

// GenerateUUID generates a random UUID-like string for use as user IDs
func GenerateUUID() string {
	return uuid.New().String()
}

// AdminUserID is a well-known UUID for the pre-seeded admin user
const AdminUserID = "550e8400-e29b-41d4-a716-446655440000"
