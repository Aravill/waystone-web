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

func GetUserByID(id string) (*models.User, error) {
	return GetStore().GetUserByID(id)
}

func DeleteUser(id string) error {
	return GetStore().DeleteUser(id)
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

// DeleteUserAndCleanupCampaigns removes a user from all campaigns and then deletes the user
func DeleteUserAndCleanupCampaigns(userID string) (removedAsDM int, removedAsPlayer int, err error) {
	// Load all campaigns
	campaigns, err := GetAllCampaigns()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to load campaigns: %w", err)
	}

	// Clean up campaign references
	for i := range campaigns {
		campaign := &campaigns[i]
		changed := false

		if campaign.DM == userID {
			campaign.DM = ""
			removedAsDM++
			changed = true
		}

		// Remove from players list
		newPlayers := []string{}
		for _, playerID := range campaign.Players {
			if playerID != userID {
				newPlayers = append(newPlayers, playerID)
			} else {
				removedAsPlayer++
				changed = true
			}
		}
		campaign.Players = newPlayers

		// Save updated campaign only if it changed
		if changed {
			if err := SaveCampaign(*campaign); err != nil {
				return removedAsDM, removedAsPlayer, fmt.Errorf("failed to save campaign: %w", err)
			}
		}
	}

	// Delete the user record
	if err := DeleteUser(userID); err != nil {
		return removedAsDM, removedAsPlayer, fmt.Errorf("failed to delete user: %w", err)
	}

	return removedAsDM, removedAsPlayer, nil
}
