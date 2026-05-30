// Package idempotency implements the HTTP-boundary Idempotency-Key
// pattern: clients send `Idempotency-Key: <opaque>` on a state-changing
// request (POST/PUT/PATCH/DELETE) and a retry carrying the SAME key
// gets the recorded response replayed verbatim, without re-running the
// handler.
//
// WHY. CSRF protects against cross-origin abuse; retries are a
// different failure mode. A network blip after the server commits but
// before the client receives the response leaves the client uncertain
// whether the action took effect. Without a key the client either
// retries (and risks a duplicate write) or gives up (and risks lost
// work). The Idempotency-Key contract makes "retry" the safe choice.
//
// WHERE IT FITS. The Outbox upgrades publisher→bus delivery to
// at-least-once; the Inbox (internal/inbox) gives subscribers per-event
// dedupe so their side effects run effectively once. The two together
// give exactly-once SERVER-side semantics for asynchronous events.
// This package is the matching boundary control for CLIENT-driven
// requests: it makes "the same HTTP retry" cheap and safe in exactly
// the same way the Inbox makes "the same outbox redelivery" cheap and
// safe.
//
// CONTRACT.
//   - The first request with a given key runs the handler; the
//     middleware records (status, headers, body) and writes them under
//     that key with a 24h TTL.
//   - A retry with the SAME key returns the stored tuple without
//     invoking the handler.
//   - Save uses INSERT ... ON CONFLICT (key) DO NOTHING so a true race
//     between two simultaneous first-time submissions still records
//     exactly one response (whichever transaction commits first wins;
//     the loser's recording is silently dropped — the loser's HTTP
//     response has already been sent over the wire, that is fine).
//   - TTL is 24h. The table is a retry-safety cache, not an audit log;
//     a longer window grows the surface without unlocking real use
//     cases.
//
// LIMITATIONS. Stored response sizes are bounded only by the table
// (bytea, jsonb) — operators MUST avoid opting in handlers that stream
// large responses. The middleware buffers the entire body in memory
// before recording it.
package idempotency

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// StoredResponse is the recorded snapshot of a successfully processed
// request. The middleware writes the captured ResponseWriter state
// here on first execution and replays the same StoredResponse on every
// subsequent retry that arrives with the same Idempotency-Key.
//
// Headers is a defensive copy of http.Header (which is itself a
// map[string][]string) — storing it through the StoredResponse value
// keeps the package storage-only with no net/http dependency.
type StoredResponse struct {
	StatusCode int
	Body       []byte
	Headers    map[string][]string
}

// Postgres is the durable Idempotency-Key store backed by the
// idempotency_key table. The struct is intentionally a value type so
// the composition root passes it around by copy (it is just a thin
// wrapper over the pool handle; no per-store state to share).
type Postgres struct {
	db *sql.DB
}

// NewPostgres builds a Postgres idempotency store from an *sql.DB.
//
// Wiring note (the constraint that main.go is off-limits to this
// agent): the composition root MUST call
//
//	app.Use(layout.IdempotencyMiddleware(idempotency.NewPostgres(db)))
//
// after layout.New so the middleware runs for every route on the
// shared gorilla/mux pipeline. The middleware is content-agnostic and
// only honors the header when it is present, so opting in a handler is
// purely a client-side decision to include `Idempotency-Key`.
func NewPostgres(db *sql.DB) Postgres {
	return Postgres{db: db}
}

// Find looks up a recorded response by key.
//
// Return contract:
//   - ok == true,  err == nil -> a fresh (not-yet-expired) row was
//     found; the middleware MUST replay it instead of calling the
//     handler.
//   - ok == false, err == nil -> no row, or the row has expired; the
//     middleware MUST run the handler and then Save the result.
//   - err != nil              -> storage failure; the middleware MUST
//     fall through to the handler. The point of the table is retry
//     safety, not availability — a DB blip should not block real
//     traffic, and the retry-on-network-blip case is itself rare
//     enough that an occasional double-execution under DB stress is
//     an acceptable degradation.
//
// The expires_at filter is applied in SQL so an expired-but-not-yet-
// reaped row is invisible to the hot path without the middleware
// having to know the TTL.
func (p Postgres) Find(ctx context.Context, key string) (StoredResponse, bool, error) {
	var (
		status     int
		body       []byte
		headersRaw []byte
	)
	err := p.db.QueryRowContext(ctx, `
		SELECT status_code, response_body, response_headers
		FROM idempotency_key
		WHERE key = $1 AND expires_at > now()
	`, key).Scan(&status, &body, &headersRaw)
	if err == sql.ErrNoRows {
		return StoredResponse{}, false, nil
	}
	if err != nil {
		return StoredResponse{}, false, fmt.Errorf("idempotency: find %q: %w", key, err)
	}
	headers := map[string][]string{}
	if len(headersRaw) > 0 {
		if jerr := json.Unmarshal(headersRaw, &headers); jerr != nil {
			return StoredResponse{}, false, fmt.Errorf("idempotency: decode headers for %q: %w", key, jerr)
		}
	}
	return StoredResponse{StatusCode: status, Body: body, Headers: headers}, true, nil
}

// Save records the response captured by the middleware under key.
//
// The INSERT uses ON CONFLICT (key) DO NOTHING so two simultaneous
// first-time requests with the same key race safely: whichever
// transaction commits first owns the recorded tuple, and the loser's
// Save is silently a no-op. The loser's response has already been sent
// to its caller — that response just doesn't get cached for a future
// retry. The next retry will replay the winner's tuple, which is the
// outcome the contract promises.
//
// ttl drives expires_at; the middleware passes 24h. The default in the
// table is also 24h, but the column is set explicitly here so a future
// caller experimenting with a different TTL doesn't need a schema
// change.
func (p Postgres) Save(ctx context.Context, key, method, path string, resp StoredResponse, ttl time.Duration) error {
	headers := resp.Headers
	if headers == nil {
		headers = map[string][]string{}
	}
	headersRaw, err := json.Marshal(headers)
	if err != nil {
		return fmt.Errorf("idempotency: encode headers for %q: %w", key, err)
	}
	_, err = p.db.ExecContext(ctx, `
		INSERT INTO idempotency_key (
			key, method, path, status_code, response_body, response_headers, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)
		ON CONFLICT (key) DO NOTHING
	`, key, method, path, resp.StatusCode, resp.Body, headersRaw, fmt.Sprintf("%d milliseconds", ttl.Milliseconds()))
	if err != nil {
		return fmt.Errorf("idempotency: save %q: %w", key, err)
	}
	return nil
}
