package adapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
)

// TestPasswordResetRoundtrip exercises the happy path: a token freshly
// minted by Begin is consumable exactly once and returns the same
// customerID that was supplied.
func TestPasswordResetRoundtrip(t *testing.T) {
	ctx := context.Background()
	store := adapter.NewInMemoryPasswordResetStorage()

	raw, err := store.BeginPasswordReset(ctx, "alice@example.com", time.Minute)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if raw == "" {
		t.Fatalf("expected raw token, got empty")
	}

	got, err := store.ConsumePasswordReset(ctx, raw)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if got != "alice@example.com" {
		t.Fatalf("Consume returned %q, want alice@example.com", got)
	}

	// Second consume must fail — tokens are one-shot.
	if _, err := store.ConsumePasswordReset(ctx, raw); !errors.Is(err, adapter.ErrInvalidResetToken) {
		t.Fatalf("second Consume err = %v, want ErrInvalidResetToken", err)
	}
}

func TestPasswordResetUnknownToken(t *testing.T) {
	store := adapter.NewInMemoryPasswordResetStorage()
	if _, err := store.ConsumePasswordReset(context.Background(), "nope"); !errors.Is(err, adapter.ErrInvalidResetToken) {
		t.Fatalf("err = %v, want ErrInvalidResetToken", err)
	}
}

func TestPasswordResetExpired(t *testing.T) {
	ctx := context.Background()
	store := adapter.NewInMemoryPasswordResetStorage()

	raw, err := store.BeginPasswordReset(ctx, "alice@example.com", -1*time.Second)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := store.ConsumePasswordReset(ctx, raw); !errors.Is(err, adapter.ErrInvalidResetToken) {
		t.Fatalf("expired Consume err = %v, want ErrInvalidResetToken", err)
	}
}

func TestPasswordResetEmptyToken(t *testing.T) {
	store := adapter.NewInMemoryPasswordResetStorage()
	if _, err := store.ConsumePasswordReset(context.Background(), ""); !errors.Is(err, adapter.ErrInvalidResetToken) {
		t.Fatalf("err = %v, want ErrInvalidResetToken", err)
	}
}
