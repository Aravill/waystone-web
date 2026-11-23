package api

import (
	"log"
	"os"
	"strings"
	"sync"
	"time"
	"waystone-web/db"
	"waystone-web/discord"
	"waystone-web/models"
)

// Discord is optional. It is on when DISCORD_BOT_TOKEN is set; linking accounts also needs
// DISCORD_CLIENT_ID, DISCORD_CLIENT_SECRET and PUBLIC_URL.
type discordConfig struct {
	token, clientID, clientSecret string
	publicURL                     string // no trailing slash
	apiBase                       string
	client                        *discord.Client
}

var (
	discordConfOnce sync.Once
	discordConfVal  discordConfig
)

func discordConf() *discordConfig {
	discordConfOnce.Do(func() {
		c := discordConfig{
			token:        strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
			clientID:     strings.TrimSpace(os.Getenv("DISCORD_CLIENT_ID")),
			clientSecret: strings.TrimSpace(os.Getenv("DISCORD_CLIENT_SECRET")),
			publicURL:    strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_URL")), "/"),
			apiBase:      strings.TrimRight(strings.TrimSpace(os.Getenv("DISCORD_API_BASE")), "/"),
		}
		if c.apiBase == "" {
			c.apiBase = "https://discord.com/api/v10"
		}
		if c.publicURL != "" && !strings.HasPrefix(c.publicURL, "http://") && !strings.HasPrefix(c.publicURL, "https://") {
			log.Printf("PUBLIC_URL %q does not start with http:// or https://; Discord links are off", c.publicURL)
			c.publicURL = ""
		}
		if c.token != "" {
			c.client = discord.New(c.apiBase, c.token)
		}
		discordConfVal = c
	})
	return &discordConfVal
}

func discordEnabled() bool { return discordConf().token != "" }

func discordCanLink() bool {
	c := discordConf()
	return c.token != "" && c.clientID != "" && c.clientSecret != "" && c.publicURL != ""
}

// discordWake nudges the outbox sender after rows were queued.
var discordWake = make(chan struct{}, 1)

func wakeDiscord() {
	select {
	case discordWake <- struct{}{}:
	default:
	}
}

// discordGroup maps a notification type to the DM toggle that governs it: "sessions", "signups",
// "characters", "always", or "" for types that are never sent as DMs.
func discordGroup(typ models.NotificationType) string {
	switch typ {
	case models.NotificationSessionSuggested, models.NotificationSessionConfirmed, models.NotificationSessionCancelled,
		models.NotificationSessionStarted, models.NotificationSessionReminder, models.NotificationPollReminder,
		models.NotificationPollComplete, models.NotificationPollPosted, models.NotificationPollSkipped:
		return "sessions"
	case models.NotificationSignupCreated, models.NotificationSignupAccepted, models.NotificationSignupDeclined,
		models.NotificationSignupWithdrawn, models.NotificationPlayerKicked, models.NotificationPlayerLeft,
		models.NotificationCampaignFull, models.NotificationSignupsOpened, models.NotificationCampaignStatus:
		return "signups"
	case models.NotificationCharacterStatus, models.NotificationCharacterAssigned, models.NotificationCharacterCleared:
		return "characters"
	case models.NotificationDiscordChannelLost:
		return "always"
	}
	return ""
}

// queueDiscordDMs mirrors a just-written batch of inbox notifications into the outbox, one DM per
// recipient who linked Discord and has the type's group on. The text is rendered once; the link is
// each recipient's own, so opening it marks their notification read.
func queueDiscordDMs(actorID string, typ models.NotificationType, batch []models.Notification) {
	group := discordGroup(typ)
	if !discordEnabled() || group == "" || len(batch) == 0 {
		return
	}
	ids := make([]string, 0, len(batch))
	for _, n := range batch {
		ids = append(ids, n.UserID)
	}
	links, err := db.GetDiscordLinksForUsers(ids)
	if err != nil {
		log.Printf("discord dm %s: %v", typ, err)
		return
	}
	if len(links) == 0 {
		return
	}
	actor := "Someone"
	if actorID != "" {
		if u, err := db.GetUserByID(actorID); err == nil && u != nil {
			actor = u.DisplayName()
		}
	}
	text := discordText(batch[0], actor)
	_, label := notificationTarget(batch[0])
	now := time.Now().UTC().Format(time.RFC3339)
	var rows []models.DiscordOutboxRow
	for _, n := range batch {
		l, ok := links[n.UserID]
		if !ok {
			continue
		}
		on := group == "always" ||
			(group == "sessions" && l.DMSessions) ||
			(group == "signups" && l.DMSignups) ||
			(group == "characters" && l.DMCharacters)
		if !on {
			continue
		}
		rows = append(rows, models.DiscordOutboxRow{
			Kind: "dm", Target: n.UserID, CampaignID: n.CampaignID,
			Payload:       string(discordPayload(text, discordButtonFor(label, notificationURL(n)))),
			NextAttemptAt: now, CreatedAt: now,
		})
	}
	if len(rows) == 0 {
		return
	}
	if err := db.EnqueueDiscord(rows); err != nil {
		log.Printf("discord dm %s: %v", typ, err)
		return
	}
	wakeDiscord()
}

// postToChannel queues one post to the game's Discord channel. It skips games without a channel,
// events the admin switched off, and (but for the status change itself) finished or cancelled games.
// n is the inbox notification the post stands for: it decides the link (notificationTarget).
func postToChannel(campaign *models.Campaign, event, text string, n models.Notification) {
	if !discordEnabled() || campaign == nil || campaign.DiscordChannelID == "" {
		return
	}
	if (campaign.Status == models.Finished || campaign.Status == models.Cancelled) && event != models.DiscordEventStatusChanged {
		return
	}
	settings, err := db.GetDiscordSettings()
	if err != nil {
		log.Printf("discord channel post: %v", err)
		return
	}
	on := false
	for _, e := range settings.Events {
		if e == event {
			on = true
			break
		}
	}
	if !on {
		return
	}
	n.CampaignID = campaign.ID
	path, label := notificationTarget(n)
	now := time.Now().UTC().Format(time.RFC3339)
	row := models.DiscordOutboxRow{
		Kind: "channel", Target: campaign.DiscordChannelID, CampaignID: campaign.ID,
		Payload:       string(discordPayload(text, discordButtonFor(label, path))),
		NextAttemptAt: now, CreatedAt: now,
	}
	if err := db.EnqueueDiscord([]models.DiscordOutboxRow{row}); err != nil {
		log.Printf("discord channel post: %v", err)
		return
	}
	wakeDiscord()
}

// sessionNotification is the notification of a session event that a channel post stands for.
func sessionNotification(typ models.NotificationType, s models.CampaignSession) models.Notification {
	return models.Notification{Type: typ, SessionID: s.ID, Data: sessionData(s)}
}

func postSessionConfirmed(c *models.Campaign, s models.CampaignSession) {
	if s.Manual {
		return
	}
	postToChannel(c, models.DiscordEventSessionConfirmed,
		"**"+mdEscape(c.Title)+"**: session confirmed for "+discordWhen(sessionData(s)),
		sessionNotification(models.NotificationSessionConfirmed, s))
}

func postSessionCancelled(c *models.Campaign, s models.CampaignSession) {
	if s.Manual {
		return
	}
	postToChannel(c, models.DiscordEventSessionCancelled,
		"**"+mdEscape(c.Title)+"**: the session on "+discordWhen(sessionData(s))+" was cancelled",
		sessionNotification(models.NotificationSessionCancelled, s))
}

func postNewPoll(c *models.Campaign, dates []models.CampaignSession) {
	if len(dates) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString("**" + mdEscape(c.Title) + "**: new poll. Reply in Waystone:")
	for _, d := range dates {
		b.WriteString("\n• " + discordWhen(sessionData(d)))
	}
	postToChannel(c, models.DiscordEventNewPoll, b.String(),
		sessionNotification(models.NotificationSessionSuggested, dates[0]))
}

func postSessionReminder(c *models.Campaign, s models.CampaignSession) {
	postToChannel(c, models.DiscordEventSessionReminder,
		"**"+mdEscape(c.Title)+"**: next session "+discordWhen(sessionData(s)),
		sessionNotification(models.NotificationSessionReminder, s))
}

func postSignupsOpened(c *models.Campaign) {
	postToChannel(c, models.DiscordEventSignupsOpened, "**"+mdEscape(c.Title)+"**: sign-ups are open",
		models.Notification{Type: models.NotificationSignupsOpened})
}

func postStatusChanged(c *models.Campaign, from, to models.CampaignStatus) {
	postToChannel(c, models.DiscordEventStatusChanged,
		"**"+mdEscape(c.Title)+"** moved from "+mdEscape(string(from))+" to "+mdEscape(string(to)),
		models.Notification{Type: models.NotificationCampaignStatus})
}

func sessionData(s models.CampaignSession) map[string]string {
	return map[string]string{"date": s.Date, "time": s.Time, "timezone": s.Timezone, "poll_id": s.PollID}
}
