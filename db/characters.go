package db

import (
	"time"
	"waystone-web/models"
)

func CreateCharacter(c models.Character) error { return GetStore().CreateCharacter(c) }

func GetCharacter(id string) (*models.Character, error) { return GetStore().GetCharacter(id) }

func GetCharactersByOwner(ownerID string) ([]models.Character, error) {
	return GetStore().GetCharactersByOwner(ownerID)
}

func UpdateCharacter(c models.Character) error { return GetStore().UpdateCharacter(c) }

func SetCharacterStatus(id string, status models.CharacterStatus) error {
	return GetStore().SetCharacterStatus(id, status)
}

func DeleteCharacter(id string) error { return GetStore().DeleteCharacter(id) }

func GetAssignmentsByCharacterIDs(ids []string) (map[string][]models.CharacterAssignment, error) {
	return GetStore().GetAssignmentsByCharacterIDs(ids)
}

func GetCampaignAssignments(campaignID string) ([]models.CharacterAssignment, error) {
	return GetStore().GetCampaignAssignments(campaignID)
}

func GetActiveAssignmentsByUser(userID string) (map[string]models.CharacterAssignment, error) {
	return GetStore().GetActiveAssignmentsByUser(userID)
}

// AssignCharacter makes characterID the user's active character in the game (ending any other).
// It reports whether anything changed.
func AssignCharacter(campaignID, userID, characterID string) (bool, error) {
	return GetStore().AssignCharacter(campaignID, userID, characterID, time.Now().UTC().Format(time.RFC3339))
}

// EndCharacterAssignment clears the user's active character in the game and returns it (nil if none).
func EndCharacterAssignment(campaignID, userID string) (*models.CharacterAssignment, error) {
	return GetStore().EndCharacterAssignment(campaignID, userID, time.Now().UTC().Format(time.RFC3339))
}

func CountCharactersBySystem(systemID string) (int, error) {
	return GetStore().CountCharactersBySystem(systemID)
}
