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

// DiscordGameEvents are a game's Discord event settings (campaigns columns, admin-only).
type DiscordGameEvents struct {
	CampaignID string
	On         bool
	VoiceID    string
	VoiceName  string
	Cover      string // file name in UPLOAD_DIR, or ""
}

// DiscordEvent is the Discord scheduled event a session got. Status is the state the app moved it
// to: scheduled, active, completed or cancelled.
type DiscordEvent struct {
	SessionID  string
	CampaignID string
	GuildID    string
	EventID    string
	Status     string
}

const (
	DiscordEventScheduled = "scheduled"
	DiscordEventActive    = "active"
	DiscordEventCompleted = "completed"
	DiscordEventCancelled = "cancelled"
)

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
