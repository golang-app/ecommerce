// Package domain holds the promo bounded context's value objects: the
// catalogue Code (an admin-created discount entry) and the resolved Discount
// (the per-order, per-subtotal amount actually applied at checkout). The
// math lives on Code.Apply so the same calculation drives both the live
// checkout and any later replay / preview.
package domain

import (
	"errors"
	"math"
	"strings"
	"time"
)

// Kind is the discount mechanic. The three values map 1:1 to the
// `promo_kind` enum in migration 000026_promo_codes.
type Kind string

const (
	// KindPercent reduces the subtotal by a whole-percent value in [1, 100].
	KindPercent Kind = "percent"
	// KindFixed subtracts a fixed amount (in minor units) from the
	// subtotal, capped so the discounted subtotal is never below zero.
	KindFixed Kind = "fixed"
	// KindFreeShipping zeroes the shipping cost regardless of the
	// configured free-shipping threshold; it never touches the subtotal.
	KindFreeShipping Kind = "free_shipping"
)

// MaxCodeLength bounds the user-facing code text. Postgres has no length
// limit on `text`, but a sensible cap stops the form from accepting
// pathologically long inputs and keeps the admin list table tidy.
const MaxCodeLength = 64

// ErrInvalidCode is returned by NewCode when the supplied parameters fail
// validation (empty code, out-of-range percent, etc.).
var ErrInvalidCode = errors.New("invalid promo code")

// Code is a catalogue entry an admin created. The fields cover the three
// supported discount mechanics plus the validity window and the per-code /
// per-customer usage limits.
type Code struct {
	code           string
	kind           Kind
	valueMinor     int64
	currency       string
	validFrom      *time.Time
	validUntil     *time.Time
	maxUses        int
	perCustomerMax int
	usedCount      int
	createdAt      time.Time
}

// NewCode validates and constructs a freshly minted Code (used_count
// defaults to 0, created_at defaults to time.Now). validFrom/validUntil
// may both be nil — that is "always valid". maxUses=0 and perCustomerMax=0
// each mean "unlimited"; the default per-customer cap chosen by the caller
// is documented in the admin UI as "1".
func NewCode(code string, kind Kind, valueMinor int64, currency string, validFrom, validUntil *time.Time, maxUses, perCustomerMax int) (Code, error) {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return Code{}, ErrInvalidCode
	}
	if len(trimmed) > MaxCodeLength {
		return Code{}, ErrInvalidCode
	}
	switch kind {
	case KindPercent:
		if valueMinor < 1 || valueMinor > 100 {
			return Code{}, ErrInvalidCode
		}
	case KindFixed:
		if valueMinor <= 0 {
			return Code{}, ErrInvalidCode
		}
		if strings.TrimSpace(currency) == "" {
			return Code{}, ErrInvalidCode
		}
	case KindFreeShipping:
		// value/currency are ignored — normalise them so the persisted
		// row is consistent regardless of what the admin entered.
		valueMinor = 0
	default:
		return Code{}, ErrInvalidCode
	}
	if maxUses < 0 || perCustomerMax < 0 {
		return Code{}, ErrInvalidCode
	}
	if validFrom != nil && validUntil != nil && validUntil.Before(*validFrom) {
		return Code{}, ErrInvalidCode
	}
	if strings.TrimSpace(currency) == "" {
		currency = "USD"
	}
	return Code{
		code:           trimmed,
		kind:           kind,
		valueMinor:     valueMinor,
		currency:       strings.ToUpper(strings.TrimSpace(currency)),
		validFrom:      validFrom,
		validUntil:     validUntil,
		maxUses:        maxUses,
		perCustomerMax: perCustomerMax,
		createdAt:      time.Now().UTC(),
	}, nil
}

// RebuildCode reconstructs a Code from storage. It bypasses validation so
// adapters can hydrate exactly what the database returned (including legacy
// rows that pre-date a tightened invariant).
func RebuildCode(code string, kind Kind, valueMinor int64, currency string, validFrom, validUntil *time.Time, maxUses, perCustomerMax, usedCount int, createdAt time.Time) Code {
	return Code{
		code:           code,
		kind:           kind,
		valueMinor:     valueMinor,
		currency:       currency,
		validFrom:      validFrom,
		validUntil:     validUntil,
		maxUses:        maxUses,
		perCustomerMax: perCustomerMax,
		usedCount:      usedCount,
		createdAt:      createdAt,
	}
}

// Getters — value object, fields are immutable from the outside.
func (c Code) CodeText() string         { return c.code }
func (c Code) Kind() Kind               { return c.kind }
func (c Code) ValueMinor() int64        { return c.valueMinor }
func (c Code) Currency() string         { return c.currency }
func (c Code) ValidFrom() *time.Time    { return c.validFrom }
func (c Code) ValidUntil() *time.Time   { return c.validUntil }
func (c Code) MaxUses() int             { return c.maxUses }
func (c Code) PerCustomerMax() int      { return c.perCustomerMax }
func (c Code) UsedCount() int           { return c.usedCount }
func (c Code) CreatedAt() time.Time     { return c.createdAt }

// KindDisplay returns a human-friendly label for the admin list.
func (c Code) KindDisplay() string {
	switch c.kind {
	case KindPercent:
		return "percent"
	case KindFixed:
		return "fixed"
	case KindFreeShipping:
		return "free shipping"
	default:
		return string(c.kind)
	}
}

// ValueDisplay renders the value column in the admin list according to the
// code's kind: "10%" for percent, "5.00 USD" for fixed, and "—" for free
// shipping.
func (c Code) ValueDisplay() string {
	switch c.kind {
	case KindPercent:
		return itoa(c.valueMinor) + "%"
	case KindFixed:
		return money(c.valueMinor) + " " + c.currency
	default:
		return "—"
	}
}

// IsActiveAt reports whether the code is currently within its validity
// window. nil bounds are open (a one-sided window is allowed).
func (c Code) IsActiveAt(t time.Time) bool {
	if c.validFrom != nil && t.Before(*c.validFrom) {
		return false
	}
	if c.validUntil != nil && t.After(*c.validUntil) {
		return false
	}
	return true
}

// MaxUsesReached reports whether the global redemption cap is hit. A
// maxUses of 0 means unlimited.
func (c Code) MaxUsesReached() bool {
	return c.maxUses > 0 && c.usedCount >= c.maxUses
}

// Discount is the resolved discount for a specific subtotal + shipping
// cost. It is the output of Code.Apply and the input the checkout pricing
// math reads.
type Discount struct {
	code         string
	kind         Kind
	amountMinor  int64
	currency     string
	freeShipping bool
}

// NewDiscount is exported so the app service can construct an "empty" no-op
// discount (no code applied) when the checkout call did not carry one.
func NewDiscount(code string, kind Kind, amountMinor int64, currency string, freeShipping bool) Discount {
	return Discount{code: code, kind: kind, amountMinor: amountMinor, currency: currency, freeShipping: freeShipping}
}

// Code returns the literal code text (empty when no discount was applied).
func (d Discount) Code() string         { return d.code }
func (d Discount) Kind() Kind           { return d.kind }
func (d Discount) AmountMinor() int64   { return d.amountMinor }
func (d Discount) Currency() string     { return d.currency }
func (d Discount) FreeShipping() bool   { return d.freeShipping }
func (d Discount) Empty() bool          { return d.code == "" && d.amountMinor == 0 && !d.freeShipping }

// Apply derives a Discount for the given subtotal/shipping. The math is
// pure and stays in the domain so it can be replayed deterministically
// from the event log.
//
//   - percent: amount = round(subtotal * value / 100), freeShipping=false
//   - fixed: amount = min(value, subtotal) so the subtotal is never negative
//   - free_shipping: amount = 0, freeShipping=true (shipping is zeroed by
//     the checkout pricing math when this flag is set)
func (c Code) Apply(subtotal int64, shippingCost int64) Discount {
	d := Discount{code: c.code, kind: c.kind, currency: c.currency}
	switch c.kind {
	case KindPercent:
		if subtotal <= 0 {
			return d
		}
		d.amountMinor = int64(math.Round(float64(subtotal) * float64(c.valueMinor) / 100.0))
	case KindFixed:
		if subtotal <= 0 {
			return d
		}
		amt := c.valueMinor
		if amt > subtotal {
			amt = subtotal
		}
		d.amountMinor = amt
	case KindFreeShipping:
		d.freeShipping = true
		// shippingCost is reported for symmetry with the other cases —
		// the checkout pricing math is the actual seat of the
		// "shipping = 0" decision.
		_ = shippingCost
	}
	return d
}

// money formats an amount in minor units as "X.YY". Local copy avoids a
// dependency on the checkout package; the format is identical.
func money(amount int64) string {
	if amount < 0 {
		amount = -amount
	}
	return itoa(amount/100) + "." + pad2(amount%100)
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
