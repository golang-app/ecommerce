package adapter

import (
	"context"
	"sync"
	"time"
)

// passwordResetInMemory is the in-memory PasswordResetStorage used by tests.
// It mirrors the postgres semantics: rows are addressed by token hash, never
// the raw token; consumed_at acts as the burn marker so a token cannot be
// used twice.
type passwordResetInMemory struct {
	mu      sync.Mutex
	entries map[string]resetEntry
}

type resetEntry struct {
	CustomerID string
	ExpiresAt  time.Time
	Consumed   bool
}

func NewInMemoryPasswordResetStorage() *passwordResetInMemory {
	return &passwordResetInMemory{entries: make(map[string]resetEntry)}
}

// BeginPasswordReset mints a token (mirroring the postgres impl), stores its
// hash with the TTL, and hands the raw value back to the caller.
func (p *passwordResetInMemory) BeginPasswordReset(ctx context.Context, customerID string, ttl time.Duration) (string, error) {
	raw, err := newResetToken()
	if err != nil {
		return "", err
	}
	hash := hashResetToken(raw)

	p.mu.Lock()
	p.entries[hash] = resetEntry{
		CustomerID: customerID,
		ExpiresAt:  time.Now().Add(ttl),
	}
	p.mu.Unlock()
	return raw, nil
}

// ConsumePasswordReset validates and burns the token in a single critical
// section. Returns ErrInvalidResetToken for unknown / expired / already-used
// tokens — same vague error as the postgres impl so the handler layer can
// treat them uniformly.
func (p *passwordResetInMemory) ConsumePasswordReset(ctx context.Context, rawToken string) (string, error) {
	if rawToken == "" {
		return "", ErrInvalidResetToken
	}
	hash := hashResetToken(rawToken)

	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.entries[hash]
	if !ok || entry.Consumed || time.Now().After(entry.ExpiresAt) {
		return "", ErrInvalidResetToken
	}
	entry.Consumed = true
	p.entries[hash] = entry
	return entry.CustomerID, nil
}
