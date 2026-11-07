package models

// CharacterStatus is only a label: it never ends assignments or limits picking.
type CharacterStatus string

const (
	CharacterAlive   CharacterStatus = "alive"
	CharacterRetired CharacterStatus = "retired"
	CharacterDead    CharacterStatus = "dead"
)

// IsValid reports whether s is a known character status.
func (s CharacterStatus) IsValid() bool {
	switch s {
	case CharacterAlive, CharacterRetired, CharacterDead:
		return true
	}
	return false
}

// Character is a player's character: a name, a description and a background.
type Character struct {
	ID          string          `json:"id"`
	OwnerID     string          `json:"owner_id"`
	SystemID    string          `json:"system_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Background  string          `json:"background"`
	Status      CharacterStatus `json:"status"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// CharacterAssignment puts a character in a game. EndedAt "" means active; history is kept.
type CharacterAssignment struct {
	ID              string          `json:"id"`
	CharacterID     string          `json:"character_id"`
	CampaignID      string          `json:"campaign_id"`
	UserID          string          `json:"user_id"`
	StartedAt       string          `json:"started_at"`
	EndedAt         string          `json:"ended_at"`
	CharacterName   string          `json:"character_name"`   // filled by joined queries
	CharacterStatus CharacterStatus `json:"character_status"` // filled by joined queries
}
