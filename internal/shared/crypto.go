// Package shared holds dependency-free primitives shared across modules.
// It must not import GORM, Gin, asynq, or any business module.
package shared

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/crypto/pbkdf2"
)

// Money represents a monetary amount in the smallest currency unit (e.g. cents/fen).
type Money int64

func (m Money) String() string {
	return fmt.Sprintf("%.2f", float64(m)/100)
}

// Slugify converts a string to a URL-friendly slug.
func Slugify(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
		} else if unicode.IsSpace(r) || r == '-' || r == '_' {
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	result := strings.TrimRight(b.String(), "-")
	if result == "" {
		return "untitled"
	}
	return result
}

// UniqueSlug ensures slug uniqueness by appending a counter if needed.
func UniqueSlug(base string, exists func(string) bool) string {
	slug := Slugify(base)
	if !exists(slug) {
		return slug
	}
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", slug, i)
		if !exists(candidate) {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", slug, 1000)
}

// PasswordHash creates a PBKDF2-SHA256 hash of the password.
func PasswordHash(password string, salt []byte) string {
	if salt == nil {
		salt = []byte("nodeloc-store-salt-v1")
	}
	hash := pbkdf2.Key([]byte(password), salt, 100000, 32, sha256.New)
	return hex.EncodeToString(hash)
}

// PasswordVerify checks a password against a hash.
func PasswordVerify(password, hashStr string, salt []byte) bool {
	expected := PasswordHash(password, salt)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(hashStr)) == 1
}

// Sign signs params with the given key using sorted key=value pairs.
func Sign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(b.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

// SignWithHashedToken signs using SHA256(secret) as the HMAC key (NodeLoc outgoing).
func SignWithHashedToken(params map[string]string, secret string) string {
	tokenHash := sha256.Sum256([]byte(secret))
	return Sign(params, hex.EncodeToString(tokenHash[:]))
}

// VerifyCallback verifies a NodeLoc callback signature.
//
// NodeLoc releases differ in two ways that both read as "signature mismatch" if
// only one form is accepted: the HMAC key is either the secret as issued or its
// SHA-256 hex, and empty-valued parameters are sometimes left out of the signed
// string. Every combination is tried, so a genuinely signed redirect settles.
func VerifyCallback(params map[string]string, secret string) bool {
	provided := strings.TrimSpace(params["signature"])
	if provided == "" {
		return false
	}
	unsigned := make(map[string]string, len(params))
	for key, value := range params {
		if key != "signature" {
			unsigned[key] = value
		}
	}

	tokenHash := sha256.Sum256([]byte(secret))
	keys := []string{secret, hex.EncodeToString(tokenHash[:])}
	for _, key := range keys {
		if matches(Sign(unsigned, key), provided) {
			return true
		}
	}

	compact := make(map[string]string, len(unsigned))
	for key, value := range unsigned {
		if strings.TrimSpace(value) != "" {
			compact[key] = value
		}
	}
	if len(compact) == len(unsigned) {
		return false
	}
	for _, key := range keys {
		if matches(Sign(compact, key), provided) {
			return true
		}
	}
	return false
}

// matches compares two hex signatures without leaking the expected value.
func matches(expected, provided string) bool {
	return hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(expected))),
		[]byte(strings.ToLower(strings.TrimSpace(provided))))
}

// Truncate truncates a string to max runes, appending "..." if cut.
func Truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}

var slugRegex = regexp.MustCompile(`[^a-z0-9-]`)
