package domain_test

import (
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

func TestVariant_StockPredicates(t *testing.T) {
	price := domain.MustNewPrice(1000, domain.MustNewCurrency("USD"))
	cases := []struct {
		stock       int
		wantInStock bool
		wantLow     bool
	}{
		{0, false, false},  // out of stock is never "low"
		{1, true, true},    // just above empty
		{5, true, true},    // at the threshold
		{6, true, false},   // above the threshold
		{100, true, false}, // plenty
	}
	for _, c := range cases {
		v := domain.NewVariant("v1", "SKU", "", nil, price, c.stock)
		if v.InStock() != c.wantInStock {
			t.Errorf("stock %d: InStock = %v, want %v", c.stock, v.InStock(), c.wantInStock)
		}
		if v.LowStock() != c.wantLow {
			t.Errorf("stock %d: LowStock = %v, want %v", c.stock, v.LowStock(), c.wantLow)
		}
	}
}

func TestVariant_NilOptionsBecomeEmptyMap(t *testing.T) {
	price := domain.MustNewPrice(1000, domain.MustNewCurrency("USD"))
	v := domain.NewVariant("v1", "SKU", "", nil, price, 1)
	if v.Options() == nil {
		t.Error("Options should be a non-nil empty map, not nil")
	}
	if len(v.Options()) != 0 {
		t.Errorf("Options len = %d, want 0", len(v.Options()))
	}
}

func TestVariant_LabelFollowsOptionTypeOrder(t *testing.T) {
	price := domain.MustNewPrice(1000, domain.MustNewCurrency("USD"))
	v := domain.NewVariant("v1", "SKU", "", map[string]string{
		"Size":  "L",
		"Color": "Red",
	}, price, 1)

	// Option types define display order: Color first, then Size.
	optionTypes := []domain.OptionType{
		domain.NewOptionType("Color", []string{"Red", "Blue"}),
		domain.NewOptionType("Size", []string{"S", "L"}),
	}
	if got := v.Label(optionTypes); got != "Red / L" {
		t.Errorf("Label = %q, want %q", got, "Red / L")
	}

	// A default variant (no options) has an empty label.
	def := domain.NewVariant("v0", "SKU0", "", nil, price, 1)
	if got := def.Label(optionTypes); got != "" {
		t.Errorf("default variant Label = %q, want empty", got)
	}
}

func TestVariant_ZeroValueIsZero(t *testing.T) {
	if !(domain.Variant{}).IsZero() {
		t.Error("zero Variant should be IsZero")
	}
}

func TestNewPrice_Validation(t *testing.T) {
	usd := domain.MustNewCurrency("USD")
	if _, err := domain.NewPrice(-1, usd); err == nil {
		t.Error("negative amount should be rejected")
	}
	if _, err := domain.NewPrice(100, ""); err == nil {
		t.Error("empty currency should be rejected")
	}
	p := domain.MustNewPrice(1234, usd)
	if p.Display() != "12.34" {
		t.Errorf("Display = %q, want 12.34", p.Display())
	}
}

func TestNewCurrency_Validation(t *testing.T) {
	if _, err := domain.NewCurrency("usd"); err == nil {
		t.Error("lowercase currency should be rejected")
	}
	if _, err := domain.NewCurrency("US"); err == nil {
		t.Error("two-letter currency should be rejected")
	}
	if _, err := domain.NewCurrency("USD"); err != nil {
		t.Errorf("USD should be valid: %v", err)
	}
}
