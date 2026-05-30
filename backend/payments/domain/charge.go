// Package domain holds the payments bounded context's own model of a
// charge attempt.
//
// What is a Charge? In the payments context, a Charge is OUR record of
// a single attempt to move money — independent of what the external
// provider calls the same thing. The provider (see
// internal/fakestripe for the mock) has its own model with its own
// status vocabulary ("requires_action" / "succeeded" / "failed"); the
// payments anti-corruption layer in payments/adapter/fakestripe_acl.go
// translates between the two. The domain therefore stays free of
// any provider-flavoured concepts:
//
//   - Status values are pending / succeeded / failed. The provider's
//     "requires_action" maps to "pending" because, from our point of
//     view, an SCA-required intent is just "not yet decided".
//
//   - ProviderRef is the only place the external identifier lives, and
//     it is opaque to the domain. Subscribers use it as a join key
//     when an asynchronous webhook arrives and needs to find the
//     Charge it pertains to.
//
//   - IdempotencyKey is the caller's retry-safety token. The
//     application service short-circuits on a hit so a re-submission
//     of the same logical charge never produces a second provider
//     call. The corresponding column has a UNIQUE constraint in
//     migration 000039 so a true race still yields one Charge.
//
// Why a Charge is its own value object (and not a column on `order`):
// the payments context owns its own state machine and the same Charge
// may outlive an order in the eyes of the provider (delayed captures,
// disputes, refunds in a real Stripe wire-up). Keeping the data here
// preserves the boundary — checkout asks payments to charge, payments
// owns what "charge" means.
package domain

import "time"

// Status is the payments-domain charge status. Provider statuses
// ("requires_action" etc.) MUST be translated through the ACL before
// reaching this type.
type Status string

const (
	// StatusPending means we have recorded the attempt but the
	// outcome is undetermined: either the provider returned an
	// in-flight state (e.g. "requires_action" for SCA) or we have not
	// yet heard back. Webhook callbacks move pending charges into a
	// terminal state.
	StatusPending Status = "pending"

	// StatusSucceeded means the provider confirmed the funds moved.
	// Terminal.
	StatusSucceeded Status = "succeeded"

	// StatusFailed means the provider declined or otherwise refused
	// the charge. Terminal.
	StatusFailed Status = "failed"
)

// Charge is the immutable value object representing one row in
// payments_charge. Mutation is by replacement: callers re-build a new
// Charge value when a status transition needs to be persisted (see
// app.Service.MarkSucceeded / MarkFailed). The struct's fields are
// unexported; constructors NewCharge / RebuildCharge enforce the
// invariants the table column constraints would catch later.
type Charge struct {
	id             string
	idempotencyKey string
	amount         int64
	currency       string
	status         Status
	providerRef    string
	createdAt      time.Time
	updatedAt      time.Time
}

// NewCharge constructs a fresh pending Charge. providerRef is empty
// at this point — it is filled in once the provider returns a
// reference. idempotencyKey may be blank, but for the Place flow it is
// always derived from the order id (so two attempts for the same
// order can't double-charge).
func NewCharge(id, idempotencyKey string, amount int64, currency string, at time.Time) Charge {
	return Charge{
		id:             id,
		idempotencyKey: idempotencyKey,
		amount:         amount,
		currency:       currency,
		status:         StatusPending,
		providerRef:    "",
		createdAt:      at,
		updatedAt:      at,
	}
}

// RebuildCharge is the storage seam: adapters rehydrate Charges from
// rows without having to expose setters for every field. The function
// is deliberately separate from NewCharge so the constructor path
// stays narrow (status defaults to pending; createdAt/updatedAt come
// from the clock).
func RebuildCharge(id, idempotencyKey string, amount int64, currency string, status Status, providerRef string, createdAt, updatedAt time.Time) Charge {
	return Charge{
		id:             id,
		idempotencyKey: idempotencyKey,
		amount:         amount,
		currency:       currency,
		status:         status,
		providerRef:    providerRef,
		createdAt:      createdAt,
		updatedAt:      updatedAt,
	}
}

// WithStatus returns a copy of the charge with status / providerRef
// replaced. The caller passes the new updatedAt so test clocks stay in
// control. Used by the application service when settling a pending
// charge against the provider's reply or a later webhook.
func (c Charge) WithStatus(status Status, providerRef string, at time.Time) Charge {
	c.status = status
	if providerRef != "" {
		c.providerRef = providerRef
	}
	c.updatedAt = at
	return c
}

// Accessors. Exported as methods (not fields) so the struct stays a
// proper value object — callers must go through the constructors to
// build one.
func (c Charge) ID() string             { return c.id }
func (c Charge) IdempotencyKey() string { return c.idempotencyKey }
func (c Charge) Amount() int64          { return c.amount }
func (c Charge) Currency() string       { return c.currency }
func (c Charge) Status() Status         { return c.status }
func (c Charge) ProviderRef() string    { return c.providerRef }
func (c Charge) CreatedAt() time.Time   { return c.createdAt }
func (c Charge) UpdatedAt() time.Time   { return c.updatedAt }
