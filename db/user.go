package db

import (
	"fmt"
	"waystone-web/models"
	"time"
)

func SaveUser(user models.User) error {
	if user.ID == "" {
		user.ID = GenerateUUID()
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}
	user.UpdatedAt = time.Now()

	// Initialize empty roles if not set
	if user.Roles == nil {
		user.Roles = []string{}
	}

	return GetStore().SaveUser(user)
}

func GetUserByGoogleID(googleID string) (*models.User, error) {
	return GetStore().GetUserByGoogleID(googleID)
}

func GetAllUsers() ([]models.User, error) {
	return GetStore().GetAllUsers()
}

// UpdateUserRoles updates the roles for a user
func UpdateUserRoles(userID string, roles []string) error {
	users, err := GetAllUsers()
	if err != nil {
		return err
	}

	for i := range users {
		if users[i].ID == userID {
			users[i].Roles = roles
			return SaveUser(users[i])
		}
	}

	return fmt.Errorf("user not found")
}

// GetUserByEmail retrieves a user by email address
func GetUserByEmail(email string) (*models.User, error) {
	return GetStore().GetUserByEmail(email)
}
