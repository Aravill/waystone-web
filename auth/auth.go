// Package auth holds password and username helpers for local accounts.
package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"waystone-web/models"
)

const (
	MinPasswordLength = 10
	// MaxPasswordBytes is bcrypt's input limit; longer passwords fail to hash.
	MaxPasswordBytes = 72
	tempPasswordLen  = 12
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

// PasswordPolicyViolations returns the password rules that password breaks,
// phrased to follow "password must ...". An empty result means it is accepted.
func PasswordPolicyViolations(password, username string) []string {
	var v []string
	if utf8.RuneCountInString(password) < MinPasswordLength {
		v = append(v, fmt.Sprintf("be at least %d characters", MinPasswordLength))
	}
	if len(password) > MaxPasswordBytes {
		v = append(v, fmt.Sprintf("be at most %d bytes (accented or non-Latin characters count as 2-4)", MaxPasswordBytes))
	}
	var lower, upper, digit bool
	for _, r := range password {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		}
	}
	if !lower {
		v = append(v, "contain a lowercase letter")
	}
	if !upper {
		v = append(v, "contain an uppercase letter")
	}
	if !digit {
		v = append(v, "contain a digit")
	}
	if username != "" && strings.Contains(strings.ToLower(password), strings.ToLower(username)) {
		v = append(v, "not contain your username")
	}
	return v
}

// PasswordPolicyError builds a user-facing message from policy violations.
func PasswordPolicyError(violations []string) string {
	return "password must " + strings.Join(violations, "; ")
}

// GenerateTempPassword returns a random temporary password that satisfies the
// password policy for username.
func GenerateTempPassword(username string) (string, error) {
	max := big.NewInt(int64(len(tempPasswordAlphabet)))
	out := make([]byte, tempPasswordLen)
	// Each draw passes ~85% of the time; the cap only makes termination obvious.
	for attempt := 0; attempt < 100; attempt++ {
		for i := range out {
			n, err := rand.Int(rand.Reader, max)
			if err != nil {
				return "", err
			}
			out[i] = tempPasswordAlphabet[n.Int64()]
		}
		if len(PasswordPolicyViolations(string(out), username)) == 0 {
			return string(out), nil
		}
	}
	return "", errors.New("could not generate a policy-compliant temporary password")
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
