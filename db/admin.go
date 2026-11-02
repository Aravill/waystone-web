package db

import (
	"fmt"
	"log"
	"os"
	"unicode/utf8"
	"waystone-web/auth"
	"waystone-web/models"
)

// EnsureAdmin forces the env-managed admin account (ADMIN_USERNAME /
// ADMIN_PASSWORD) to exist, be unblocked, hold the admin role and use the
// password from the environment. It runs on every startup.
func EnsureAdmin() error {
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		log.Println("WARNING: ADMIN_PASSWORD is not set - the admin account cannot log in")
		return nil
	}
	if len(password) > auth.MaxPasswordBytes {
		log.Fatalf("ADMIN_PASSWORD is %d bytes long; bcrypt accepts at most %d bytes - use a shorter password", len(password), auth.MaxPasswordBytes)
	}
	if n := utf8.RuneCountInString(password); n < auth.MinPasswordLength {
		log.Printf("WARNING: ADMIN_PASSWORD is only %d characters - use at least %d", n, auth.MinPasswordLength)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("failed to hash admin password: %w", err)
	}

	username := auth.AdminUsername()
	user, err := GetUserByUsername(username)
	if err != nil {
		return fmt.Errorf("failed to look up admin: %w", err)
	}
	if user == nil {
		user = &models.User{Username: username, Name: username, Roles: []string{models.RoleUser}}
	}

	// Revoke existing sessions only when ADMIN_PASSWORD actually changed, so a
	// plain restart doesn't log the admin out.
	if user.PasswordHash != "" && !auth.CheckPassword(user.PasswordHash, password) {
		user.SessionVersion++
	}
	user.PasswordHash = hash
	user.MustChangePassword = false
	user.Blocked = false
	if !user.IsAdmin() {
		user.Roles = append(user.Roles, models.RoleAdmin)
	}

	if err := SaveUser(*user); err != nil {
		return fmt.Errorf("failed to save admin: %w", err)
	}
	return nil
}
