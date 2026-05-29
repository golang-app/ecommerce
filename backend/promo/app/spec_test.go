package app_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/promo/app"
	"github.com/bkielbasa/go-ecommerce/backend/promo/domain"
)

// fixedNow is the "current time" used by every spec unit test. It sits
// well inside any open-ended window so the validity-window suite only
// rejects on bounds we deliberately push past it.
var fixedNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func TestNotAnonymous(t *testing.T) {
	t.Parallel()
	spec := app.NotAnonymous{}

	if err := spec.IsSatisfiedBy(app.EligibilityContext{CustomerID: ""}); !errors.Is(err, app.ErrCodeAnonymous) {
		t.Errorf("empty customer: err = %v, want ErrCodeAnonymous", err)
	}
	if err := spec.IsSatisfiedBy(app.EligibilityContext{CustomerID: "jane@example.com"}); err != nil {
		t.Errorf("non-empty customer: err = %v, want nil", err)
	}
}

func TestWithinValidityWindow(t *testing.T) {
	t.Parallel()
	past := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	type tc struct {
		name      string
		from, to  *time.Time
		wantError error
	}
	cases := []tc{
		{"no bounds", nil, nil, nil},
		{"future-only from rejects", &future, nil, app.ErrCodeExpired},
		{"past-only from passes", &past, nil, nil},
		{"past-only until rejects", nil, &past, app.ErrCodeExpired},
		{"future-only until passes", nil, &future, nil},
		{"open window with both bounds", &past, &future, nil},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			code, err := domain.NewCode("CODE", domain.KindPercent, 5, "USD", c.from, c.to, 0, 0)
			if err != nil {
				t.Fatalf("NewCode: %v", err)
			}
			got := app.WithinValidityWindow{}.IsSatisfiedBy(app.EligibilityContext{Code: code, Now: fixedNow})
			if !errors.Is(got, c.wantError) {
				t.Errorf("err = %v, want %v", got, c.wantError)
			}
		})
	}
}

func TestUnderMaxUses(t *testing.T) {
	t.Parallel()

	// Unlimited (maxUses == 0) always passes regardless of usedCount.
	unlimited := domain.RebuildCode("U", domain.KindPercent, 5, "USD", nil, nil, 0, 0, 999, time.Now())
	if err := (app.UnderMaxUses{}).IsSatisfiedBy(app.EligibilityContext{Code: unlimited}); err != nil {
		t.Errorf("unlimited: err = %v, want nil", err)
	}

	// Capped but not yet hit.
	below := domain.RebuildCode("B", domain.KindPercent, 5, "USD", nil, nil, 5, 0, 4, time.Now())
	if err := (app.UnderMaxUses{}).IsSatisfiedBy(app.EligibilityContext{Code: below}); err != nil {
		t.Errorf("below cap: err = %v, want nil", err)
	}

	// Capped and exactly at the limit.
	atCap := domain.RebuildCode("C", domain.KindPercent, 5, "USD", nil, nil, 5, 0, 5, time.Now())
	if err := (app.UnderMaxUses{}).IsSatisfiedBy(app.EligibilityContext{Code: atCap}); !errors.Is(err, app.ErrCodeMaxUsesReached) {
		t.Errorf("at cap: err = %v, want ErrCodeMaxUsesReached", err)
	}
}

func TestUnderPerCustomerLimit(t *testing.T) {
	t.Parallel()

	// PerCustomerMax == 0 means unlimited; tally is irrelevant.
	unlimited := domain.RebuildCode("U", domain.KindPercent, 5, "USD", nil, nil, 0, 0, 0, time.Now())
	if err := (app.UnderPerCustomerLimit{}).IsSatisfiedBy(app.EligibilityContext{Code: unlimited, CustomerRedemptions: 99}); err != nil {
		t.Errorf("unlimited: err = %v, want nil", err)
	}

	// Capped at 1, customer has never redeemed → passes.
	cap1 := domain.RebuildCode("L", domain.KindPercent, 5, "USD", nil, nil, 0, 1, 0, time.Now())
	if err := (app.UnderPerCustomerLimit{}).IsSatisfiedBy(app.EligibilityContext{Code: cap1, CustomerRedemptions: 0}); err != nil {
		t.Errorf("fresh customer: err = %v, want nil", err)
	}

	// Capped at 1, customer already redeemed once → rejects.
	if err := (app.UnderPerCustomerLimit{}).IsSatisfiedBy(app.EligibilityContext{Code: cap1, CustomerRedemptions: 1}); !errors.Is(err, app.ErrCodeCustomerLimit) {
		t.Errorf("over cap: err = %v, want ErrCodeCustomerLimit", err)
	}
}

// stubSpec is a Specification that returns a preset error and records that
// it was evaluated. It exists so the composition tests can prove And
// short-circuits without leaking state from the production specs.
type stubSpec struct {
	err    error
	called *bool
}

func (s stubSpec) IsSatisfiedBy(_ app.EligibilityContext) error {
	if s.called != nil {
		*s.called = true
	}
	return s.err
}

func TestAnd_ShortCircuitsOnFirstFailure(t *testing.T) {
	t.Parallel()
	var firstCalled, secondCalled, thirdCalled bool
	want := errors.New("boom")
	spec := app.And(
		stubSpec{err: nil, called: &firstCalled},
		stubSpec{err: want, called: &secondCalled},
		stubSpec{err: nil, called: &thirdCalled},
	)
	got := spec.IsSatisfiedBy(app.EligibilityContext{})
	if !errors.Is(got, want) {
		t.Fatalf("err = %v, want %v", got, want)
	}
	if !firstCalled {
		t.Errorf("first spec should have run")
	}
	if !secondCalled {
		t.Errorf("second spec should have run (it's the failure)")
	}
	if thirdCalled {
		t.Errorf("third spec must NOT run after a failure (And must short-circuit)")
	}
}

func TestAnd_AllSatisfiedReturnsNil(t *testing.T) {
	t.Parallel()
	spec := app.And(
		stubSpec{err: nil},
		stubSpec{err: nil},
		stubSpec{err: nil},
	)
	if err := spec.IsSatisfiedBy(app.EligibilityContext{}); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

func TestAnd_EmptyReturnsNil(t *testing.T) {
	t.Parallel()
	// And() with no specs is the identity element: nothing to violate.
	if err := app.And().IsSatisfiedBy(app.EligibilityContext{}); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}
