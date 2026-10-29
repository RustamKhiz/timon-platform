package adminauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var ErrUnauthorized = errors.New("unauthorized")

type Manager struct {
	username     string
	passwordHash []byte
	secret       []byte
	ttl          time.Duration
}

func New(username, passwordHash, secret string, ttl time.Duration) (*Manager, error) {
	if strings.TrimSpace(username) == "" || passwordHash == "" || len(secret) < 32 {
		return nil, errors.New("admin username, bcrypt password hash and a session secret of at least 32 characters are required")
	}
	if _, err := bcrypt.Cost([]byte(passwordHash)); err != nil {
		return nil, fmt.Errorf("invalid admin password hash: %w", err)
	}
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &Manager{username: username, passwordHash: []byte(passwordHash), secret: []byte(secret), ttl: ttl}, nil
}

func (m *Manager) Authenticate(username, password string) bool {
	validUsername := hmac.Equal([]byte(username), []byte(m.username))
	validPassword := bcrypt.CompareHashAndPassword(m.passwordHash, []byte(password)) == nil
	return validUsername && validPassword
}

func (m *Manager) Issue(now time.Time) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(m.username + "\n" + strconv.FormatInt(now.Add(m.ttl).Unix(), 10)))
	return payload + "." + m.signature(payload)
}

func (m *Manager) Verify(token string, now time.Time) error {
	payload, signature, ok := strings.Cut(token, ".")
	if !ok || !hmac.Equal([]byte(signature), []byte(m.signature(payload))) {
		return ErrUnauthorized
	}
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return ErrUnauthorized
	}
	username, expiresRaw, ok := strings.Cut(string(decoded), "\n")
	if !ok || !hmac.Equal([]byte(username), []byte(m.username)) {
		return ErrUnauthorized
	}
	expires, err := strconv.ParseInt(expiresRaw, 10, 64)
	if err != nil || !now.Before(time.Unix(expires, 0)) {
		return ErrUnauthorized
	}
	return nil
}

func (m *Manager) TTL() time.Duration { return m.ttl }

func (m *Manager) signature(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
