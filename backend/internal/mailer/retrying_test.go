package mailer

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeMailer is a deterministic Mailer test double: it returns the
// configured error sequence in order and counts the number of Send calls.
type fakeMailer struct {
	errs  []error
	calls int
}

func (f *fakeMailer) Send(_ context.Context, _ Message) error {
	idx := f.calls
	f.calls++
	if idx >= len(f.errs) {
		return nil
	}
	return f.errs[idx]
}

func TestRetryingMailerSucceedsOnFirstAttempt(t *testing.T) {
	inner := &fakeMailer{errs: nil}
	r := NewRetrying(inner, 3, time.Millisecond, time.Now)

	if err := r.Send(context.Background(), Message{To: "a@b"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("want 1 inner call, got %d", inner.calls)
	}
}

func TestRetryingMailerSucceedsAfterTransientFailure(t *testing.T) {
	inner := &fakeMailer{errs: []error{errors.New("smtp 421"), nil}}
	r := NewRetrying(inner, 3, time.Microsecond, time.Now)

	if err := r.Send(context.Background(), Message{To: "a@b"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.calls != 2 {
		t.Fatalf("want 2 inner calls, got %d", inner.calls)
	}
}

func TestRetryingMailerExhaustsAttempts(t *testing.T) {
	boom := errors.New("smtp permanent")
	inner := &fakeMailer{errs: []error{boom, boom, boom}}
	r := NewRetrying(inner, 3, time.Microsecond, time.Now)

	err := r.Send(context.Background(), Message{To: "a@b"})
	if err == nil {
		t.Fatalf("expected error after exhausted attempts")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped boom error, got %v", err)
	}
	if inner.calls != 3 {
		t.Fatalf("want 3 inner calls, got %d", inner.calls)
	}
}

func TestRetryingMailerClampsAttemptsToOne(t *testing.T) {
	boom := errors.New("smtp permanent")
	inner := &fakeMailer{errs: []error{boom}}
	r := NewRetrying(inner, 0, time.Microsecond, time.Now)

	if err := r.Send(context.Background(), Message{To: "a@b"}); err == nil {
		t.Fatalf("expected error from single attempt")
	}
	if inner.calls != 1 {
		t.Fatalf("want 1 inner call (attempts clamped to 1), got %d", inner.calls)
	}
}

func TestRetryingMailerHonoursCancelledContext(t *testing.T) {
	inner := &fakeMailer{errs: []error{errors.New("boom"), nil}}
	// 1 second backoff would dominate the test if not cancelled.
	r := NewRetrying(inner, 3, time.Second, time.Now)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE the call so the first ctx.Err() check fires.

	if err := r.Send(ctx, Message{To: "a@b"}); err == nil {
		t.Fatalf("expected error on cancelled context")
	}
	if inner.calls != 0 {
		t.Fatalf("want 0 inner calls on pre-cancelled ctx, got %d", inner.calls)
	}
}
