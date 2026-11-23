package api

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"waystone-web/models"
)

// discordURL builds an absolute link under PUBLIC_URL, or "" without it.
func discordURL(path string) string {
	base := discordConf().publicURL
	if base == "" {
		return ""
	}
	return base + path
}

// discordWhen renders a session start as Discord timestamps ("<t:U:F> (<t:U:R>)"). Without a
// parseable start (manual, empty time, unknown zone) it falls back to the date and time as text.
func discordWhen(d map[string]string) string {
	s := models.CampaignSession{Date: d["date"], Time: d["time"], Timezone: d["timezone"]}
	if start, ok := sessionStart(s); ok {
		u := strconv.FormatInt(start.Unix(), 10)
		return "<t:" + u + ":F> (<t:" + u + ":R>)"
	}
	return mdEscape(strings.TrimSpace(d["date"] + " " + d["time"]))
}

// fmtDay is "Fri, Dec 11" from YYYY-MM-DD, like fmtDay in static/notifications.js.
func fmtDay(ymd string) string {
	t, err := time.Parse("2006-01-02", ymd)
	if err != nil {
		return mdEscape(ymd)
	}
	return t.Format("Mon, Jan 2")
}

func dayList(dates string) string {
	if dates == "" {
		return ""
	}
	parts := strings.Split(dates, ",")
	for i, p := range parts {
		parts[i] = fmtDay(p)
	}
	return strings.Join(parts, " · ")
}

var mdReplacer = strings.NewReplacer(
	`\`, `\\`, "*", `\*`, "_", `\_`, "~", `\~`, "|", `\|`, ">", `\>`, "#", `\#`,
	"[", `\[`, "]", `\]`, "(", `\(`, ")", `\)`, "`", "\\`",
)

// mdEscape stops user text from turning into Discord markdown.
func mdEscape(s string) string { return mdReplacer.Replace(s) }

// discordText is the plain-text form of a notification for Discord. It mirrors message() in
// static/notifications.js case by case: keep the two in step.
func discordText(n models.Notification, actor string) string {
	d := n.Data
	if d == nil {
		d = map[string]string{}
	}
	count, err := strconv.Atoi(d["count"])
	if err != nil || count == 0 {
		count = 1
	}
	days := dayList(d["dates"])
	actor = mdEscape(actor)
	title := `"` + mdEscape(d["campaign_title"]) + `"`
	when := discordWhen(d)
	switch string(n.Type) {
	case "campaign_status":
		return title + " moved from " + mdEscape(d["from_status"]) + " to " + mdEscape(d["to_status"])
	case "signup_accepted":
		s := "Your sign-up for " + title + " was accepted"
		if d["characters"] != "" {
			s += ". Choose your character"
		}
		return s
	case "signup_declined":
		return "Your sign-up for " + title + " was declined"
	case "player_kicked":
		return "You were removed from " + title
	case "session_suggested":
		if d["added"] == "1" {
			if count == 1 {
				return actor + " added a date to the poll for " + title + ": " + days
			}
			return actor + " added " + strconv.Itoa(count) + " dates to the poll for " + title + ": " + days
		}
		if count > 1 {
			return actor + " suggested " + strconv.Itoa(count) + " dates for " + title + ": " + days
		}
		return actor + " suggested a session for " + title + " on " + when
	case "session_cancelled":
		if d["was"] == "Suggested" && count > 1 {
			return actor + " withdrew the poll for " + title
		}
		if d["was"] == "Suggested" {
			return actor + " withdrew the suggested session for " + title + " on " + when
		}
		return actor + " cancelled the session for " + title + " on " + when
	case "poll_reminder":
		if count > 1 {
			return "The poll for " + title + " still needs your reply. First date: " + fmtDay(d["date"])
		}
		return "The session suggested for " + title + " on " + when + " still needs your reply"
	case "poll_complete":
		if count > 1 {
			return "Everyone has replied to the poll for " + title
		}
		return "Everyone has replied to the session suggested for " + title + " on " + when
	case "poll_posted":
		return "Your repeating poll for " + title + " was posted: " + days
	case "poll_skipped":
		if d["reason"] == "cap" {
			return "No poll was posted for " + title + ": the game has reached its limit of 999 sessions"
		}
		if count == 1 {
			return "No poll was posted for " + title + ": " + days + " already has a session"
		}
		return "No poll was posted for " + title + ": " + days + " already have sessions"
	case "signup_created":
		return actor + " signed up for " + title
	case "player_left":
		return actor + " left " + title
	case "signup_withdrawn":
		return actor + " withdrew their sign-up for " + title
	case "campaign_full":
		players := "players"
		if d["player_count"] == "1" {
			players = "player"
		}
		return title + " reached " + mdEscape(d["player_count"]) + " " + players + " - sign-ups closed automatically"
	case "signups_opened":
		return "Sign-ups opened for " + title
	case "session_started":
		return "A session of " + title + " has begun"
	case "character_assigned":
		return actor + " now plays " + mdEscape(d["character_name"]) + " in " + title
	case "character_cleared":
		return actor + " removed their character " + mdEscape(d["character_name"]) + " from " + title
	case "character_status":
		return actor + " marked " + mdEscape(d["character_name"]) + " as " + mdEscape(d["to_status"])
	case "session_confirmed":
		return actor + " confirmed the session for " + title + " on " + when
	case "session_reminder":
		return "Reminder: the session of " + title + " starts " + when
	case "discord_channel_lost":
		return "The Discord channel of " + title + " is gone. Pick another in Admin › Discord"
	}
	return "Update for " + title
}

// discordButton is a message's link button; with an empty url the message carries none.
type discordButton struct{ label, url string }

// discordButtonFor builds the button for a path under PUBLIC_URL.
func discordButtonFor(label, path string) discordButton {
	return discordButton{label: label, url: discordURL(path)}
}

// discordPayload builds a message: the text, plus a link button (component type 2, style 5), which
// needs no gateway and no interactions endpoint. Mentions are off.
func discordPayload(text string, button discordButton) []byte {
	if r := []rune(text); len(r) > 2000 {
		text = string(r[:2000])
	}
	msg := map[string]interface{}{
		"content":          text,
		"allowed_mentions": map[string]interface{}{"parse": []string{}},
	}
	if button.url != "" {
		msg["components"] = []interface{}{map[string]interface{}{
			"type": 1,
			"components": []interface{}{map[string]interface{}{
				"type": 2, "style": 5, "label": button.label, "url": button.url,
			}},
		}}
	}
	b, _ := json.Marshal(msg)
	return b
}
