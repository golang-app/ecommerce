package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/promo/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/promo/app"
	"github.com/bkielbasa/go-ecommerce/backend/promo/domain"
)

func newService(t *testing.T) (*app.Service, *adapter.InMemory) {
	t.Helper()
	store := adapter.NewInMemory()
	srv := app.NewService(store).WithClock(func() time.Time {
		return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	})
	return srv, store
}

func mustCode(t *testing.T, code string, kind domain.Kind, value int64, ccy string, from, until *time.Time, max, perCustomer int) domain.Code {
	t.Helper()
	c, err := domain.NewCode(code, kind, value, ccy, from, until, max, perCustomer)
	if err != nil {
		t.Fatalf("NewCode: %v", err)
	}
	return c
}

func TestResolve_HappyPath_Percent(t *testing.T) {
	srv, store := newService(t)
	ctx := context.Background()
	if err := store.Create(ctx, mustCode(t, "SAVE10", domain.KindPercent, 10, "USD", nil, nil, 0, 1)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	d, err := srv.Resolve(ctx, "SAVE10", "jane@example.com", 2500, 500)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if d.AmountMinor() != 250 {
		t.Errorf("amount = %d, want 250", d.AmountMinor())
	}
}

func TestResolve_HappyPath_FreeShipping(t *testing.T) {
	srv, store := newService(t)
	ctx := context.Background()
	if err := store.Create(ctx, mustCode(t, "FREESHIP", domain.KindFreeShipping, 0, "", nil, nil, 0, 1)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	d, err := srv.Resolve(ctx, "FREESHIP", "jane@example.com", 2500, 1500)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !d.FreeShipping() || d.AmountMinor() != 0 {
		t.Errorf("expected free shipping with amount 0, got amount=%d free=%v", d.AmountMinor(), d.FreeShipping())
	}
}

func TestResolve_AnonymousRejected(t *testing.T) {
	srv, store := newService(t)
	ctx := context.Background()
	_ = store.Create(ctx, mustCode(t, "X", domain.KindPercent, 5, "USD", nil, nil, 0, 1))
	if _, err := srv.Resolve(ctx, "X", "", 1000, 0); !errors.Is(err, app.ErrCodeAnonymous) {
		t.Errorf("err = %v, want ErrCodeAnonymous", err)
	}
}

func TestResolve_NotFound(t *testing.T) {
	srv, _ := newService(t)
	ctx := context.Background()
	if _, err := srv.Resolve(ctx, "MISSING", "jane@example.com", 1000, 0); !errors.Is(err, app.ErrCodeNotFound) {
		t.Errorf("err = %v, want ErrCodeNotFound", err)
	}
}

func TestResolve_Expired_BeforeWindow(t *testing.T) {
	srv, store := newService(t)
	ctx := context.Background()
	from := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Create(ctx, mustCode(t, "FUTURE", domain.KindPercent, 5, "USD", &from, nil, 0, 1)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := srv.Resolve(ctx, "FUTURE", "jane@example.com", 1000, 0); !errors.Is(err, app.ErrCodeExpired) {
		t.Errorf("err = %v, want ErrCodeExpired", err)
	}
}

func TestResolve_Expired_AfterWindow(t *testing.T) {
	srv, store := newService(t)
	ctx := context.Background()
	until := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	if err := store.Create(ctx, mustCode(t, "PAST", domain.KindPercent, 5, "USD", nil, &until, 0, 1)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := srv.Resolve(ctx, "PAST", "jane@example.com", 1000, 0); !errors.Is(err, app.ErrCodeExpired) {
		t.Errorf("err = %v, want ErrCodeExpired", err)
	}
}

func TestResolve_MaxUsesReached(t *testing.T) {
	srv, store := newService(t)
	ctx := context.Background()
	c := domain.RebuildCode("CAP", domain.KindPercent, 5, "USD", nil, nil, 1, 1, 1, time.Now())
	if err := store.Create(ctx, c); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := srv.Resolve(ctx, "CAP", "jane@example.com", 1000, 0); !errors.Is(err, app.ErrCodeMaxUsesReached) {
		t.Errorf("err = %v, want ErrCodeMaxUsesReached", err)
	}
}

func TestResolve_CustomerLimit(t *testing.T) {
	srv, store := newService(t)
	ctx := context.Background()
	if err := store.Create(ctx, mustCode(t, "ONEPER", domain.KindPercent, 5, "USD", nil, nil, 0, 1)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := srv.Redeem(ctx, "ONEPER", "ord-1", "jane@example.com", domain.NewDiscount("ONEPER", domain.KindPercent, 50, "USD", false)); err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if _, err := srv.Resolve(ctx, "ONEPER", "jane@example.com", 1000, 0); !errors.Is(err, app.ErrCodeCustomerLimit) {
		t.Errorf("err = %v, want ErrCodeCustomerLimit", err)
	}
}

func TestRedeem_IdempotentOnSameOrder(t *testing.T) {
	srv, store := newService(t)
	ctx := context.Background()
	if err := store.Create(ctx, mustCode(t, "DUP", domain.KindPercent, 5, "USD", nil, nil, 1, 0)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	d := domain.NewDiscount("DUP", domain.KindPercent, 50, "USD", false)
	if err := srv.Redeem(ctx, "DUP", "ord-1", "jane@example.com", d); err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if err := srv.Redeem(ctx, "DUP", "ord-1", "jane@example.com", d); err != nil {
		t.Errorf("idempotent redeem err = %v, want nil", err)
	}
	got, err := store.Find(ctx, "DUP")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.UsedCount() != 1 {
		t.Errorf("used_count = %d, want 1 (idempotent retry must not double-count)", got.UsedCount())
	}
}
