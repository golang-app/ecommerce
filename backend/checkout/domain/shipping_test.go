package domain_test

import (
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

func TestShippingMethodByCode(t *testing.T) {
	cases := []struct {
		code            string
		wantCost        int64
		requiresAddress bool
	}{
		{"flat", 500, true},
		{"pickup", 0, false},
		{"courier", 1500, true},
	}
	for _, c := range cases {
		m, err := domain.ShippingMethodByCode(c.code)
		if err != nil {
			t.Fatalf("ShippingMethodByCode(%q): %v", c.code, err)
		}
		if m.Cost() != c.wantCost {
			t.Errorf("%q cost = %d, want %d", c.code, m.Cost(), c.wantCost)
		}
		if m.RequiresAddress() != c.requiresAddress {
			t.Errorf("%q RequiresAddress = %v, want %v", c.code, m.RequiresAddress(), c.requiresAddress)
		}
		if m.IsZero() {
			t.Errorf("%q should not be zero", c.code)
		}
	}
}

func TestShippingMethodByCode_Invalid(t *testing.T) {
	_, err := domain.ShippingMethodByCode("teleport")
	if !errors.Is(err, domain.ErrInvalidShippingMethod) {
		t.Errorf("err = %v, want ErrInvalidShippingMethod", err)
	}
}

func TestShippingMethods_ReturnsDefensiveCopy(t *testing.T) {
	got := domain.ShippingMethods()
	if len(got) == 0 {
		t.Fatal("expected at least one shipping method")
	}
	got[0] = domain.ShippingMethod{} // mutate the returned slice
	again := domain.ShippingMethods()
	if again[0].IsZero() {
		t.Error("mutating the returned slice leaked into the catalogue")
	}
}

func TestRebuildShippingMethod_RequiresAddressFromCode(t *testing.T) {
	pickup := domain.RebuildShippingMethod("pickup", "Personal pickup", 0)
	if pickup.RequiresAddress() {
		t.Error("rebuilt pickup should not require an address")
	}
	courier := domain.RebuildShippingMethod("courier", "Courier", 1500)
	if !courier.RequiresAddress() {
		t.Error("rebuilt courier should require an address")
	}
}

func TestShippingMethod_CostDisplay(t *testing.T) {
	m := domain.RebuildShippingMethod("courier", "Courier", 1500)
	if got := m.CostDisplay(); got != "15.00" {
		t.Errorf("CostDisplay = %q, want %q", got, "15.00")
	}
}

func TestShippingMethod_ZeroValueIsZero(t *testing.T) {
	if !(domain.ShippingMethod{}).IsZero() {
		t.Error("zero ShippingMethod should be IsZero")
	}
}
