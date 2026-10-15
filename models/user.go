package models

import (
	"fmt"
	"time"
)

type User struct {
	ID                 string    `json:"id"`
	Username           string    `json:"username"`
	PasswordHash       string    `json:"-"`
	MustChangePassword bool      `json:"must_change_password"`
	Name               string    `json:"name"`
	Nickname           string    `json:"nickname"`
	Roles              []string  `json:"roles"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Blocked            bool      `json:"blocked"`
	Deleted            bool      `json:"deleted"`
	AvatarType         string    `json:"-"` // "" = no avatar
	AvatarVersion      int       `json:"-"`
}

// DisplayName returns nickname, then name, then username ("Deleted user" once anonymized).
func (u *User) DisplayName() string {
	if u.Deleted {
		return "Deleted user"
	}
	if u.Nickname != "" {
		return u.Nickname
	}
	if u.Name != "" {
		return u.Name
	}
	return u.Username
}

// AvatarURL returns "" when there is no avatar; ?v= busts the cache on replace.
func (u *User) AvatarURL() string {
	if u.AvatarType == "" {
		return ""
	}
	return fmt.Sprintf("/api/users/%s/avatar?v=%d", u.ID, u.AvatarVersion)
}
