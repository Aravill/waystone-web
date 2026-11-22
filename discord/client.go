// Package discord is a tiny client for Discord's HTTP API (no gateway). It never imports db.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"waystone-web/config"
)

// Discord error codes the app reacts to.
const (
	CodeUnknownChannel = 10003
	CodeUnknownGuild   = 10004
	CodeUnknownMember  = 10007
	CodeMissingAccess  = 50001
	CodeCannotDM       = 50007
	CodeMissingPerms   = 50013
)

const (
	permAdministrator = 1 << 3
	permViewChannel   = 1 << 10
	permSendMessages  = 1 << 11
	permEmbedLinks    = 1 << 14
)

// Error is a non-2xx answer from Discord.
type Error struct {
	Status     int
	Code       int
	Message    string
	RetryAfter time.Duration // set on a 429 that was not retried
	Global     bool
}

func (e *Error) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("discord: %d (code %d): %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("discord: %d: %s", e.Status, e.Message)
}

// HasCode reports whether err is a Discord error with one of the codes.
func HasCode(err error, codes ...int) bool {
	var de *Error
	if !errors.As(err, &de) {
		return false
	}
	for _, c := range codes {
		if de.Code == c {
			return true
		}
	}
	return false
}

// IsStatus reports whether err is a Discord error with the HTTP status.
func IsStatus(err error, status int) bool {
	var de *Error
	return errors.As(err, &de) && de.Status == status
}

type User struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
}

type Overwrite struct {
	ID    string `json:"id"`
	Type  int    `json:"type"` // 0 role, 1 member
	Allow string `json:"allow"`
	Deny  string `json:"deny"`
}

type Channel struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Type       int         `json:"type"`
	Position   int         `json:"position"`
	Overwrites []Overwrite `json:"permission_overwrites"`
}

type Role struct {
	ID          string `json:"id"`
	Permissions string `json:"permissions"`
}

type Guild struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OwnerID string `json:"owner_id"`
	Roles   []Role `json:"roles"`
}

type Member struct {
	User  User     `json:"user"`
	Roles []string `json:"roles"`
}

// Client talks to Discord as the bot and honours its rate limits.
type Client struct {
	base, token string
	hc          *http.Client

	mu          sync.Mutex
	globalUntil time.Time
	routeUntil  map[string]time.Time
	botID       string
}

func New(base, token string) *Client {
	return &Client{
		base:       strings.TrimRight(base, "/"),
		token:      token,
		hc:         &http.Client{Timeout: 15 * time.Second},
		routeUntil: map[string]time.Time{},
	}
}

func userAgent() string {
	return "DiscordBot (https://github.com/Aravill/waystone-web, " + config.Version + ")"
}

// wait blocks until the global and route limits allow a request.
func (c *Client) wait(ctx context.Context, route string) error {
	for {
		c.mu.Lock()
		until := c.globalUntil
		if r := c.routeUntil[route]; r.After(until) {
			until = r
		}
		c.mu.Unlock()
		d := time.Until(until)
		if d <= 0 {
			return nil
		}
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (c *Client) hold(route string, d time.Duration, global bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	until := time.Now().Add(d)
	if global {
		c.globalUntil = until
	} else {
		c.routeUntil[route] = until
	}
}

// do sends a request. auth is the full Authorization header value. A 429 is retried once when
// the wait is at most five seconds, otherwise returned as an *Error with RetryAfter.
func (c *Client) do(ctx context.Context, method, path string, body []byte, contentType, auth string, out interface{}) error {
	route := method + " " + path
	for attempt := 0; ; attempt++ {
		if err := c.wait(ctx, route); err != nil {
			return err
		}
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", userAgent())
		req.Header.Set("Authorization", auth)
		if body != nil {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := c.hc.Do(req)
		if err != nil {
			return err
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()

		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			if f, err := strconv.ParseFloat(resp.Header.Get("X-RateLimit-Reset-After"), 64); err == nil && f > 0 {
				c.hold(route, time.Duration(f*float64(time.Second)), false)
			}
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			var rl struct {
				RetryAfter float64 `json:"retry_after"`
				Global     bool    `json:"global"`
			}
			_ = json.Unmarshal(data, &rl)
			if rl.RetryAfter <= 0 {
				rl.RetryAfter, _ = strconv.ParseFloat(resp.Header.Get("Retry-After"), 64)
			}
			if rl.RetryAfter <= 0 {
				rl.RetryAfter = 1
			}
			d := time.Duration(rl.RetryAfter * float64(time.Second))
			c.hold(route, d, rl.Global)
			if d <= 5*time.Second && attempt == 0 {
				continue
			}
			return &Error{Status: resp.StatusCode, Message: "rate limited", RetryAfter: d, Global: rl.Global}
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			var de struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			_ = json.Unmarshal(data, &de)
			if de.Message == "" {
				de.Message = http.StatusText(resp.StatusCode)
			}
			return &Error{Status: resp.StatusCode, Code: de.Code, Message: de.Message}
		}
		if out != nil && len(data) > 0 {
			return json.Unmarshal(data, out)
		}
		return nil
	}
}

func (c *Client) botGet(ctx context.Context, path string, out interface{}) error {
	return c.do(ctx, http.MethodGet, path, nil, "", "Bot "+c.token, out)
}

// BotUser returns the bot's own user; the ID is cached.
func (c *Client) BotUser(ctx context.Context) (*User, error) {
	var u User
	if err := c.botGet(ctx, "/users/@me", &u); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.botID = u.ID
	c.mu.Unlock()
	return &u, nil
}

func (c *Client) botUserID(ctx context.Context) (string, error) {
	c.mu.Lock()
	id := c.botID
	c.mu.Unlock()
	if id != "" {
		return id, nil
	}
	u, err := c.BotUser(ctx)
	if err != nil {
		return "", err
	}
	return u.ID, nil
}

func (c *Client) GetGuild(ctx context.Context, guildID string) (*Guild, error) {
	var g Guild
	if err := c.botGet(ctx, "/guilds/"+url.PathEscape(guildID), &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// GuildMember returns nil, nil when the user is not in the guild.
func (c *Client) GuildMember(ctx context.Context, guildID, userID string) (*Member, error) {
	var m Member
	err := c.botGet(ctx, "/guilds/"+url.PathEscape(guildID)+"/members/"+url.PathEscape(userID), &m)
	if err != nil {
		if IsStatus(err, http.StatusNotFound) || HasCode(err, CodeUnknownMember) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func parsePerm(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

// PostableChannels lists the guild's text and announcement channels where the bot can view the
// channel, send messages and embed links, ordered by position.
func (c *Client) PostableChannels(ctx context.Context, guildID string) ([]Channel, error) {
	botID, err := c.botUserID(ctx)
	if err != nil {
		return nil, err
	}
	g, err := c.GetGuild(ctx, guildID)
	if err != nil {
		return nil, err
	}
	m, err := c.GuildMember(ctx, guildID, botID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, &Error{Status: http.StatusNotFound, Code: CodeUnknownMember, Message: "the bot is not in that server"}
	}
	var channels []Channel
	if err := c.botGet(ctx, "/guilds/"+url.PathEscape(guildID)+"/channels", &channels); err != nil {
		return nil, err
	}

	botRoles := map[string]bool{}
	for _, r := range m.Roles {
		botRoles[r] = true
	}
	var base uint64
	for _, r := range g.Roles {
		if r.ID == g.ID || botRoles[r.ID] {
			base |= parsePerm(r.Permissions)
		}
	}
	all := g.OwnerID == botID || base&permAdministrator != 0

	need := uint64(permViewChannel | permSendMessages | permEmbedLinks)
	out := []Channel{}
	for _, ch := range channels {
		if ch.Type != 0 && ch.Type != 5 {
			continue
		}
		perms := base
		if !all {
			var everyone *Overwrite
			var roleAllow, roleDeny uint64
			var member *Overwrite
			for i := range ch.Overwrites {
				o := ch.Overwrites[i]
				switch {
				case o.Type == 0 && o.ID == g.ID:
					everyone = &ch.Overwrites[i]
				case o.Type == 0 && botRoles[o.ID]:
					roleAllow |= parsePerm(o.Allow)
					roleDeny |= parsePerm(o.Deny)
				case o.Type == 1 && o.ID == botID:
					member = &ch.Overwrites[i]
				}
			}
			if everyone != nil {
				perms = perms&^parsePerm(everyone.Deny) | parsePerm(everyone.Allow)
			}
			perms = perms&^roleDeny | roleAllow
			if member != nil {
				perms = perms&^parsePerm(member.Deny) | parsePerm(member.Allow)
			}
		}
		if all || perms&need == need {
			out = append(out, ch)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out, nil
}

// CreateDM opens (or fetches) the DM channel with a user.
func (c *Client) CreateDM(ctx context.Context, recipientID string) (string, error) {
	body, _ := json.Marshal(map[string]string{"recipient_id": recipientID})
	var ch Channel
	if err := c.do(ctx, http.MethodPost, "/users/@me/channels", body, "application/json", "Bot "+c.token, &ch); err != nil {
		return "", err
	}
	return ch.ID, nil
}

// SendMessage posts a JSON message payload to a channel.
func (c *Client) SendMessage(ctx context.Context, channelID string, payload []byte) error {
	return c.do(ctx, http.MethodPost, "/channels/"+url.PathEscape(channelID)+"/messages", payload, "application/json", "Bot "+c.token, nil)
}

// ExchangeCode trades an OAuth code for an access token.
func ExchangeCode(ctx context.Context, base, clientID, secret, code, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent())
	req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(secret))
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", &Error{Status: resp.StatusCode, Message: "token exchange failed"}
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &tok); err != nil || tok.AccessToken == "" {
		return "", errors.New("discord: no access token in answer")
	}
	return tok.AccessToken, nil
}

// OAuthUser returns the user an access token belongs to.
func OAuthUser(ctx context.Context, base, accessToken string) (*User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/users/@me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", userAgent())
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, &Error{Status: resp.StatusCode, Message: "could not read the Discord user"}
	}
	var u User
	if err := json.Unmarshal(data, &u); err != nil || u.ID == "" {
		return nil, errors.New("discord: no user in answer")
	}
	return &u, nil
}

// AuthorizeURL is where the user is sent to approve the identify scope.
func AuthorizeURL(clientID, redirectURI, state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("scope", "identify")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("prompt", "none")
	return "https://discord.com/oauth2/authorize?" + q.Encode()
}
