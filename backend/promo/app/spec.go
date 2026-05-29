package app

// This file implements the Specification pattern for the promo-code
// eligibility rules. Each rule is a tiny exported struct whose
// IsSatisfiedBy method inspects an EligibilityContext and returns nil when
// the rule passes or the matching sentinel error when it fails.
//
// Why return an error instead of a bool? Service.Resolve needs to tell the
// caller WHICH rule rejected the code (the checkout handler maps each
// sentinel to a different flash message), not merely "something failed".
// A bool would force a parallel error-lookup step; returning the sentinel
// directly keeps the rule and its rejection reason in one place.
//
// Specifications are composed with And — the composite short-circuits on
// the first non-nil error so we never spend a DB round-trip on a rule that
// can't change the outcome. New eligibility policies can be assembled by
// composing the existing specs (or new ones) without touching Service.

import (
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/promo/domain"
)

// EligibilityContext bundles every input a Specification might inspect.
// Service.Resolve builds it once (a single CountRedemptionsByCustomer DB
// call) and hands the same value to each spec, so the composed evaluation
// is read-only and side-effect-free.
type EligibilityContext struct {
	Code                domain.Code
	CustomerID          string
	Subtotal            int64
	ShippingCost        int64
	Now                 time.Time
	CustomerRedemptions int
}

// Specification is the rule contract. Implementations return nil when the
// rule is satisfied or the matching sentinel error (ErrCodeAnonymous,
// ErrCodeExpired, …) when it is violated.
type Specification interface {
	IsSatisfiedBy(ctx EligibilityContext) error
}

// NotAnonymous rejects empty CustomerIDs. Per-customer caps need a stable
// identity to be meaningful so anonymous use is refused outright.
type NotAnonymous struct{}

// IsSatisfiedBy returns ErrCodeAnonymous when CustomerID is empty.
func (NotAnonymous) IsSatisfiedBy(ctx EligibilityContext) error {
	if ctx.CustomerID == "" {
		return ErrCodeAnonymous
	}
	return nil
}

// WithinValidityWindow guards both ends of the validity window. nil bounds
// are open: a code with no validFrom is "valid since forever" and a code
// with no validUntil is "valid until further notice".
type WithinValidityWindow struct{}

// IsSatisfiedBy returns ErrCodeExpired when Now is outside [validFrom,
// validUntil].
func (WithinValidityWindow) IsSatisfiedBy(ctx EligibilityContext) error {
	if !ctx.Code.IsActiveAt(ctx.Now) {
		return ErrCodeExpired
	}
	return nil
}

// UnderMaxUses enforces the global redemption cap. MaxUses == 0 means
// unlimited and always passes.
type UnderMaxUses struct{}

// IsSatisfiedBy returns ErrCodeMaxUsesReached when the global cap is hit.
func (UnderMaxUses) IsSatisfiedBy(ctx EligibilityContext) error {
	if ctx.Code.MaxUsesReached() {
		return ErrCodeMaxUsesReached
	}
	return nil
}

// UnderPerCustomerLimit enforces the per-customer redemption cap.
// PerCustomerMax == 0 means unlimited and always passes; the caller is
// expected to have populated CustomerRedemptions on the context.
type UnderPerCustomerLimit struct{}

// IsSatisfiedBy returns ErrCodeCustomerLimit when this customer's tally
// has reached the configured per-customer cap.
func (UnderPerCustomerLimit) IsSatisfiedBy(ctx EligibilityContext) error {
	if ctx.Code.PerCustomerMax() > 0 && ctx.CustomerRedemptions >= ctx.Code.PerCustomerMax() {
		return ErrCodeCustomerLimit
	}
	return nil
}

// andSpec is the AND combinator: it evaluates its children in order and
// returns the first non-nil error. The short-circuit is intentional —
// callers compose cheap predicates ahead of expensive ones so a failing
// up-front check never reaches a DB-touching predicate further down.
type andSpec struct {
	specs []Specification
}

// IsSatisfiedBy walks the children and returns the first error encountered.
func (a andSpec) IsSatisfiedBy(ctx EligibilityContext) error {
	for _, s := range a.specs {
		if err := s.IsSatisfiedBy(ctx); err != nil {
			return err
		}
	}
	return nil
}

// And composes the supplied specifications into a single Specification.
// The composite short-circuits on the first failing rule.
func And(specs ...Specification) Specification {
	return andSpec{specs: specs}
}

// defaultEligibility is the policy Service.Resolve enforces. New policies
// can be assembled by composing these specs (or new ones) without
// changing Service — the package comment documents the extension point.
var defaultEligibility = And(
	NotAnonymous{},
	WithinValidityWindow{},
	UnderMaxUses{},
	UnderPerCustomerLimit{},
)
