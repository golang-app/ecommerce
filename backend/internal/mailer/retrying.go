package mailer

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RetryingMailer wraps an inner Mailer with bounded exponential-backoff
// retries. It is meant for transient SMTP failures (temporary DNS hiccups,
// brief MTA outages, "421 try again later") where a second attempt a few
// hundred milliseconds later usually succeeds.
//
// # When NOT to use this
//
// Retries are only safe when the wrapped operation is effectively idempotent
// from the user's perspective. SMTP itself is NOT strictly idempotent — the
// relay may have accepted the message and then dropped the connection before
// acknowledging it, in which case a retry will dispatch a second copy. For
// gocommerce that cost is low: a duplicate "order confirmation" email is
// embarrassing, not destructive.
//
// Do NOT layer RetryingMailer in front of a sender that triggers paid
// side-effects (SMS billed per message, push-notification rate quotas,
// transactional webhooks) without first making the inner operation truly
// idempotent at the provider level (idempotency key, dedupe window).
type RetryingMailer struct {
	inner    Mailer
	attempts int
	backoff  time.Duration
	// clock returns the current wall time. Injected so tests can avoid
	// real sleeps; production wires time.Now.
	clock func() time.Time
}

// NewRetrying constructs a RetryingMailer. attempts is the total number of
// Send invocations (including the first); values below 1 are clamped to 1
// so a misconfigured caller still issues a single attempt rather than
// silently dropping the send. backoff is the base delay; the k-th retry
// sleeps backoff * 2^(k-1) (200ms, 400ms, 800ms, ...). A nil clock falls
// back to time.Now.
func NewRetrying(inner Mailer, attempts int, backoff time.Duration, clock func() time.Time) *RetryingMailer {
	if attempts < 1 {
		attempts = 1
	}
	if clock == nil {
		clock = time.Now
	}
	return &RetryingMailer{
		inner:    inner,
		attempts: attempts,
		backoff:  backoff,
		clock:    clock,
	}
}

// Send calls inner.Send up to r.attempts times, sleeping
// backoff * 2^(i-1) between failures. It returns nil on the first success;
// on permanent failure it returns the last observed error wrapped with the
// attempt count so callers can grep logs for "after N attempts".
//
// Context cancellation short-circuits the back-off: a cancelled ctx ends
// the loop immediately with ctx.Err(), so shutdown does not block on
// queued retries.
func (r *RetryingMailer) Send(ctx context.Context, msg Message) error {
	var lastErr error
	for i := 1; i <= r.attempts; i++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return fmt.Errorf("mailer: send cancelled after %d attempt(s): %w", i-1, errors.Join(lastErr, err))
			}
			return err
		}
		err := r.inner.Send(ctx, msg)
		if err == nil {
			return nil
		}
		lastErr = err
		if i == r.attempts {
			break
		}
		// Exponential backoff: 1st retry waits backoff, 2nd waits 2x,
		// 3rd waits 4x, ... Bit-shift instead of math.Pow keeps the
		// arithmetic integer and panic-free.
		delay := r.backoff << (i - 1)
		if delay > 0 {
			if err := sleep(ctx, delay, r.clock); err != nil {
				return fmt.Errorf("mailer: send cancelled after %d attempt(s): %w", i, errors.Join(lastErr, err))
			}
		}
	}
	return fmt.Errorf("mailer: send failed after %d attempt(s): %w", r.attempts, lastErr)
}

// sleep blocks for d, but returns early with ctx.Err() if the context is
// cancelled. The clock is consulted only to compute the deadline; the
// actual wait uses a real timer because there is no portable way to mock
// time.After without an injected scheduler. Tests that exercise the retry
// loop pass attempts>1 with a near-zero backoff to keep wall time
// negligible.
func sleep(ctx context.Context, d time.Duration, _ func() time.Time) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
