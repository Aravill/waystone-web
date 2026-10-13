// Package auth holds password and username helpers for local accounts.
package auth

import (
	"crypto/rand"
	"errors"
	"math/big"
	"os"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"waystone-web/models"
)

const (
	MinPasswordLength = 8
	tempPasswordLen   = 12
	// Unambiguous alphabet: no 0/O, 1/l/I.
	tempPasswordAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_-]{2,32}$`)

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether password matches the bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// GenerateTempPassword returns a random temporary password.
func GenerateTempPassword() (string, error) {
	max := big.NewInt(int64(len(tempPasswordAlphabet)))
	out := make([]byte, tempPasswordLen)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = tempPasswordAlphabet[n.Int64()]
	}
	return string(out), nil
}

// ParseUsername trims, lowercases and validates a username.
func ParseUsername(raw string) (string, error) {
	u := strings.ToLower(strings.TrimSpace(raw))
	if !usernamePattern.MatchString(u) {
		return "", errors.New("username must be 2-32 characters: letters, digits, _ or -")
	}
	return u, nil
}

// AdminUsername returns the env-managed admin username (default "admin").
func AdminUsername() string {
	if u := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))); u != "" {
		return u
	}
	return "admin"
}

// IsEnvAdmin reports whether u is the env-managed admin account.
func IsEnvAdmin(u *models.User) bool {
	return u != nil && strings.EqualFold(u.Username, AdminUsername())
}
