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
		wantCarrier     string
		wantEnabled     bool
	}{
		{"flat", 500, true, "Standard Post", true},
		{"pickup", 0, false, "Store Pickup", true},
		{"courier", 1500, true, "Express Courier", true},
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
		if m.Carrier() != c.wantCarrier {
			t.Errorf("%q Carrier = %q, want %q", c.code, m.Carrier(), c.wantCarrier)
		}
		if m.IsEnabled() != c.wantEnabled {
			t.Errorf("%q IsEnabled = %v, want %v", c.code, m.IsEnabled(), c.wantEnabled)
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

func TestNewShippingMethod(t *testing.T) {
	m := domain.NewShippingMethod("express", "Express Delivery", 2000, true, "DHL", true)
	if m.Code() != "express" {
		t.Errorf("Code = %q, want %q", m.Code(), "express")
	}
	if m.Label() != "Express Delivery" {
		t.Errorf("Label = %q, want %q", m.Label(), "Express Delivery")
	}
	if m.Cost() != 2000 {
		t.Errorf("Cost = %d, want %d", m.Cost(), 2000)
	}
	if !m.RequiresAddress() {
		t.Errorf("RequiresAddress = false, want true")
	}
	if m.Carrier() != "DHL" {
		t.Errorf("Carrier = %q, want %q", m.Carrier(), "DHL")
	}
	if !m.IsEnabled() {
		t.Errorf("IsEnabled = false, want true")
	}
	if m.CostDisplay() != "20.00" {
		t.Errorf("CostDisplay = %q, want %q", m.CostDisplay(), "20.00")
	}
	if m.IsZero() {
		t.Errorf("IsZero = true, want false")
	}

	disabled := domain.NewShippingMethod("legacy", "Legacy Method", 1000, false, "OldPost", false)
	if disabled.IsEnabled() {
		t.Errorf("IsEnabled = true, want false")
	}
	if disabled.Carrier() != "OldPost" {
		t.Errorf("Carrier = %q, want %q", disabled.Carrier(), "OldPost")
	}
}

func TestRebuildShippingMethod_CarrierAndEnabledDefaults(t *testing.T) {
	m := domain.RebuildShippingMethod("courier", "Courier", 1500)
	if !m.IsEnabled() {
		t.Errorf("rebuilt method should be enabled")
	}
	if m.Carrier() != "" {
		t.Errorf("rebuilt method Carrier = %q, want empty", m.Carrier())
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
