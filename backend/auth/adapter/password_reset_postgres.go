package adapter

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// ErrInvalidResetToken is returned by ConsumePasswordReset when the supplied
// token does not match any unused, unexpired row. It is intentionally vague:
// callers MUST NOT leak whether the token was wrong, expired, or already
// consumed (each variant is useful intelligence to an attacker).
var ErrInvalidResetToken = errors.New("invalid or expired password reset token")

// passwordResetPostgres is the postgres-backed implementation of
// PasswordResetStorage. The token row stores a sha256 of the raw token so a
// database read leak cannot itself be used to impersonate a reset request;
// the raw token only ever lives on the wire (in the email link) and in the
// browser tab the user opened.
type passwordResetPostgres struct {
	db *sql.DB
}

func NewPostgresPasswordResetStorage(db *sql.DB) passwordResetPostgres {
	return passwordResetPostgres{db: db}
}

// BeginPasswordReset mints a fresh token, persists its hash with the chosen
// TTL, and returns the RAW token to the caller. The caller is responsible
// for emailing it; the raw token is never written to the DB.
func (p passwordResetPostgres) BeginPasswordReset(ctx context.Context, customerID string, ttl time.Duration) (string, error) {
	raw, err := newResetToken()
	if err != nil {
		return "", fmt.Errorf("could not generate reset token: %w", err)
	}
	hash := hashResetToken(raw)
	expires := time.Now().Add(ttl)

	_, err = p.db.ExecContext(ctx,
		"INSERT INTO auth_password_reset (token_hash, customer_id, expires_at) VALUES ($1, $2, $3)",
		hash, customerID, expires)
	if err != nil {
		return "", fmt.Errorf("could not store reset token: %w", err)
	}
	return raw, nil
}

// ConsumePasswordReset validates and atomically burns a reset token. The
// SELECT … FOR UPDATE inside the same tx ensures two concurrent consumers
// cannot both succeed; the second one sees consumed_at IS NOT NULL and is
// rejected with ErrInvalidResetToken (we never tell the caller which of
// "wrong / expired / already used" applied).
func (p passwordResetPostgres) ConsumePasswordReset(ctx context.Context, rawToken string) (string, error) {
	if rawToken == "" {
		return "", ErrInvalidResetToken
	}
	hash := hashResetToken(rawToken)

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("could not begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var customerID string
	err = tx.QueryRowContext(ctx,
		`SELECT customer_id FROM auth_password_reset
		   WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > now()
		   FOR UPDATE`,
		hash).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidResetToken
	}
	if err != nil {
		return "", fmt.Errorf("could not lookup reset token: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE auth_password_reset SET consumed_at = now() WHERE token_hash = $1",
		hash); err != nil {
		return "", fmt.Errorf("could not consume reset token: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("could not commit consume tx: %w", err)
	}
	return customerID, nil
}

// newResetToken returns a fresh 32-byte (256-bit) random token, base64-url
// encoded. The encoding produces ~43 URL-safe characters — short enough to
// paste comfortably and far too long to brute-force.
func newResetToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashResetToken applies sha256 and returns the hex digest. We avoid bcrypt
// here on purpose: reset tokens are already high-entropy random strings, and
// the storage layer needs deterministic lookups by hash (a bcrypt salt
// rotates every call, defeating the WHERE clause).
func hashResetToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
