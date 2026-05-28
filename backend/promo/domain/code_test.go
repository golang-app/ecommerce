package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/promo/domain"
)

func TestNewCode_ValidatesKind(t *testing.T) {
	cases := []struct {
		name    string
		code    string
		kind    domain.Kind
		value   int64
		ccy     string
		wantErr bool
	}{
		{"percent ok", "SAVE10", domain.KindPercent, 10, "USD", false},
		{"percent zero", "ZERO", domain.KindPercent, 0, "USD", true},
		{"percent over 100", "OVER", domain.KindPercent, 150, "USD", true},
		{"fixed ok", "FIVE", domain.KindFixed, 500, "USD", false},
		{"fixed zero", "ZEROFIX", domain.KindFixed, 0, "USD", true},
		{"fixed missing ccy", "BADCCY", domain.KindFixed, 500, "", true},
		{"free shipping ok", "FREESHIP", domain.KindFreeShipping, 0, "", false},
		{"empty code", "  ", domain.KindPercent, 10, "USD", true},
		{"bad kind", "X", domain.Kind("weird"), 1, "USD", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := domain.NewCode(c.code, c.kind, c.value, c.ccy, nil, nil, 0, 1)
			if c.wantErr {
				if !errors.Is(err, domain.ErrInvalidCode) {
					t.Fatalf("want ErrInvalidCode, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestCode_Apply_Percent(t *testing.T) {
	c, err := domain.NewCode("SAVE10", domain.KindPercent, 10, "USD", nil, nil, 0, 1)
	if err != nil {
		t.Fatalf("NewCode: %v", err)
	}
	d := c.Apply(2500, 500)
	if d.AmountMinor() != 250 {
		t.Errorf("amount = %d, want 250", d.AmountMinor())
	}
	if d.FreeShipping() {
		t.Errorf("percent should not flag free shipping")
	}
	if d.Code() != "SAVE10" {
		t.Errorf("code text lost: %q", d.Code())
	}
}

func TestCode_Apply_FixedCapsAtSubtotal(t *testing.T) {
	c, err := domain.NewCode("BIG", domain.KindFixed, 10000, "USD", nil, nil, 0, 1)
	if err != nil {
		t.Fatalf("NewCode: %v", err)
	}
	// subtotal smaller than discount: amount caps at subtotal so the
	// discounted subtotal never goes negative.
	d := c.Apply(500, 0)
	if d.AmountMinor() != 500 {
		t.Errorf("amount = %d, want 500", d.AmountMinor())
	}
}

func TestCode_Apply_FreeShipping(t *testing.T) {
	c, err := domain.NewCode("FREESHIP", domain.KindFreeShipping, 0, "", nil, nil, 0, 1)
	if err != nil {
		t.Fatalf("NewCode: %v", err)
	}
	d := c.Apply(2500, 500)
	if d.AmountMinor() != 0 {
		t.Errorf("amount = %d, want 0", d.AmountMinor())
	}
	if !d.FreeShipping() {
		t.Errorf("free shipping flag not set")
	}
}

func TestCode_IsActiveAt(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	c, err := domain.NewCode("WIN", domain.KindPercent, 5, "USD", &from, &until, 0, 1)
	if err != nil {
		t.Fatalf("NewCode: %v", err)
	}
	if c.IsActiveAt(time.Date(2025, 12, 31, 23, 0, 0, 0, time.UTC)) {
		t.Errorf("before window: should not be active")
	}
	if !c.IsActiveAt(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("inside window: should be active")
	}
	if c.IsActiveAt(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("after window: should not be active")
	}
}
