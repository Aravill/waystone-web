package db

import (
	"fmt"
	"time"
	"waystone-web/models"
)

func SaveUser(user models.User) error {
	now := time.Now()

	if user.ID == "" {
		user.ID = GenerateUUID()
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = user.CreatedAt
	} else {
		user.UpdatedAt = now
	}

	// Initialize empty roles if not set
	if user.Roles == nil {
		user.Roles = []string{}
	}

	return GetStore().SaveUser(user)
}

func GetUserByID(id string) (*models.User, error) {
	return GetStore().GetUserByID(id)
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

// GetUserByUsername retrieves a user by username (case-insensitive)
func GetUserByUsername(username string) (*models.User, error) {
	return GetStore().GetUserByUsername(username)
}

// AnonymizeUserAndCleanupCampaigns removes a user from all campaigns' player lists
// (DM references are kept) and then anonymizes the user record.
func AnonymizeUserAndCleanupCampaigns(userID string) (removedAsPlayer int, err error) {
	campaigns, err := GetAllCampaigns()
	if err != nil {
		return 0, fmt.Errorf("failed to load campaigns: %w", err)
	}

	for i := range campaigns {
		campaign := &campaigns[i]
		newPlayers := []string{}
		changed := false
		for _, playerID := range campaign.Players {
			if playerID != userID {
				newPlayers = append(newPlayers, playerID)
			} else {
				removedAsPlayer++
				changed = true
			}
		}
		if !changed {
			continue
		}
		campaign.Players = newPlayers
		if err := SaveCampaign(*campaign); err != nil {
			return removedAsPlayer, fmt.Errorf("failed to save campaign: %w", err)
		}
	}

	if err := GetStore().AnonymizeUser(userID); err != nil {
		return removedAsPlayer, fmt.Errorf("failed to anonymize user: %w", err)
	}

	return removedAsPlayer, nil
}

// SetUserProfile updates the provided name and/or nickname (nil fields are left unchanged)
func SetUserProfile(id string, name, nickname *string) error {
	return GetStore().SetUserProfile(id, name, nickname)
}

// SetUserAvatar stores a user's avatar; empty data removes it
func SetUserAvatar(id string, data []byte, contentType string) error {
	return GetStore().SetUserAvatar(id, data, contentType)
}

// GetUserAvatar returns avatar bytes and content type; empty data means no avatar
func GetUserAvatar(id string) ([]byte, string, error) {
	return GetStore().GetUserAvatar(id)
}

// SetUserBlocked updates the blocked status of a user
func SetUserBlocked(id string, blocked bool) error {
	return GetStore().SetUserBlocked(id, blocked)
}

// SetUserPassword stores a new password hash and the must-change flag
func SetUserPassword(id, hash string, mustChange bool) error {
	return GetStore().SetUserPassword(id, hash, mustChange)
}
