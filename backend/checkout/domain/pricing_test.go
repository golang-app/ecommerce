package domain_test

import (
	"strings"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

// perLineTaxStrategy is a test-local TaxStrategy that demonstrates the
// pluggability of the interface: it taxes only the lines whose product
// name starts with "Tax:" at a flat rate, ignoring the rest of the
// basket. Real-world per-line strategies follow the same shape
// (jurisdictional categories, VAT rates per product class, etc.).
type perLineTaxStrategy struct {
	ratePercent float64
}

func (s perLineTaxStrategy) Tax(lines []domain.Line, _ int64) int64 {
	if s.ratePercent <= 0 {
		return 0
	}
	var taxable int64
	for _, ln := range lines {
		if strings.HasPrefix(ln.ProductName(), "Tax:") {
			taxable += ln.LineTotal()
		}
	}
	if taxable <= 0 {
		return 0
	}
	// Truncating division keeps the test arithmetic predictable; the
	// production strategy in domain rounds to the nearest minor unit.
	return taxable * int64(s.ratePercent) / 100
}

// TestPriceQuote covers the pricing domain service's interesting cases.
// The math order is documented on PriceQuote itself; this table exercises
// each branch in isolation plus the clamp invariant, with the default
// FlatTaxStrategy + ThresholdShippingStrategy that the composition root
// wires in production.
func TestPriceQuote(t *testing.T) {
	lines := func() []domain.Line {
		// 2 * 1500 + 1 * 2000 = 5000 subtotal.
		return []domain.Line{
			domain.NewLine("v1", "Mug", 2, 1500, "USD"),
			domain.NewLine("v2", "Plate", 1, 2000, "USD"),
		}
	}
	courier := domain.RebuildShippingMethod("courier", "Courier", 1500)
	pickup := domain.RebuildShippingMethod("pickup", "Personal pickup", 0)

	tests := []struct {
		name     string
		lines    []domain.Line
		method   domain.ShippingMethod
		discount domain.DiscountInput
		tax      domain.TaxStrategy
		ship     domain.ShippingStrategy
		want     domain.Quote
	}{
		{
			name:     "no discount no tax no free-shipping",
			lines:    lines(),
			method:   courier,
			discount: domain.DiscountInput{},
			tax:      domain.FlatTaxStrategy{},
			ship:     domain.ThresholdShippingStrategy{},
			want: domain.Quote{
				Subtotal:       5000,
				DiscountAmount: 0,
				Tax:            0,
				ShippingCost:   1500,
				Total:          6500,
				FreeShipping:   false,
			},
		},
		{
			// 10% of 5000 = 500.
			name:     "percent tax only",
			lines:    lines(),
			method:   courier,
			discount: domain.DiscountInput{},
			tax:      domain.FlatTaxStrategy{RatePercent: 10},
			ship:     domain.ThresholdShippingStrategy{},
			want: domain.Quote{
				Subtotal:       5000,
				DiscountAmount: 0,
				Tax:            500,
				ShippingCost:   1500,
				Total:          7000,
				FreeShipping:   false,
			},
		},
		{
			// 1000 off, 10% tax of remaining 4000 = 400.
			name:     "fixed discount applied before tax",
			lines:    lines(),
			method:   courier,
			discount: domain.DiscountInput{AmountMinor: 1000},
			tax:      domain.FlatTaxStrategy{RatePercent: 10},
			ship:     domain.ThresholdShippingStrategy{},
			want: domain.Quote{
				Subtotal:       5000,
				DiscountAmount: 1000,
				Tax:            400,
				ShippingCost:   1500,
				Total:          5900,
				FreeShipping:   false,
			},
		},
		{
			// Threshold 4000 — discounted subtotal 5000 clears it, shipping is zeroed.
			name:     "free shipping via threshold",
			lines:    lines(),
			method:   courier,
			discount: domain.DiscountInput{},
			tax:      domain.FlatTaxStrategy{},
			ship:     domain.ThresholdShippingStrategy{FreeShippingThreshold: 4000},
			want: domain.Quote{
				Subtotal:       5000,
				DiscountAmount: 0,
				Tax:            0,
				ShippingCost:   0,
				Total:          5000,
				FreeShipping:   true,
			},
		},
		{
			// Promo flag wins regardless of the threshold being unset.
			name:     "free shipping via discount flag",
			lines:    lines(),
			method:   courier,
			discount: domain.DiscountInput{FreeShipping: true},
			tax:      domain.FlatTaxStrategy{},
			ship:     domain.ThresholdShippingStrategy{},
			want: domain.Quote{
				Subtotal:       5000,
				DiscountAmount: 0,
				Tax:            0,
				ShippingCost:   0,
				Total:          5000,
				FreeShipping:   true,
			},
		},
		{
			// Discount larger than subtotal must clamp; total can't go negative
			// and tax on a zero discounted subtotal is zero too.
			name:     "discount greater than subtotal clamps to subtotal",
			lines:    lines(),
			method:   courier,
			discount: domain.DiscountInput{AmountMinor: 9_999_999},
			tax:      domain.FlatTaxStrategy{RatePercent: 10},
			ship:     domain.ThresholdShippingStrategy{},
			want: domain.Quote{
				Subtotal:       5000,
				DiscountAmount: 5000,
				Tax:            0,
				ShippingCost:   1500,
				Total:          1500,
				FreeShipping:   false,
			},
		},
		{
			// Pickup is free shipping by virtue of Cost()==0 — assert the
			// "no override needed" path stays internally consistent. The
			// quote's FreeShipping flag stays false because the rule
			// didn't fire; the method just costs nothing.
			name:     "pickup method costs nothing without free-shipping flag",
			lines:    lines(),
			method:   pickup,
			discount: domain.DiscountInput{},
			tax:      domain.FlatTaxStrategy{},
			ship:     domain.ThresholdShippingStrategy{},
			want: domain.Quote{
				Subtotal:       5000,
				DiscountAmount: 0,
				Tax:            0,
				ShippingCost:   0,
				Total:          5000,
				FreeShipping:   false,
			},
		},
		{
			// Per-line tax demo: only lines whose name starts with "Tax:"
			// are taxed, at 20%. The "Mug" + "Plate" basket above has no
			// such lines, so we build a bespoke basket here.
			//   Tax:Wine (1 * 1000) is taxed at 20% = 200.
			//   Bread (2 * 500 = 1000) is not.
			// Total taxable = 1000, tax = 200. Subtotal = 2000.
			// Shipping = courier 1500. Total = 2000 + 200 + 1500 = 3700.
			name:   "per-line tax strategy taxes only matching lines",
			method: courier,
			lines: []domain.Line{
				domain.NewLine("v3", "Tax:Wine", 1, 1000, "USD"),
				domain.NewLine("v4", "Bread", 2, 500, "USD"),
			},
			discount: domain.DiscountInput{},
			tax:      perLineTaxStrategy{ratePercent: 20},
			ship:     domain.ThresholdShippingStrategy{},
			want: domain.Quote{
				Subtotal:       2000,
				DiscountAmount: 0,
				Tax:            200,
				ShippingCost:   1500,
				Total:          3700,
				FreeShipping:   false,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.PriceQuote(tc.lines, tc.method, tc.discount, tc.tax, tc.ship)
			if got != tc.want {
				t.Errorf("PriceQuote = %+v\n want %+v", got, tc.want)
			}
		})
	}
}
