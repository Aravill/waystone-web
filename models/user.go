package models

import "time"

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
}

// DisplayName returns nickname, then name, then username.
func (u *User) DisplayName() string {
	if u.Nickname != "" {
		return u.Nickname
	}
	if u.Name != "" {
		return u.Name
	}
	return u.Username
}
