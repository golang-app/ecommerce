package app

// This file is the storage-conformance suite for the promo Storage port.
// One suite, two adapters: both promo/adapter/inmemory_test.go and
// promo/adapter/postgres_test.go call RunStorageConformance with their own
// factory, so every contract guarantee declared here is verified against
// both implementations.
//
// Why this lives in `package app` and not `package app_test` or in a
// dedicated testing package: the suite references the unexported Storage
// interface and the sentinel errors (ErrCodeNotFound, ErrCodeMaxUsesReached
// …) directly, and it has to be importable from the adapter test files
// (which sit in `package adapter`). Putting it in app_test would hide it
// from the adapters; putting it in a separate "promotest" package would
// require either exporting more app internals or duplicating fixtures.
//
// See backend/internal/storagetest for the pattern's documentation.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/promo/domain"
)

// RunStorageConformance exercises every contract guarantee promo.Storage
// makes to the application service. The factory is invoked once per
// sub-test so each clause starts against a clean store — postgres
// factories are expected to wipe the relevant tables before returning.
//
// Sub-test naming follows Verb_What so a single `go test -run` regex
// targets one clause across both adapters
// (e.g. `-run Conformance/Redeem_AtomicUnderConcurrency`).
func RunStorageConformance(t *testing.T, newStorage func() Storage) {
	t.Helper()

	t.Run("Create_RoundTripsViaFind", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		code := newPercentCode(t, "CONF-CREATE", 10)
		if err := s.Create(ctx, code); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := s.Find(ctx, "CONF-CREATE")
		if err != nil {
			t.Fatalf("Find after Create: %v", err)
		}
		if got.CodeText() != code.CodeText() || got.Kind() != code.Kind() || got.ValueMinor() != code.ValueMinor() {
			t.Errorf("round-trip mismatch: got code=%q kind=%q value=%d, want code=%q kind=%q value=%d",
				got.CodeText(), got.Kind(), got.ValueMinor(),
				code.CodeText(), code.Kind(), code.ValueMinor())
		}
	})

	t.Run("Create_DuplicateReturnsAlreadyExists", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		code := newPercentCode(t, "CONF-DUP", 10)
		if err := s.Create(ctx, code); err != nil {
			t.Fatalf("first Create: %v", err)
		}
		err := s.Create(ctx, code)
		if !errors.Is(err, ErrCodeAlreadyExists) {
			t.Errorf("duplicate Create err = %v, want ErrCodeAlreadyExists", err)
		}
	})

	t.Run("Update_OverwritesPersistedState", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		original := newPercentCode(t, "CONF-UPDATE", 10)
		if err := s.Create(ctx, original); err != nil {
			t.Fatalf("Create: %v", err)
		}
		updated := newPercentCode(t, "CONF-UPDATE", 25)
		if err := s.Update(ctx, updated); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := s.Find(ctx, "CONF-UPDATE")
		if err != nil {
			t.Fatalf("Find after Update: %v", err)
		}
		if got.ValueMinor() != 25 {
			t.Errorf("value after Update = %d, want 25", got.ValueMinor())
		}
	})

	t.Run("Update_MissingReturnsNotFound", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		ghost := newPercentCode(t, "CONF-GHOST", 10)
		err := s.Update(ctx, ghost)
		if !errors.Is(err, ErrCodeNotFound) {
			t.Errorf("Update on missing row err = %v, want ErrCodeNotFound", err)
		}
	})

	t.Run("Delete_RemovesAndSubsequentFindNotFound", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		if err := s.Create(ctx, newPercentCode(t, "CONF-DEL", 10)); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := s.Delete(ctx, "CONF-DEL"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := s.Find(ctx, "CONF-DEL"); !errors.Is(err, ErrCodeNotFound) {
			t.Errorf("Find after Delete err = %v, want ErrCodeNotFound", err)
		}
	})

	t.Run("Delete_MissingReturnsNotFound", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		err := s.Delete(ctx, "CONF-GHOST")
		if !errors.Is(err, ErrCodeNotFound) {
			t.Errorf("Delete on missing row err = %v, want ErrCodeNotFound", err)
		}
	})

	t.Run("Find_MissingReturnsNotFound", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		_, err := s.Find(ctx, "CONF-MISSING")
		if !errors.Is(err, ErrCodeNotFound) {
			t.Errorf("Find on missing row err = %v, want ErrCodeNotFound", err)
		}
	})

	t.Run("ListAll_NewestFirst", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		// Insert three codes; assert ListAll returns them ordered by
		// created_at DESC. The domain's NewCode stamps created_at = now,
		// so a small sleep between inserts is enough to give them distinct
		// timestamps for both adapters.
		want := []string{"CONF-LIST-A", "CONF-LIST-B", "CONF-LIST-C"}
		for _, c := range want {
			if err := s.Create(ctx, newPercentCode(t, c, 10)); err != nil {
				t.Fatalf("Create %s: %v", c, err)
			}
			time.Sleep(2 * time.Millisecond)
		}
		got, err := s.ListAll(ctx)
		if err != nil {
			t.Fatalf("ListAll: %v", err)
		}
		// Filter to just our codes so a postgres run against a non-wiped
		// table won't blow up the assertion order.
		got = filterCodes(got, want)
		if len(got) != len(want) {
			t.Fatalf("ListAll returned %d rows, want %d", len(got), len(want))
		}
		// Reverse-want is the expected order (newest-first).
		expected := []string{"CONF-LIST-C", "CONF-LIST-B", "CONF-LIST-A"}
		for i, c := range got {
			if c.CodeText() != expected[i] {
				t.Errorf("ListAll[%d] = %q, want %q (newest-first contract)", i, c.CodeText(), expected[i])
			}
		}
	})

	t.Run("CountRedemptionsByCustomer_ZeroWhenNeverRedeemed", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		if err := s.Create(ctx, newPercentCode(t, "CONF-COUNT0", 10)); err != nil {
			t.Fatalf("Create: %v", err)
		}
		n, err := s.CountRedemptionsByCustomer(ctx, "CONF-COUNT0", "jane@example.com")
		if err != nil {
			t.Fatalf("CountRedemptionsByCustomer: %v", err)
		}
		if n != 0 {
			t.Errorf("count for unredeemed customer = %d, want 0", n)
		}
	})

	t.Run("CountRedemptionsByCustomer_TalliesAndIsolatesPerCustomer", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		// max_uses=0 (unlimited) and per_customer_max=0 (unlimited) so
		// we can drive the count up freely.
		c, err := domain.NewCode("CONF-COUNT", domain.KindPercent, 10, "USD", nil, nil, 0, 0)
		if err != nil {
			t.Fatalf("NewCode: %v", err)
		}
		if err := s.Create(ctx, c); err != nil {
			t.Fatalf("Create: %v", err)
		}
		for i := 0; i < 3; i++ {
			err := s.Redeem(ctx, Redemption{
				Code:        "CONF-COUNT",
				OrderID:     "jane-ord-" + itoa(i),
				CustomerID:  "jane@example.com",
				AmountMinor: 100,
				Currency:    "USD",
			})
			if err != nil {
				t.Fatalf("Redeem #%d: %v", i, err)
			}
		}
		// John never redeems.
		got, err := s.CountRedemptionsByCustomer(ctx, "CONF-COUNT", "jane@example.com")
		if err != nil {
			t.Fatalf("CountRedemptionsByCustomer(jane): %v", err)
		}
		if got != 3 {
			t.Errorf("jane's count = %d, want 3", got)
		}
		gotJohn, err := s.CountRedemptionsByCustomer(ctx, "CONF-COUNT", "john@example.com")
		if err != nil {
			t.Fatalf("CountRedemptionsByCustomer(john): %v", err)
		}
		if gotJohn != 0 {
			t.Errorf("john's count = %d, want 0 (per-customer isolation)", gotJohn)
		}
	})

	t.Run("Redeem_BumpsUsedCount", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		if err := s.Create(ctx, newPercentCode(t, "CONF-BUMP", 10)); err != nil {
			t.Fatalf("Create: %v", err)
		}
		err := s.Redeem(ctx, Redemption{
			Code:        "CONF-BUMP",
			OrderID:     "ord-1",
			CustomerID:  "jane@example.com",
			AmountMinor: 100,
			Currency:    "USD",
		})
		if err != nil {
			t.Fatalf("Redeem: %v", err)
		}
		got, err := s.Find(ctx, "CONF-BUMP")
		if err != nil {
			t.Fatalf("Find: %v", err)
		}
		if got.UsedCount() != 1 {
			t.Errorf("used_count after Redeem = %d, want 1", got.UsedCount())
		}
	})

	t.Run("Redeem_IdempotentOnSameCodeOrder", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		if err := s.Create(ctx, newPercentCode(t, "CONF-IDEMP", 10)); err != nil {
			t.Fatalf("Create: %v", err)
		}
		r := Redemption{
			Code:        "CONF-IDEMP",
			OrderID:     "ord-1",
			CustomerID:  "jane@example.com",
			AmountMinor: 100,
			Currency:    "USD",
		}
		if err := s.Redeem(ctx, r); err != nil {
			t.Fatalf("first Redeem: %v", err)
		}
		if err := s.Redeem(ctx, r); err != nil {
			t.Errorf("idempotent Redeem err = %v, want nil", err)
		}
		got, err := s.Find(ctx, "CONF-IDEMP")
		if err != nil {
			t.Fatalf("Find: %v", err)
		}
		if got.UsedCount() != 1 {
			t.Errorf("used_count after idempotent retry = %d, want 1 (must not double-count)", got.UsedCount())
		}
	})

	t.Run("Redeem_RespectsMaxUsesCap", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		// max_uses=1, per_customer_max=0 (so the second redemption is
		// blocked by the global cap, not the per-customer one).
		c, err := domain.NewCode("CONF-CAPMAX", domain.KindPercent, 10, "USD", nil, nil, 1, 0)
		if err != nil {
			t.Fatalf("NewCode: %v", err)
		}
		if err := s.Create(ctx, c); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := s.Redeem(ctx, Redemption{
			Code:        "CONF-CAPMAX",
			OrderID:     "ord-1",
			CustomerID:  "alice@example.com",
			AmountMinor: 100,
			Currency:    "USD",
		}); err != nil {
			t.Fatalf("first Redeem: %v", err)
		}
		err = s.Redeem(ctx, Redemption{
			Code:        "CONF-CAPMAX",
			OrderID:     "ord-2",
			CustomerID:  "bob@example.com",
			AmountMinor: 100,
			Currency:    "USD",
		})
		if !errors.Is(err, ErrCodeMaxUsesReached) {
			t.Errorf("over-cap Redeem err = %v, want ErrCodeMaxUsesReached", err)
		}
	})

	t.Run("Redeem_RespectsPerCustomerCap", func(t *testing.T) {
		s := newStorage()
		ctx := context.Background()
		// max_uses=0 (unlimited globally), per_customer_max=1 — the
		// customer's second redemption must be rejected, but a different
		// customer can still redeem.
		c, err := domain.NewCode("CONF-PERCUS", domain.KindPercent, 10, "USD", nil, nil, 0, 1)
		if err != nil {
			t.Fatalf("NewCode: %v", err)
		}
		if err := s.Create(ctx, c); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := s.Redeem(ctx, Redemption{
			Code:        "CONF-PERCUS",
			OrderID:     "ord-1",
			CustomerID:  "alice@example.com",
			AmountMinor: 100,
			Currency:    "USD",
		}); err != nil {
			t.Fatalf("first Redeem: %v", err)
		}
		err = s.Redeem(ctx, Redemption{
			Code:        "CONF-PERCUS",
			OrderID:     "ord-2",
			CustomerID:  "alice@example.com",
			AmountMinor: 100,
			Currency:    "USD",
		})
		if !errors.Is(err, ErrCodeCustomerLimit) {
			t.Errorf("alice's second Redeem err = %v, want ErrCodeCustomerLimit", err)
		}
		// Bob is a different customer, so the per-customer cap doesn't
		// apply to him — global cap is unlimited here.
		if err := s.Redeem(ctx, Redemption{
			Code:        "CONF-PERCUS",
			OrderID:     "ord-3",
			CustomerID:  "bob@example.com",
			AmountMinor: 100,
			Currency:    "USD",
		}); err != nil {
			t.Errorf("bob's first Redeem err = %v, want nil (per-customer cap is per customer)", err)
		}
	})

	t.Run("Redeem_AtomicUnderConcurrency", func(t *testing.T) {
		// Atomicity test: N goroutines hammer Redeem on a code with
		// max_uses=1. Exactly one must succeed; the rest must fail with
		// ErrCodeMaxUsesReached. The barrier (WaitGroup) makes all
		// goroutines start as close to simultaneously as possible, so
		// the postgres SELECT ... FOR UPDATE and the in-memory mutex
		// both face real contention.
		s := newStorage()
		ctx := context.Background()
		c, err := domain.NewCode("CONF-ATOMIC", domain.KindPercent, 10, "USD", nil, nil, 1, 0)
		if err != nil {
			t.Fatalf("NewCode: %v", err)
		}
		if err := s.Create(ctx, c); err != nil {
			t.Fatalf("Create: %v", err)
		}
		const goroutines = 8
		var start sync.WaitGroup
		start.Add(1)
		var done sync.WaitGroup
		done.Add(goroutines)
		results := make(chan error, goroutines)
		for i := 0; i < goroutines; i++ {
			go func(i int) {
				defer done.Done()
				start.Wait()
				results <- s.Redeem(ctx, Redemption{
					Code:        "CONF-ATOMIC",
					OrderID:     "ord-" + itoa(i),
					CustomerID:  "racer-" + itoa(i) + "@example.com",
					AmountMinor: 100,
					Currency:    "USD",
				})
			}(i)
		}
		start.Done()
		done.Wait()
		close(results)
		successes, capHits, other := 0, 0, 0
		for err := range results {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, ErrCodeMaxUsesReached):
				capHits++
			default:
				other++
				t.Errorf("unexpected Redeem error under contention: %v", err)
			}
		}
		if successes != 1 {
			t.Errorf("successful redemptions = %d, want exactly 1 (max_uses=1 atomicity)", successes)
		}
		if capHits != goroutines-1 {
			t.Errorf("ErrCodeMaxUsesReached count = %d, want %d", capHits, goroutines-1)
		}
		got, err := s.Find(ctx, "CONF-ATOMIC")
		if err != nil {
			t.Fatalf("Find: %v", err)
		}
		if got.UsedCount() != 1 {
			t.Errorf("used_count after race = %d, want 1 (no oversubscription)", got.UsedCount())
		}
	})
}

// newPercentCode is a fixture helper for the common case: a percent-off
// code with no validity bounds, no global cap, no per-customer cap. Tests
// that need different settings call domain.NewCode directly.
func newPercentCode(t *testing.T, code string, percent int64) domain.Code {
	t.Helper()
	c, err := domain.NewCode(code, domain.KindPercent, percent, "USD", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("NewCode(%q): %v", code, err)
	}
	return c
}

// filterCodes keeps the rows whose CodeText appears in want. The
// postgres adapter test owns its own table wipe, but having the suite
// filter defensively keeps ListAll's order assertion meaningful even
// when run against a non-empty database by mistake.
func filterCodes(in []domain.Code, want []string) []domain.Code {
	keep := make(map[string]struct{}, len(want))
	for _, w := range want {
		keep[w] = struct{}{}
	}
	out := make([]domain.Code, 0, len(want))
	for _, c := range in {
		if _, ok := keep[c.CodeText()]; ok {
			out = append(out, c)
		}
	}
	return out
}

// itoa is a tiny, allocation-free int->string for sub-test fixtures. The
// production code already has its own copy in domain/code.go for its own
// reasons; replicating it here keeps the test suite free of strconv churn
// and matches the surrounding style.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return strings.Clone(string(buf[i:]))
}
