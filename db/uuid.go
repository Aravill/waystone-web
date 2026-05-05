package db

import (
	"crypto/rand"
	"fmt"
)

// GenerateUUID generates a random UUID-like string for use as user IDs
func GenerateUUID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		panic(err) // Should not happen in practice
	}

	// Format as UUID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// AdminUserID is a well-known UUID for the pre-seeded admin user
const AdminUserID = "550e8400-e29b-41d4-a716-446655440000"
