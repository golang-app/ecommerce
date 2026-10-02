// Package ratelimit provides a rate limiting port and adapters for
// distributed (PostgreSQL) and in-memory rate limiting.
package ratelimit

import (
	"context"
	"database/sql"
	"math"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Standard action names for rate limiting across the application.
const (
	ActionLogin          = "login"
	ActionRegister       = "register"
	ActionAddToCart      = "add_to_cart"
	ActionForgotPassword = "forgot_password"
	ActionCheckout       = "checkout"
)

// Rule defines the token refill rate and burst capacity for an action.
type Rule struct {
	Rate  float64 // Tokens added per second
	Burst int     // Maximum burst capacity
}

// NewRule constructs a Rule given a maximum limit over a duration window.
func NewRule(limit int, window time.Duration) Rule {
	if limit < 1 {
		limit = 1
	}
	if window <= 0 {
		window = time.Second
	}
	return Rule{
		Rate:  float64(limit) / window.Seconds(),
		Burst: limit,
	}
}

// DefaultRules returns the production rate limit rules for standard application actions.
func DefaultRules() map[string]Rule {
	return map[string]Rule{
		ActionLogin:          NewRule(5, time.Minute),
		ActionRegister:       NewRule(3, time.Hour),
		ActionAddToCart:      NewRule(30, time.Minute),
		ActionForgotPassword: NewRule(3, time.Hour),
		ActionCheckout:       NewRule(5, time.Minute),
	}
}

// Limiter determines whether an action for a given key is permitted.
type Limiter interface {
	Allow(ctx context.Context, action string, key string) bool
}

// inMemoryBucket stores token bucket state for a single (action, key) in-memory.
type inMemoryBucket struct {
	tokens     float64
	lastRefill time.Time
}

type inMemoryLimiter struct {
	mu      sync.Mutex
	rules   map[string]Rule
	buckets map[string]*inMemoryBucket
	now     func() time.Time
}

// NewInMemory constructs a thread-safe in-memory Limiter with the provided rules.
func NewInMemory(rules map[string]Rule) Limiter {
	copiedRules := make(map[string]Rule, len(rules))
	for k, v := range rules {
		copiedRules[k] = v
	}
	return &inMemoryLimiter{
		rules:   copiedRules,
		buckets: make(map[string]*inMemoryBucket),
		now:     time.Now,
	}
}

func (l *inMemoryLimiter) Allow(ctx context.Context, action string, key string) bool {
	rule, ok := l.rules[action]
	if !ok {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	bucketKey := action + ":" + key
	now := l.now().UTC()

	b, exists := l.buckets[bucketKey]
	if !exists {
		l.buckets[bucketKey] = &inMemoryBucket{
			tokens:     float64(rule.Burst) - 1.0,
			lastRefill: now,
		}
		return true
	}

	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * rule.Rate
		if b.tokens > float64(rule.Burst) {
			b.tokens = float64(rule.Burst)
		}
		b.lastRefill = now
	}

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}

	return false
}

type postgresLimiter struct {
	db     *sql.DB
	rules  map[string]Rule
	logger logrus.FieldLogger
}

// NewPostgres constructs a Limiter backed by a PostgreSQL database table,
// synchronizing rate limiting state across all instances/pods.
func NewPostgres(db *sql.DB, rules map[string]Rule, logger logrus.FieldLogger) Limiter {
	copiedRules := make(map[string]Rule, len(rules))
	for k, v := range rules {
		copiedRules[k] = v
	}
	return &postgresLimiter{
		db:     db,
		rules:  copiedRules,
		logger: logger,
	}
}

func (p *postgresLimiter) Allow(ctx context.Context, action string, key string) bool {
	rule, ok := p.rules[action]
	if !ok {
		return true
	}

	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		if p.logger != nil {
			p.logger.WithError(err).Warn("rate limiter: failed to begin transaction, failing open")
		}
		return true
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()

	// Ensure the row exists with full burst capacity on first access.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO rate_limits (action, key, tokens, last_refill)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (action, key) DO NOTHING
	`, action, key, float64(rule.Burst), now)
	if err != nil {
		if p.logger != nil {
			p.logger.WithError(err).Warn("rate limiter: failed to ensure bucket row, failing open")
		}
		return true
	}

	var tokens float64
	var lastRefill time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT tokens, last_refill
		FROM rate_limits
		WHERE action = $1 AND key = $2
		FOR UPDATE
	`, action, key).Scan(&tokens, &lastRefill)
	if err != nil {
		if p.logger != nil {
			p.logger.WithError(err).Warn("rate limiter: failed to lock bucket row, failing open")
		}
		return true
	}

	elapsed := now.Sub(lastRefill).Seconds()
	if elapsed > 0 {
		tokens = math.Min(float64(rule.Burst), tokens+elapsed*rule.Rate)
	}

	allowed := false
	if tokens >= 1.0 {
		tokens -= 1.0
		allowed = true
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE rate_limits
		SET tokens = $1, last_refill = $2
		WHERE action = $3 AND key = $4
	`, tokens, now, action, key)
	if err != nil {
		if p.logger != nil {
			p.logger.WithError(err).Warn("rate limiter: failed to update bucket, failing open")
		}
		return true
	}

	if err := tx.Commit(); err != nil {
		if p.logger != nil {
			p.logger.WithError(err).Warn("rate limiter: failed to commit transaction, failing open")
		}
		return true
	}

	return allowed
}
