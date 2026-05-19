package models

type SessionStatus string

const (
	SessionSuggested SessionStatus = "Suggested"
	SessionConfirmed SessionStatus = "Confirmed"
	SessionCancelled SessionStatus = "Cancelled"
)

type CampaignSession struct {
	ID                       string              `json:"id"`
	CampaignID               string              `json:"campaign_id"`
	Date                     string              `json:"date"`     // YYYY-MM-DD format
	Time                     string              `json:"time"`     // HH:MM format
	Duration                 int                 `json:"duration"` // minutes
	Status                   SessionStatus       `json:"status"`
	AcceptedCount            int                 `json:"accepted_count,omitempty"`
	DeclinedCount            int                 `json:"declined_count,omitempty"`
	TentativeCount           int                 `json:"tentative_count,omitempty"`
	PendingCount             int                 `json:"pending_count,omitempty"`
	CurrentUserParticipation ParticipationStatus `json:"current_user_participation,omitempty"`
	CreatedAt                string              `json:"created_at"`
	UpdatedAt                string              `json:"updated_at"`
}
