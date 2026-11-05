package db

import "time"

// IssuePasswordInvite clears the user's password and stores the invite's token hash,
// replacing any earlier invite.
func IssuePasswordInvite(userID, tokenHash string, expiresAt time.Time) error {
	return GetStore().IssuePasswordInvite(userID, tokenHash, expiresAt)
}

// GetPasswordInvite resolves an invite by token hash; userID is empty when unknown.
func GetPasswordInvite(tokenHash string) (string, time.Time, error) {
	return GetStore().GetPasswordInvite(tokenHash)
}

// RedeemPasswordInvite atomically sets the password and consumes the invite. The
// returned user ID is empty when the invite is unknown, expired or its user is
// deleted or blocked.
func RedeemPasswordInvite(tokenHash, passwordHash string) (string, error) {
	return GetStore().RedeemPasswordInvite(tokenHash, passwordHash)
}

func DeletePasswordInvite(userID string) error {
	return GetStore().DeletePasswordInvite(userID)
}
