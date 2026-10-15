package models

type SessionStatus string

const (
	SessionSuggested SessionStatus = "Suggested"
	SessionConfirmed SessionStatus = "Confirmed"
	SessionCancelled SessionStatus = "Cancelled"
)

// UnansweredSessions summarises upcoming suggested sessions a user has not responded to in one campaign.
type UnansweredSessions struct {
	Count     int
	FirstDate string
}

type CampaignSession struct {
	ID                       string              `json:"id"`
	CampaignID               string              `json:"campaign_id"`
	Date                     string              `json:"date"`     // YYYY-MM-DD format
	Time                     string              `json:"time"`     // HH:MM format
	Duration                 int                 `json:"duration"` // minutes
	Status                   SessionStatus       `json:"status"`
	Timezone                 string              `json:"timezone"` // IANA name; empty for legacy sessions
	AcceptedCount            int                 `json:"accepted_count,omitempty"`
	DeclinedCount            int                 `json:"declined_count,omitempty"`
	TentativeCount           int                 `json:"tentative_count,omitempty"`
	PendingCount             int                 `json:"pending_count,omitempty"`
	CurrentUserParticipation ParticipationStatus `json:"current_user_participation,omitempty"`
	CreatedAt                string              `json:"created_at"`
	UpdatedAt                string              `json:"updated_at"`
}
