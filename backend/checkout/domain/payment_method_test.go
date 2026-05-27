package domain_test

import (
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

func TestPaymentMethodByCode(t *testing.T) {
	cases := []struct {
		code         string
		requiresCard bool
	}{
		{"card", true},
		{"paypal", false},
		{"cod", false},
	}
	for _, c := range cases {
		m, err := domain.PaymentMethodByCode(c.code)
		if err != nil {
			t.Fatalf("PaymentMethodByCode(%q): %v", c.code, err)
		}
		if m.RequiresCard() != c.requiresCard {
			t.Errorf("%q RequiresCard = %v, want %v", c.code, m.RequiresCard(), c.requiresCard)
		}
	}
}

func TestPaymentMethodByCode_Invalid(t *testing.T) {
	_, err := domain.PaymentMethodByCode("barter")
	if !errors.Is(err, domain.ErrInvalidPaymentMethod) {
		t.Errorf("err = %v, want ErrInvalidPaymentMethod", err)
	}
}

func TestRebuildPaymentMethod_RequiresCardFromCode(t *testing.T) {
	if !domain.RebuildPaymentMethod("card", "Credit / debit card").RequiresCard() {
		t.Error("rebuilt card method should require a card")
	}
	if domain.RebuildPaymentMethod("cod", "Cash on delivery").RequiresCard() {
		t.Error("rebuilt cod method should not require a card")
	}
}

func TestPaymentMethods_ReturnsDefensiveCopy(t *testing.T) {
	got := domain.PaymentMethods()
	if len(got) == 0 {
		t.Fatal("expected at least one payment method")
	}
	got[0] = domain.PaymentMethod{}
	if domain.PaymentMethods()[0].IsZero() {
		t.Error("mutating the returned slice leaked into the catalogue")
	}
}
