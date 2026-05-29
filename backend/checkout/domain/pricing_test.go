package domain_test

import (
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

// TestPriceQuote covers the pricing domain service's interesting cases.
// The math order is documented on PriceQuote itself; this table exercises
// each branch in isolation plus the clamp invariant.
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
		policy   domain.PricingPolicy
		want     domain.Quote
	}{
		{
			name:     "no discount no tax no free-shipping",
			lines:    lines(),
			method:   courier,
			discount: domain.DiscountInput{},
			policy:   domain.PricingPolicy{},
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
			policy:   domain.PricingPolicy{TaxRatePercent: 10},
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
			policy:   domain.PricingPolicy{TaxRatePercent: 10},
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
			policy:   domain.PricingPolicy{FreeShippingThreshold: 4000},
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
			policy:   domain.PricingPolicy{},
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
			policy:   domain.PricingPolicy{TaxRatePercent: 10},
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
			// "no override needed" path stays internally consistent.
			name:     "pickup method costs nothing without free-shipping flag",
			lines:    lines(),
			method:   pickup,
			discount: domain.DiscountInput{},
			policy:   domain.PricingPolicy{},
			want: domain.Quote{
				Subtotal:       5000,
				DiscountAmount: 0,
				Tax:            0,
				ShippingCost:   0,
				Total:          5000,
				FreeShipping:   false,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.PriceQuote(tc.lines, tc.method, tc.discount, tc.policy)
			if got != tc.want {
				t.Errorf("PriceQuote = %+v\n want %+v", got, tc.want)
			}
		})
	}
}
