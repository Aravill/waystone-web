package models

import "fmt"

// System is a known game system (e.g. "D&D 5e") with an optional logo.
type System struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	LogoType    string `json:"-"` // "" = no logo
	LogoVersion int    `json:"-"`
}

// LogoURL returns "" when there is no logo; ?v= busts the cache on replace.
func (s System) LogoURL() string {
	if s.LogoType == "" {
		return ""
	}
	return fmt.Sprintf("/api/systems/%s/logo?v=%d", s.ID, s.LogoVersion)
}
