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
//
// # The Strategy pattern
//
// The two policy-driven steps of the math — computing tax and computing
// the effective shipping cost — used to be baked into PriceQuote as
// "flat percent on the discounted subtotal" and "flat method cost,
// zeroed above a configurable free-shipping threshold". Both are fine
// defaults but they are also only two points in a much larger space:
//
//   - Tax: VAT-inclusive jurisdictions (price already contains the tax),
//     per-line categories (food vs alcohol vs digital goods), destination-
//     based US sales tax, B2B reverse-charge, etc.
//   - Shipping: weight-based couriers, per-jurisdiction surcharges,
//     promotional free-shipping windows ("free shipping on Wednesdays"),
//     free pickup at a store, dimensional weight, etc.
//
// Rather than grow the PricingPolicy struct with one more boolean every
// quarter, the two policies are lifted behind small Strategy interfaces
// (TaxStrategy, ShippingStrategy). PriceQuote orchestrates the order of
// operations and delegates the two policy-dependent calculations to the
// injected strategies; the composition root (cmd/web) decides which
// concrete strategy gets wired.
//
// Adding a new strategy is a three-step change with no edits to
// PriceQuote, the Order aggregate, or the application service:
//
//  1. Implement the relevant interface in a new type (anywhere — inside
//     this package for shared defaults, or in an adapter package for a
//     vendor-specific calculator).
//  2. Add the configuration the new strategy needs to the composition
//     root (cmd/web reads cfg, builds the strategy value, passes it to
//     NewCheckoutService).
//  3. Optionally add a dedicated test exercising the new behaviour —
//     PriceQuote's tests already lock down the orchestration; the new
//     strategy only needs to cover its own math.
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

// TaxStrategy computes the tax due on a discounted subtotal.
// Implementations may inspect the lines (e.g. per-line VAT categories
// such as food vs alcohol) or apply a single flat rate. Returning 0
// means "no tax" — that is also the historical zero-value behaviour the
// composition root falls back to when no rate is configured.
type TaxStrategy interface {
	Tax(lines []Line, discountedSubtotal int64) int64
}

// ShippingStrategy computes the effective shipping cost for an order.
// It receives the chosen ShippingMethod (with its catalogue cost), the
// discounted subtotal, the per-line basket, and a flag indicating
// whether the resolved discount itself includes free shipping (the
// promo's FreeShipping flag). Returning 0 means "free shipping",
// whatever the reason — the FreeShipping field on the resulting Quote
// is set accordingly.
type ShippingStrategy interface {
	Cost(lines []Line, shipMethod ShippingMethod, discountedSubtotal int64, freeShippingFromDiscount bool) int64
}

// FlatTaxStrategy applies a single rate (percent) to the discounted
// subtotal. It is the historical default that the composition root
// builds from cfg.TaxRatePercent — a zero rate yields zero tax, matching
// the pre-strategy behaviour.
type FlatTaxStrategy struct {
	// RatePercent is the flat tax rate applied to the discounted
	// subtotal, e.g. 8.875 for 8.875%. 0 disables tax.
	RatePercent float64
}

// Tax returns round(discountedSubtotal * RatePercent / 100). A
// non-positive subtotal yields 0 so a fully-discounted basket is
// charged no tax. The result rounds to the nearest minor unit.
func (s FlatTaxStrategy) Tax(_ []Line, discountedSubtotal int64) int64 {
	if s.RatePercent <= 0 || discountedSubtotal <= 0 {
		return 0
	}
	return int64(math.Round(float64(discountedSubtotal) * s.RatePercent / 100.0))
}

// ThresholdShippingStrategy is the historical shipping behaviour: a
// flat cost taken from the chosen method, zeroed when the discounted
// subtotal exceeds a configurable free-shipping threshold or when the
// discount itself includes free shipping. The composition root builds
// it from cfg.FreeShippingThreshold — a zero threshold disables the
// override entirely, matching the pre-strategy behaviour.
type ThresholdShippingStrategy struct {
	// FreeShippingThreshold is the minimum (discounted) subtotal at or
	// above which the chosen method's shipping cost is overridden to 0.
	// 0 disables the override.
	FreeShippingThreshold int64
}

// Cost returns 0 when free shipping applies (either the discount flag
// or the configured threshold being reached), otherwise the chosen
// method's catalogue Cost().
func (s ThresholdShippingStrategy) Cost(_ []Line, sm ShippingMethod, discountedSubtotal int64, freeShippingFromDiscount bool) int64 {
	if freeShippingFromDiscount {
		return 0
	}
	if s.FreeShippingThreshold > 0 && discountedSubtotal >= s.FreeShippingThreshold {
		return 0
	}
	return sm.Cost()
}

// PriceQuote is the domain service entry point. It is a pure function:
// given the line snapshots, the chosen shipping method, the resolved
// discount and the two pluggable strategies, it returns the complete
// Quote. The math, in order:
//
//  1. subtotal = sum of Line.LineTotal() over every line.
//  2. discount = min(max(DiscountInput.AmountMinor, 0), subtotal) — clamp
//     so the discounted subtotal cannot go negative.
//  3. discountedSubtotal = subtotal - discount.
//  4. tax = tax.Tax(lines, discountedSubtotal).
//  5. shippingCost = ship.Cost(lines, shipMethod, discountedSubtotal,
//     discount.FreeShipping).
//  6. freeShipping = (shippingCost == 0 AND shipMethod.Cost() > 0) OR
//     discount.FreeShipping. Pickup-style methods whose catalogue cost
//     is already 0 are not flagged as "free shipping applied" — that
//     summary line is reserved for the cases where the pricing rules
//     actively zeroed a chargeable method.
//  7. total = discountedSubtotal + tax + shippingCost.
func PriceQuote(lines []Line, shipMethod ShippingMethod, discount DiscountInput, tax TaxStrategy, ship ShippingStrategy) Quote {
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

	var taxAmount int64
	if tax != nil {
		taxAmount = tax.Tax(lines, discountedSubtotal)
	}

	var shippingCost int64
	if ship != nil {
		shippingCost = ship.Cost(lines, shipMethod, discountedSubtotal, discount.FreeShipping)
	} else {
		shippingCost = shipMethod.Cost()
	}

	// freeShipping reports whether the pricing rules actively zeroed a
	// chargeable method. A pickup-style method whose catalogue Cost()
	// is already 0 is NOT flagged here — the summary line "Free
	// shipping applied" would be misleading.
	freeShipping := discount.FreeShipping ||
		(shippingCost == 0 && shipMethod.Cost() > 0)

	return Quote{
		Subtotal:       subtotal,
		DiscountAmount: discountAmount,
		Tax:            taxAmount,
		ShippingCost:   shippingCost,
		Total:          discountedSubtotal + taxAmount + shippingCost,
		FreeShipping:   freeShipping,
	}
}
