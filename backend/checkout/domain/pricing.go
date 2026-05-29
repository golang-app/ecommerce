// Pricing is a domain service: pure math that doesn't belong to any
// aggregate. It lives in the domain package because it speaks the domain's
// vocabulary (Line, ShippingMethod) and has no infrastructure dependencies.
//
// The split keeps the Order aggregate focused on its lifecycle (place,
// pay, cancel, ship, ...) and the application service (checkout/app)
// focused on orchestration (load cart, reserve stock, charge, save).
// The maths — subtotal, discount clamp, tax on the discounted subtotal,
// free-shipping override, total — lives here so it is independently
// testable and easy to reason about.
package domain

import "math"

// Quote is the value object returned by PriceQuote: every line a checkout
// summary or order confirmation can render. Currency is implicit — the
// caller derives it from the order's lines and pairs it with these
// numeric amounts. All fields are in minor units (e.g. cents).
type Quote struct {
	// Subtotal is the sum of every Line.LineTotal() before any discount,
	// tax or shipping is applied.
	Subtotal int64
	// DiscountAmount is the resolved promo discount actually subtracted
	// from the subtotal, clamped so the discounted subtotal never goes
	// negative.
	DiscountAmount int64
	// Tax is computed on the discounted subtotal so the customer is not
	// charged tax on a discount they never paid.
	Tax int64
	// ShippingCost is the effective shipping price: 0 when free shipping
	// applies (either the promo flag or the configured threshold),
	// otherwise the chosen ShippingMethod's Cost().
	ShippingCost int64
	// Total is discountedSubtotal + Tax + ShippingCost.
	Total int64
	// FreeShipping records whether the shipping cost was zeroed by the
	// pricing rules — useful for the summary line ("Free shipping
	// applied"). Set when either the promo's FreeShipping flag is true
	// or the configured FreeShippingThreshold was reached.
	FreeShipping bool
}

// PricingPolicy carries the configurable inputs to the pricing math:
// the flat tax rate and the free-shipping threshold. Both default to
// zero, which disables the corresponding rule (no tax, no automatic
// free-shipping override) — matching the historical behaviour before
// either was wired.
type PricingPolicy struct {
	// TaxRatePercent is the flat tax rate applied to the discounted
	// subtotal, e.g. 8.875 for 8.875%. 0 disables tax.
	TaxRatePercent float64
	// FreeShippingThreshold is the minimum (discounted) subtotal at or
	// above which the chosen method's shipping cost is overridden to 0.
	// 0 disables the override.
	FreeShippingThreshold int64
}

// DiscountInput is what the pricing service needs from a resolved
// discount. It is deliberately a small, local value type so the pricing
// math does not depend on the promo bounded context's types — the
// application service translates from promo.Discount into DiscountInput.
type DiscountInput struct {
	// AmountMinor is the discount in minor units. Negative values are
	// treated as zero; values larger than the subtotal are clamped to
	// the subtotal so the discounted subtotal cannot go negative.
	AmountMinor int64
	// FreeShipping zeroes the effective shipping cost regardless of the
	// configured threshold.
	FreeShipping bool
}

// PriceQuote is the domain service entry point. It is a pure function:
// given the line snapshots, the chosen shipping method, the resolved
// discount and the configured policy, it returns the complete Quote.
// The math, in order:
//
//  1. subtotal = sum of Line.LineTotal() over every line.
//  2. discount = min(max(DiscountInput.AmountMinor, 0), subtotal) — clamp
//     so the discounted subtotal cannot go negative.
//  3. discountedSubtotal = subtotal - discount.
//  4. tax = round(discountedSubtotal * policy.TaxRatePercent / 100).
//  5. freeShipping = DiscountInput.FreeShipping OR
//     (policy.FreeShippingThreshold > 0 AND
//     discountedSubtotal >= policy.FreeShippingThreshold).
//  6. shippingCost = 0 when freeShipping, else shipMethod.Cost().
//  7. total = discountedSubtotal + tax + shippingCost.
func PriceQuote(lines []Line, shipMethod ShippingMethod, discount DiscountInput, policy PricingPolicy) Quote {
	var subtotal int64
	for _, ln := range lines {
		subtotal += ln.LineTotal()
	}

	discountAmount := discount.AmountMinor
	if discountAmount < 0 {
		discountAmount = 0
	}
	if discountAmount > subtotal {
		discountAmount = subtotal
	}
	discountedSubtotal := subtotal - discountAmount

	var tax int64
	if policy.TaxRatePercent > 0 && discountedSubtotal > 0 {
		tax = int64(math.Round(float64(discountedSubtotal) * policy.TaxRatePercent / 100.0))
	}

	freeShipping := discount.FreeShipping ||
		(policy.FreeShippingThreshold > 0 && discountedSubtotal >= policy.FreeShippingThreshold)

	var shippingCost int64
	if !freeShipping {
		shippingCost = shipMethod.Cost()
	}

	return Quote{
		Subtotal:       subtotal,
		DiscountAmount: discountAmount,
		Tax:            tax,
		ShippingCost:   shippingCost,
		Total:          discountedSubtotal + tax + shippingCost,
		FreeShipping:   freeShipping,
	}
}
