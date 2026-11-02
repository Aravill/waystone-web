package models

// GuestToken is an expiring per-(campaign, user) link token.
type GuestToken struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"` // RFC3339 UTC
}
