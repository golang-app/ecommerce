// Package domain holds the repricing bounded context's value object: a
// Reprice record models the long-running "bulk reprice category by N%"
// operation as a state-stored Process Manager.
//
// PROCESS MANAGER PATTERN
//
// The reprice is a long-running workflow that walks every product in a
// category and applies a percentage change to its base price. The unit
// of work — one product's UpdateProduct call — is small, but the batch
// can be hundreds of items long; a naive "do it all in one HTTP
// request" implementation would either hold an admin browser open for
// minutes or fail half-way through with no way to resume.
//
// The Reprice value object plus the application service's RunActive
// method form a saga:
//
//   - The state-stored Reprice row carries the total item count, the
//     count of items processed so far, and the last per-item error.
//   - Each item's processing increments processedItems and bumps the
//     row's version (optimistic concurrency); a partial failure on
//     one item is recorded in lastError but the saga continues.
//   - On a process restart, the in-progress Reprice row is still on
//     disk: a future call to RunActive picks up from processedItems
//     without re-applying the price change to items it already touched.
//
// The 'one active at a time' invariant is enforced both in the
// application service (Start refuses when FindActive returns a row)
// and at the database level via a partial unique index on the status
// column — belt and braces, with the DB constraint being the last
// line of defence against two concurrent admins clicking the button
// at the same time.
//
// The shape mirrors the fulfillment context's state-stored aggregate
// (different store, different events, different ubiquitous language)
// so a reader who has internalised one finds the other immediately
// familiar.
package domain

import (
	"errors"
	"fmt"
	"time"
)

// Status is the operational stage of a Reprice. The four states form
// a linear lifecycle (scheduled -> in_progress -> completed) with a
// failure terminal reachable from in_progress when the saga aborts
// hard (e.g. ctx cancelled and the operator chose to fail the run
// rather than resume).
type Status string

const (
	// StatusScheduled is the initial state: Start has created the
	// row but the goroutine that walks the items has not yet
	// transitioned it.
	StatusScheduled Status = "scheduled"
	// StatusInProgress means the saga is actively iterating the
	// planned items. A process restart leaves the row in this state
	// so a future RunActive call can resume from processedItems.
	StatusInProgress Status = "in_progress"
	// StatusCompleted is the happy-path terminal state: every
	// planned item has been processed (with or without per-item
	// errors recorded in lastError).
	StatusCompleted Status = "completed"
	// StatusFailed is the failure terminal: an unrecoverable error
	// (storage down, ctx cancelled with no resume desired) ended
	// the run.
	StatusFailed Status = "failed"
)

// ErrInvalidReprice is returned by NewReprice when its arguments
// fail validation (empty id/category, non-positive total,
// out-of-range percent). Wrapped via fmt.Errorf("...: %w", ...) so
// callers can branch with errors.Is.
var ErrInvalidReprice = errors.New("repricing: invalid reprice arguments")

// ErrInvalidTransition is returned by Start / RecordItemProcessed /
// RecordItemFailed / Complete / Fail when the current status does
// not permit the requested command.
var ErrInvalidTransition = errors.New("repricing: invalid state transition")

// maxAbsPercent bounds the percent_change argument to a sensible
// range so an admin slip ("set to +200%") does not silently nuke the
// catalogue. The bound is symmetric and deliberately conservative;
// loosening it is a config / migration concern, not a hot-fix.
const maxAbsPercent = 50.0

// Reprice is the state-stored aggregate root for one bulk-reprice
// operation. Construct fresh records via NewReprice; rebuild
// existing rows via Rebuild. The value object is immutable in
// spirit: the command methods mutate the receiver only after
// validating the transition.
type Reprice struct {
	id             string
	categoryID     string
	percentChange  float64
	status         Status
	totalItems     int
	processedItems int
	lastError      string
	startedAt      time.Time
	completedAt    *time.Time
	version        int
}

// NewReprice constructs a fresh, unsaved Reprice in the
// StatusScheduled state.
func NewReprice(id, categoryID string, percentChange float64, totalItems int, at time.Time) (Reprice, error) {
	if id == "" {
		return Reprice{}, fmt.Errorf("empty id: %w", ErrInvalidReprice)
	}
	if categoryID == "" {
		return Reprice{}, fmt.Errorf("empty categoryID: %w", ErrInvalidReprice)
	}
	if totalItems <= 0 {
		return Reprice{}, fmt.Errorf("totalItems must be positive: %w", ErrInvalidReprice)
	}
	if percentChange < -maxAbsPercent || percentChange > maxAbsPercent {
		return Reprice{}, fmt.Errorf("percentChange out of range: %w", ErrInvalidReprice)
	}
	if percentChange == 0 {
		// A zero-percent reprice would be a no-op disguised as a
		// long-running workflow. Reject it so the admin's foot-gun
		// stays visible.
		return Reprice{}, fmt.Errorf("percentChange cannot be zero: %w", ErrInvalidReprice)
	}
	return Reprice{
		id:            id,
		categoryID:    categoryID,
		percentChange: percentChange,
		status:        StatusScheduled,
		totalItems:    totalItems,
		startedAt:     at,
		version:       1,
	}, nil
}

// Rebuild reconstructs a Reprice from a storage row. No transitions
// are applied — this is purely for reading a persisted record back
// into memory.
func Rebuild(
	id, categoryID string,
	percentChange float64,
	status Status,
	totalItems, processedItems int,
	lastError string,
	startedAt time.Time,
	completedAt *time.Time,
	version int,
) Reprice {
	return Reprice{
		id:             id,
		categoryID:     categoryID,
		percentChange:  percentChange,
		status:         status,
		totalItems:     totalItems,
		processedItems: processedItems,
		lastError:      lastError,
		startedAt:      startedAt,
		completedAt:    completedAt,
		version:        version,
	}
}

// ID returns the record's stable identifier.
func (r Reprice) ID() string { return r.id }

// CategoryID returns the catalogue category slug the saga targets.
func (r Reprice) CategoryID() string { return r.categoryID }

// PercentChange is the signed multiplier applied to every product's
// base price (e.g. -10.0 means "drop every price by 10%").
func (r Reprice) PercentChange() float64 { return r.percentChange }

// Status returns the current lifecycle stage.
func (r Reprice) Status() Status { return r.status }

// TotalItems is the count of products planned at Start time.
func (r Reprice) TotalItems() int { return r.totalItems }

// ProcessedItems is the count of products RunActive has already
// updated (incremented after each per-item Update).
func (r Reprice) ProcessedItems() int { return r.processedItems }

// LastError is the per-item error message recorded by the most
// recent RecordItemFailed (empty when no item has failed yet).
func (r Reprice) LastError() string { return r.lastError }

// StartedAt is when the row was first created.
func (r Reprice) StartedAt() time.Time { return r.startedAt }

// CompletedAt is when the row reached Complete or Fail (nil while
// still active).
func (r Reprice) CompletedAt() *time.Time { return r.completedAt }

// Version is the optimistic-concurrency counter incremented on every
// successful transition.
func (r Reprice) Version() int { return r.version }

// ProgressFraction returns processedItems / totalItems as a value
// between 0 and 1. The admin UI uses this to render a progress bar.
func (r Reprice) ProgressFraction() float64 {
	if r.totalItems == 0 {
		return 0
	}
	return float64(r.processedItems) / float64(r.totalItems)
}

// Start transitions a scheduled reprice to in_progress. This is the
// transition RunActive applies the first time it picks up the row;
// resuming after a restart finds the row already in_progress and
// skips this command.
func (r *Reprice) Start() error {
	if r.status != StatusScheduled {
		return ErrInvalidTransition
	}
	r.status = StatusInProgress
	r.version++
	return nil
}

// RecordItemProcessed increments processedItems and bumps the
// version. The `at` argument is unused today but reserved so a
// future "rate-limit / pacing" feature can record per-item
// timestamps without changing the call site.
func (r *Reprice) RecordItemProcessed(at time.Time) error {
	if r.status != StatusInProgress {
		return ErrInvalidTransition
	}
	_ = at
	r.processedItems++
	r.version++
	return nil
}

// RecordItemFailed records a per-item error message without
// aborting the saga. The processed counter still advances — the
// item is considered "done" from the saga's point of view; the
// error is recorded so the admin can see something went wrong.
func (r *Reprice) RecordItemFailed(errMsg string, at time.Time) error {
	if r.status != StatusInProgress {
		return ErrInvalidTransition
	}
	_ = at
	r.lastError = errMsg
	r.processedItems++
	r.version++
	return nil
}

// Complete transitions an in-progress reprice to completed. Callers
// MUST have processed every planned item before calling this; the
// guard rejects an early Complete so the saga can't be marked done
// while there is still work outstanding.
func (r *Reprice) Complete(at time.Time) error {
	if r.status != StatusInProgress {
		return ErrInvalidTransition
	}
	if r.processedItems < r.totalItems {
		return ErrInvalidTransition
	}
	r.status = StatusCompleted
	completedAt := at
	r.completedAt = &completedAt
	r.version++
	return nil
}

// Fail transitions any active state (scheduled or in_progress) to
// failed. The errMsg overwrites lastError so the operator sees the
// fatal reason on the detail page.
func (r *Reprice) Fail(errMsg string, at time.Time) error {
	switch r.status {
	case StatusScheduled, StatusInProgress:
		// allowed
	default:
		return ErrInvalidTransition
	}
	r.status = StatusFailed
	r.lastError = errMsg
	completedAt := at
	r.completedAt = &completedAt
	r.version++
	return nil
}
