package models

type ParticipationStatus string

const (
	Accepted  ParticipationStatus = "Accepted"
	Declined  ParticipationStatus = "Declined"
	Tentative ParticipationStatus = "Tentative"
)

type SessionResponse struct {
	ID            string              `json:"id"`
	SessionID     string              `json:"session_id"`
	UserID        string              `json:"user_id"`
	Participation ParticipationStatus `json:"participation"`
	CreatedAt     string              `json:"created_at"`
	UpdatedAt     string              `json:"updated_at"`
}
