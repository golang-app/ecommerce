package domain

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"time"
)

var ErrSessionNotFound = errors.New("session not found")

type Session struct {
	id         string
	customerID string
	expiresAt  time.Time
}

func NewSession(id string, customerID string, expires time.Time) *Session {
	return &Session{
		id:         id,
		customerID: customerID,
		expiresAt:  expires,
	}
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) CustomerID() string {
	return s.customerID
}

func (s *Session) ExpiresAt() time.Time {
	return s.expiresAt
}

func (s *Session) Invalidate() {
	s.expiresAt = time.Now().Add(-1 * time.Hour)
}

func (s *Session) Expired() bool {
	now := time.Now()
	return s.expiresAt.Before(now)
}

func NewSessionID() string {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(b)
}
