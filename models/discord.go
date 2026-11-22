package models

// DiscordLink ties a Waystone user to one Discord account, with the DM preferences.
type DiscordLink struct {
	UserID          string
	DiscordUserID   string
	DiscordUsername string
	DMChannelID     string
	DMSessions      bool
	DMSignups       bool
	DMCharacters    bool
	DMError         string
	DMErrorAt       string
	LinkedAt        string
}

// DiscordOutboxRow is one queued Discord message ("dm" to a user or "channel" to a channel).
type DiscordOutboxRow struct {
	ID            int64
	Kind          string
	Target        string
	CampaignID    string
	Payload       string
	Attempts      int
	NextAttemptAt string
	LastError     string
	Status        string
	CreatedAt     string
}

// DiscordSettings are the admin-managed settings kept in app_settings.
type DiscordSettings struct {
	GuildID   string
	InviteURL string
	Events    []string // channel events that are on
}

const (
	DiscordEventSessionConfirmed = "session_confirmed"
	DiscordEventSessionCancelled = "session_cancelled"
	DiscordEventNewPoll          = "new_poll"
	DiscordEventSessionReminder  = "session_reminder"
	DiscordEventSignupsOpened    = "signups_opened"
	DiscordEventStatusChanged    = "status_changed"
)

// DiscordEvents lists every channel event with its label, in display order.
var DiscordEvents = []struct{ Key, Label string }{
	{DiscordEventSessionConfirmed, "Session confirmed"},
	{DiscordEventSessionCancelled, "Session cancelled"},
	{DiscordEventNewPoll, "New poll posted"},
	{DiscordEventSessionReminder, "Session reminder"},
	{DiscordEventSignupsOpened, "Signups opened"},
	{DiscordEventStatusChanged, "Game status changed"},
}
