package models

type NotificationType string

const (
	NotificationCampaignStatus   NotificationType = "campaign_status"
	NotificationSignupAccepted   NotificationType = "signup_accepted"
	NotificationSignupDeclined   NotificationType = "signup_declined"
	NotificationPlayerKicked     NotificationType = "player_kicked"
	NotificationSessionSuggested NotificationType = "session_suggested"
	NotificationSessionConfirmed NotificationType = "session_confirmed"
	NotificationSessionCancelled NotificationType = "session_cancelled"
	NotificationSignupCreated    NotificationType = "signup_created"
	NotificationCampaignFull     NotificationType = "campaign_full"
	NotificationPlayerLeft       NotificationType = "player_left"
	NotificationSignupWithdrawn  NotificationType = "signup_withdrawn"
)

type Notification struct {
	ID         string            `json:"id"`
	UserID     string            `json:"user_id"`
	Type       NotificationType  `json:"type"`
	CampaignID string            `json:"campaign_id"`
	SessionID  string            `json:"session_id"`
	ActorID    string            `json:"actor_id"`
	Data       map[string]string `json:"data"`
	CreatedAt  string            `json:"created_at"`
	ReadAt     string            `json:"read_at"`
}
