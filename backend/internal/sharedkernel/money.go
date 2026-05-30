// Package sharedkernel hosts the small DDD Shared Kernel: a published common
// language shared by multiple bounded contexts in this codebase. Types here
// are intentionally tiny, dependency-free, and stable — every owning context
// pays a coordination cost when they change, so changes must stay rare and
// backwards-compatible.
//
// Money is the kernel's first inhabitant. It replaces the
// (int64 minor units, string currency) pairs that callers were forced to
// thread through hand-in-hand. Wrapping the pair in a value object gives us
// three things the loose pair could not:
//
//  1. compile-time prevention of cross-currency arithmetic (Add/Sub/Compare
//     return an error when currencies disagree),
//  2. a single place to enforce ISO 4217 currency validation, and
//  3. a single rounding policy for percent math (MulFloat uses math.Round).
//
// Adoption is intentionally gradual. The checkout bounded context exposes
// Money-returning getters ALONGSIDE the existing int64+currency accessors
// in this PR; other contexts continue to use the loose pairs at their
// boundaries and convert at the edge. See the package README for the
// migration plan and the rationale for keeping the storage layer on int64
// for now.
package sharedkernel

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// ErrInvalidMoney is returned when constructing a Money with an invalid
// currency (anything other than three uppercase ASCII letters per the
// ISO 4217 alphabetic code shape).
var ErrInvalidMoney = errors.New("invalid money")

// ErrCurrencyMismatch is returned by every binary operation when the two
// operands carry different currencies. Cross-currency arithmetic requires
// an explicit FX conversion that this kernel deliberately does not provide.
var ErrCurrencyMismatch = errors.New("currency mismatch")

// Currency is an ISO 4217 three-letter alphabetic code (e.g. "EUR", "USD").
// The string is treated as opaque — Money never reasons about the code's
// minor-unit exponent or symbol. Display formatting (currency placement,
// thousands separator, locale) lives in the presentation layer; this
// package's Display() returns only "X.YY".
type Currency string

// String returns the underlying ISO code so a Currency interpolates as the
// raw three-letter symbol in log lines and error messages.
func (c Currency) String() string { return string(c) }

// Validate enforces the ISO 4217 alphabetic shape: exactly three uppercase
// ASCII letters. It does NOT check the code against the live ISO register —
// callers stay free to use test currencies in unit tests.
func (c Currency) Validate() error {
	s := string(c)
	if len(s) != 3 {
		return fmt.Errorf("%w: currency %q must be 3 letters", ErrInvalidMoney, s)
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < 'A' || ch > 'Z' {
			return fmt.Errorf("%w: currency %q must be uppercase ASCII letters", ErrInvalidMoney, s)
		}
	}
	return nil
}

// Money is an immutable value object: an amount in minor units (e.g. cents)
// paired with the currency it is denominated in. Fields are unexported so the
// only way to obtain a Money is via the constructors below, which guarantees
// every Money in the system carries a validated currency.
//
// All operations are value-receiver: they never mutate the receiver and
// always return a new Money.
type Money struct {
	amount   int64
	currency Currency
}

// NewMoney constructs a Money after validating the currency. It is the
// preferred constructor at boundaries where the currency originated from
// untrusted input (HTTP form, database row, external event).
func NewMoney(amount int64, currency Currency) (Money, error) {
	if err := currency.Validate(); err != nil {
		return Money{}, err
	}
	return Money{amount: amount, currency: currency}, nil
}

// MustNewMoney is the panicking counterpart of NewMoney. Use it only when
// the currency is a compile-time constant or has already been validated
// upstream — e.g. inside aggregate getters that know the stored currency
// passed validation when the aggregate was constructed.
func MustNewMoney(amount int64, currency Currency) Money {
	m, err := NewMoney(amount, currency)
	if err != nil {
		panic(err)
	}
	return m
}

// Zero returns a Money worth nothing in the supplied currency. Useful as
// an additive identity in summation loops and as the default value when a
// field is optional (e.g. an order with no discount).
func Zero(currency Currency) Money {
	return MustNewMoney(0, currency)
}

// Amount returns the value in minor units (e.g. cents). Persistence layers
// pair this with Currency() to round-trip Money through int64 columns
// without a custom SQL type.
func (m Money) Amount() int64 { return m.amount }

// Currency returns the ISO 4217 code Money is denominated in.
func (m Money) Currency() Currency { return m.currency }

// IsZero reports whether the amount is exactly 0 (independent of currency).
// A zero-EUR and a zero-USD both report true.
func (m Money) IsZero() bool { return m.amount == 0 }

// Add returns m + other. It rejects cross-currency operands with
// ErrCurrencyMismatch — adding 10 EUR to 5 USD is meaningless without an
// FX rate.
func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.currency, other.currency)
	}
	return Money{amount: m.amount + other.amount, currency: m.currency}, nil
}

// Sub returns m - other. It rejects cross-currency operands, same as Add.
func (m Money) Sub(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.currency, other.currency)
	}
	return Money{amount: m.amount - other.amount, currency: m.currency}, nil
}

// Mul scales the amount by an integer factor. The currency is preserved, so
// there is no error path. Negative factors flip the sign — same as Negate
// when factor is -1.
func (m Money) Mul(factor int64) Money {
	return Money{amount: m.amount * factor, currency: m.currency}
}

// MulFloat scales the amount by a float factor and rounds the result via
// math.Round (banker's rounding is NOT used — see the README for the
// rounding-policy rationale). This is the percent-math hook used for tax
// and discount lines.
func (m Money) MulFloat(factor float64) Money {
	return Money{
		amount:   int64(math.Round(float64(m.amount) * factor)),
		currency: m.currency,
	}
}

// Compare returns -1, 0 or 1 when m is less than, equal to or greater than
// other respectively. It rejects cross-currency operands — there is no
// total order across currencies without an FX rate.
func (m Money) Compare(other Money) (int, error) {
	if m.currency != other.currency {
		return 0, fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.currency, other.currency)
	}
	switch {
	case m.amount < other.amount:
		return -1, nil
	case m.amount > other.amount:
		return 1, nil
	default:
		return 0, nil
	}
}

// Negate returns -m, preserving the currency.
func (m Money) Negate() Money {
	return Money{amount: -m.amount, currency: m.currency}
}

// Display renders the amount as a "X.YY" minor-units string with no
// currency suffix. Templates pair this with the currency code so the
// presentation layer keeps full control over symbol placement and locale.
// Negative amounts render with a leading "-" (e.g. "-1.50").
func (m Money) Display() string {
	a := m.amount
	sign := ""
	if a < 0 {
		sign = "-"
		a = -a
	}
	return fmt.Sprintf("%s%d.%02d", sign, a/100, a%100)
}

// String renders Money as "X.YY CCY" for logs and error messages. It is
// distinct from Display() on purpose: log lines want the currency, the
// storefront templates do not.
func (m Money) String() string {
	return strings.TrimSpace(m.Display() + " " + string(m.currency))
}
