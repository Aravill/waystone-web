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
// recipient who linked Discord and has the type's group on. The text is rendered once.
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
	payload := string(discordPayload(discordText(batch[0], actor), "", discordLink(batch[0])))
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
			Kind: "dm", Target: n.UserID, CampaignID: n.CampaignID, Payload: payload,
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
func postToChannel(campaign *models.Campaign, event, text, link string) {
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
	now := time.Now().UTC().Format(time.RFC3339)
	row := models.DiscordOutboxRow{
		Kind: "channel", Target: campaign.DiscordChannelID, CampaignID: campaign.ID,
		Payload:       string(discordPayload(text, "", link)),
		NextAttemptAt: now, CreatedAt: now,
	}
	if err := db.EnqueueDiscord([]models.DiscordOutboxRow{row}); err != nil {
		log.Printf("discord channel post: %v", err)
		return
	}
	wakeDiscord()
}

func gameLink(c *models.Campaign) string { return discordURL("/games/" + c.ID) }

func postSessionConfirmed(c *models.Campaign, s models.CampaignSession) {
	if s.Manual {
		return
	}
	postToChannel(c, models.DiscordEventSessionConfirmed,
		"**"+mdEscape(c.Title)+"**: session confirmed for "+discordWhen(sessionData(s)), gameLink(c))
}

func postSessionCancelled(c *models.Campaign, s models.CampaignSession) {
	if s.Manual {
		return
	}
	postToChannel(c, models.DiscordEventSessionCancelled,
		"**"+mdEscape(c.Title)+"**: the session on "+discordWhen(sessionData(s))+" was cancelled", gameLink(c))
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
	postToChannel(c, models.DiscordEventNewPoll, b.String(), discordURL("/games/"+c.ID+"/calendar?date="+dates[0].Date))
}

func postSessionReminder(c *models.Campaign, s models.CampaignSession) {
	postToChannel(c, models.DiscordEventSessionReminder,
		"**"+mdEscape(c.Title)+"**: next session "+discordWhen(sessionData(s)), gameLink(c))
}

func postSignupsOpened(c *models.Campaign) {
	postToChannel(c, models.DiscordEventSignupsOpened, "**"+mdEscape(c.Title)+"**: sign-ups are open", gameLink(c))
}

func postStatusChanged(c *models.Campaign, from, to models.CampaignStatus) {
	postToChannel(c, models.DiscordEventStatusChanged,
		"**"+mdEscape(c.Title)+"** moved from "+mdEscape(string(from))+" to "+mdEscape(string(to)), gameLink(c))
}

func sessionData(s models.CampaignSession) map[string]string {
	return map[string]string{"date": s.Date, "time": s.Time, "timezone": s.Timezone}
}
