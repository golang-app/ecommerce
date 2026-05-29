// Package domain is the fulfillment bounded context's heart: the
// Fulfillment value object and the small state machine that drives it
// through the operational lifecycle of an order (scheduled → labeled →
// shipped → delivered, with a refund branch terminal from any active
// state).
//
// PROCESS MANAGER PATTERN
//
// Why does this context exist at all when the checkout context's Order
// aggregate already carries MarkShipped / MarkDelivered / Refund?
//
// Two different lifecycles were entangled on one aggregate:
//
//  1. Commercial state — pending → paid (or failed / cancelled). Owned by
//     finance: did we ever charge the customer's card?
//  2. Operational state — scheduled → labeled → shipped → delivered (or
//     refunded). Owned by the warehouse: where is the box?
//
// Conflating both on `Order` worked for a single-team demo but it ties
// changes to the shipping workflow to the checkout/event log, makes the
// state machine harder to reason about (transitions and rejections are
// status-by-status guards instead of one table), and prevents the
// fulfillment workflow from evolving on its own cadence.
//
// The Fulfillment Process Manager fixes that:
//
//   - It is a SEPARATE bounded context (different store, different
//     events, different ubiquitous language). The Order aggregate keeps
//     its old methods for replay / back-compat but the admin UI no
//     longer drives them.
//   - It SUBSCRIBES to the published OrderPaid integration event and
//     spawns a Fulfillment record in StatusScheduled. From there the
//     warehouse drives it forward via Label / Ship / Deliver / Refund —
//     each transition validated by an explicit allowed-source-state
//     check rather than a sea of status-specific errors.
//   - It is STATE-STORED, not event-sourced. The Order aggregate is
//     event-sourced; the Fulfillment is a row in a table with an
//     optimistic-concurrency `version` column. Demonstrating BOTH
//     persistence styles inside one codebase is intentional: an
//     event-sourced aggregate is the right call when the audit trail
//     matters (money moved), while a state-stored aggregate is fine for
//     a flat operational record — and the read/write code is shorter.
//
// Pending integration events are accumulated on the value object during
// command execution and drained by the service layer once the row has
// committed, so subscribers in other bounded contexts see only events
// that actually persisted.
package domain

import (
	"errors"
	"time"
)

// Status is the operational stage of a Fulfillment. It is deliberately
// distinct from checkout/domain.Status (which carries the commercial
// state); the two state machines run in parallel.
type Status string

const (
	// StatusScheduled is the initial state: OnOrderPaid has just
	// created the record but the warehouse hasn't touched it yet.
	StatusScheduled Status = "scheduled"
	// StatusLabeled means a shipping label has been printed and the
	// carrier + tracking code recorded — the parcel exists but
	// hasn't been collected by the carrier yet.
	StatusLabeled Status = "labeled"
	// StatusShipped means the carrier has accepted the parcel.
	StatusShipped Status = "shipped"
	// StatusDelivered is the happy-path terminal state.
	StatusDelivered Status = "delivered"
	// StatusReturned is reserved for the inbound return flow
	// (parcel arrived back at the warehouse). Not driven by any
	// command in this iteration — the column accepts it so a future
	// "return scanned" flow doesn't need a migration.
	StatusReturned Status = "returned"
	// StatusRefunded is the refund-terminal state, reachable from
	// any active (non-terminal-happy-path) state.
	StatusRefunded Status = "refunded"
)

// ErrInvalidTransition is returned by Schedule / Label / Ship / Deliver
// / Refund when the current status does not permit the requested
// command (e.g. shipping an already-delivered fulfillment).
var ErrInvalidTransition = errors.New("fulfillment: invalid state transition")

// ErrAlreadyExists is returned by Schedule when invoked on a record
// that has already been spawned for the order. OnOrderPaid uses this
// to stay idempotent under outbox redelivery — duplicate OrderPaid
// publishes from the at-least-once dispatcher are silently no-ops.
var ErrAlreadyExists = errors.New("fulfillment: already exists for order")

// Event is an internal domain event raised by a state transition. Each
// event mirrors a command and carries just enough payload for the
// service layer to translate it into an integration event for other
// bounded contexts. Events on this aggregate are NOT persisted (the
// Fulfillment is state-stored, not event-sourced); they exist purely
// as the in-flight bridge to the bus.
type Event interface {
	eventMarker()
}

// FulfillmentScheduled is raised by Schedule.
type FulfillmentScheduled struct {
	ID      string
	OrderID string
	At      time.Time
}

func (FulfillmentScheduled) eventMarker() {}

// FulfillmentLabeled is raised by Label.
type FulfillmentLabeled struct {
	ID           string
	OrderID      string
	Carrier      string
	TrackingCode string
	At           time.Time
}

func (FulfillmentLabeled) eventMarker() {}

// FulfillmentShipped is raised by Ship.
type FulfillmentShipped struct {
	ID      string
	OrderID string
	At      time.Time
}

func (FulfillmentShipped) eventMarker() {}

// FulfillmentDelivered is raised by Deliver.
type FulfillmentDelivered struct {
	ID      string
	OrderID string
	At      time.Time
}

func (FulfillmentDelivered) eventMarker() {}

// FulfillmentRefunded is raised by Refund.
type FulfillmentRefunded struct {
	ID      string
	OrderID string
	Reason  string
	At      time.Time
}

func (FulfillmentRefunded) eventMarker() {}

// Fulfillment is the state-stored aggregate root for one shipment.
// Construct fresh records via NewFulfillment / Schedule; rebuild
// existing rows via Rebuild. The value object is immutable in spirit:
// the command methods mutate the receiver only after validating the
// transition.
type Fulfillment struct {
	id           string
	orderID      string
	status       Status
	carrier      string
	trackingCode string
	scheduledAt  time.Time
	shippedAt    time.Time
	deliveredAt  time.Time
	refundReason string
	version      int

	pendingEvents []Event
}

// NewFulfillment constructs a fresh, unsaved Fulfillment in the
// StatusScheduled state and raises FulfillmentScheduled. Used by the
// service's OnOrderPaid handler — most callers should go through
// Schedule, which is a thin wrapper that takes the same arguments and
// makes the call site read like the rest of the command surface.
func NewFulfillment(id, orderID string, at time.Time) Fulfillment {
	f := Fulfillment{
		id:          id,
		orderID:     orderID,
		status:      StatusScheduled,
		scheduledAt: at,
		version:     1,
	}
	f.pendingEvents = append(f.pendingEvents, FulfillmentScheduled{
		ID:      id,
		OrderID: orderID,
		At:      at,
	})
	return f
}

// Rebuild reconstructs a Fulfillment from a storage row. No events are
// raised — this is purely for reading a persisted record back into
// memory.
func Rebuild(
	id, orderID string,
	status Status,
	carrier, trackingCode string,
	scheduledAt, shippedAt, deliveredAt time.Time,
	refundReason string,
	version int,
) Fulfillment {
	return Fulfillment{
		id:           id,
		orderID:      orderID,
		status:       status,
		carrier:      carrier,
		trackingCode: trackingCode,
		scheduledAt:  scheduledAt,
		shippedAt:    shippedAt,
		deliveredAt:  deliveredAt,
		refundReason: refundReason,
		version:      version,
	}
}

// ID returns the record's stable identifier (separate from OrderID so a
// single order could in principle spawn multiple fulfillments — split
// shipments — without changing the type).
func (f Fulfillment) ID() string { return f.id }

// OrderID returns the checkout order id this fulfillment belongs to.
func (f Fulfillment) OrderID() string { return f.orderID }

// Status returns the current operational stage.
func (f Fulfillment) Status() Status { return f.status }

// Carrier is the chosen carrier label (e.g. "UPS"). Empty until Label
// runs.
func (f Fulfillment) Carrier() string { return f.carrier }

// TrackingCode is the carrier's tracking number. Empty until Label
// runs.
func (f Fulfillment) TrackingCode() string { return f.trackingCode }

// ScheduledAt is when the record was first created (OnOrderPaid).
func (f Fulfillment) ScheduledAt() time.Time { return f.scheduledAt }

// ShippedAt is when Ship was applied; zero time when not yet shipped.
func (f Fulfillment) ShippedAt() time.Time { return f.shippedAt }

// DeliveredAt is when Deliver was applied; zero time when not yet
// delivered.
func (f Fulfillment) DeliveredAt() time.Time { return f.deliveredAt }

// RefundReason is the operator-supplied reason recorded by Refund.
func (f Fulfillment) RefundReason() string { return f.refundReason }

// Version is the optimistic-concurrency counter incremented on every
// successful transition. Storage UPDATEs match on the expected
// version; a mismatch returns ErrOptimisticLock from the adapter so
// callers can retry against a fresh load.
func (f Fulfillment) Version() int { return f.version }

// PendingEvents returns the events raised by command methods since the
// record was loaded (or constructed). The service drains these after a
// successful save and publishes them on the integration bus.
func (f Fulfillment) PendingEvents() []Event { return f.pendingEvents }

// ClearPending discards any uncommitted events. Called by the service
// once they've been handed off to the publisher.
func (f *Fulfillment) ClearPending() { f.pendingEvents = nil }

// Schedule is the constructor-style command paired with NewFulfillment
// — included so the command surface reads symmetrically with the rest
// (Label / Ship / Deliver / Refund all live on the value). Returns
// ErrAlreadyExists when called on a non-zero receiver.
func (f *Fulfillment) Schedule(orderID string, at time.Time) error {
	if f.status != "" {
		return ErrAlreadyExists
	}
	*f = NewFulfillment(f.id, orderID, at)
	return nil
}

// Label records the carrier + tracking code and transitions the
// record from StatusScheduled to StatusLabeled. Empty carrier or
// trackingCode are accepted by the domain (the adapter persists them
// verbatim); enforcing minimum content is the service / HTTP layer's
// job.
func (f *Fulfillment) Label(carrier, trackingCode string, at time.Time) error {
	if f.status != StatusScheduled {
		return ErrInvalidTransition
	}
	f.carrier = carrier
	f.trackingCode = trackingCode
	f.status = StatusLabeled
	f.version++
	f.pendingEvents = append(f.pendingEvents, FulfillmentLabeled{
		ID:           f.id,
		OrderID:      f.orderID,
		Carrier:      carrier,
		TrackingCode: trackingCode,
		At:           at,
	})
	return nil
}

// Ship transitions a labeled record to StatusShipped. The carrier
// MUST have been recorded first (i.e. Label must have run).
func (f *Fulfillment) Ship(at time.Time) error {
	if f.status != StatusLabeled {
		return ErrInvalidTransition
	}
	f.status = StatusShipped
	f.shippedAt = at
	f.version++
	f.pendingEvents = append(f.pendingEvents, FulfillmentShipped{
		ID:      f.id,
		OrderID: f.orderID,
		At:      at,
	})
	return nil
}

// Deliver transitions a shipped record to StatusDelivered (the
// happy-path terminal state).
func (f *Fulfillment) Deliver(at time.Time) error {
	if f.status != StatusShipped {
		return ErrInvalidTransition
	}
	f.status = StatusDelivered
	f.deliveredAt = at
	f.version++
	f.pendingEvents = append(f.pendingEvents, FulfillmentDelivered{
		ID:      f.id,
		OrderID: f.orderID,
		At:      at,
	})
	return nil
}

// Refund transitions any active state (scheduled / labeled / shipped /
// delivered) to StatusRefunded. The refund branch is reachable from
// every non-terminal active state because a refund can be issued at
// any point in the lifecycle: before the label is printed, after the
// parcel ships, or after delivery. Already-refunded fulfillments
// reject the second call so the integration event isn't double-fired.
func (f *Fulfillment) Refund(reason string, at time.Time) error {
	switch f.status {
	case StatusScheduled, StatusLabeled, StatusShipped, StatusDelivered:
		// allowed
	default:
		return ErrInvalidTransition
	}
	f.status = StatusRefunded
	f.refundReason = reason
	f.version++
	f.pendingEvents = append(f.pendingEvents, FulfillmentRefunded{
		ID:      f.id,
		OrderID: f.orderID,
		Reason:  reason,
		At:      at,
	})
	return nil
}
