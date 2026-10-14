package models

type SignupStatus string

const (
	SignupPending  SignupStatus = "Pending"
	SignupAccepted SignupStatus = "Accepted"
	SignupDeclined SignupStatus = "Declined"
	SignupKicked   SignupStatus = "Kicked"
)

type CampaignSignup struct {
	ID         string       `json:"id"`
	CampaignID string       `json:"campaign_id"`
	UserID     string       `json:"user_id"`
	Status     SignupStatus `json:"status"`
	CreatedAt  string       `json:"created_at"`
	UpdatedAt  string       `json:"updated_at"`
}
