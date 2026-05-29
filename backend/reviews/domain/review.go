// Package domain holds the reviews bounded-context value objects: a Review
// (a single rating + body left by a verified buyer) and an Aggregate (average
// + count used for storefront badges). Reviews are authored once per
// (customer, product); admins soft-delete (the unique index is partial and
// ignores deleted rows so a buyer can re-review after a removal).
package domain

import (
	"errors"
	"strings"
	"time"
)

// ErrInvalidReview is returned by NewReview when any of its inputs fail
// validation (empty ids, rating out of range, empty/oversized body).
var ErrInvalidReview = errors.New("invalid review")

// maxBodyLen caps the body so a single review can't grow without bound
// (matches the storefront textarea expectations).
const maxBodyLen = 4000

// Status models the moderation lifecycle of a review. Fresh submissions
// always land as StatusPending; admins flip them to StatusApproved or
// StatusRejected from the moderation queue. The set is closed (validated
// by NewReview) and mirrors the DB CHECK constraint on reviews_review.status.
type Status string

const (
	// StatusPending is the initial state of any new submission — invisible
	// to the storefront until an admin approves it.
	StatusPending Status = "pending"
	// StatusApproved is the visible state — counted by the aggregate and
	// listed on the product page.
	StatusApproved Status = "approved"
	// StatusRejected is the terminal "not visible" state — the row stays
	// in storage (the partial unique index still blocks resubmission until
	// an admin soft-deletes it) but is hidden from the storefront.
	StatusRejected Status = "rejected"
)

// IsValid reports whether s is one of the three known statuses. Used by the
// constructor to reject typo'd inputs from callers.
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusApproved, StatusRejected:
		return true
	}
	return false
}

// Review is the value object persisted in reviews_review. createdAt is set
// by the application layer at submission time so the in-memory and postgres
// adapters agree on ordering; deletedAt is nil for active reviews and stamped
// by SoftDelete. status models the moderation lifecycle.
type Review struct {
	id         string
	productID  string
	customerID string
	rating     int
	body       string
	createdAt  time.Time
	deletedAt  *time.Time
	status     Status
}

// NewReview validates inputs and returns a freshly-built Review. The body is
// trimmed before length validation so whitespace-only submissions are
// rejected. The createdAt comes from the caller (the service) so tests can
// pin time. The status is supplied by the caller (the application service)
// so moderation policy lives in one place — Submit always passes
// StatusPending today.
func NewReview(id, productID, customerID, body string, rating int, createdAt time.Time, status Status) (Review, error) {
	if strings.TrimSpace(id) == "" {
		return Review{}, errors.New("review id cannot be empty")
	}
	if strings.TrimSpace(productID) == "" {
		return Review{}, errors.New("product id cannot be empty")
	}
	if strings.TrimSpace(customerID) == "" {
		return Review{}, errors.New("customer id cannot be empty")
	}
	if rating < 1 || rating > 5 {
		return Review{}, errors.New("rating must be between 1 and 5")
	}
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return Review{}, errors.New("review body cannot be empty")
	}
	if len(trimmed) > maxBodyLen {
		return Review{}, errors.New("review body is too long")
	}
	if !status.IsValid() {
		return Review{}, errors.New("review status is invalid")
	}
	return Review{
		id:         id,
		productID:  productID,
		customerID: customerID,
		rating:     rating,
		body:       trimmed,
		createdAt:  createdAt,
		status:     status,
	}, nil
}

// RebuildReview reconstructs a Review from storage rows (skipping validation,
// since the row has already been validated once). deletedAt may be nil.
func RebuildReview(id, productID, customerID, body string, rating int, createdAt time.Time, deletedAt *time.Time, status Status) Review {
	return Review{
		id:         id,
		productID:  productID,
		customerID: customerID,
		rating:     rating,
		body:       body,
		createdAt:  createdAt,
		deletedAt:  deletedAt,
		status:     status,
	}
}

func (r Review) ID() string            { return r.id }
func (r Review) ProductID() string     { return r.productID }
func (r Review) CustomerID() string    { return r.customerID }
func (r Review) Rating() int           { return r.rating }
func (r Review) Body() string          { return r.body }
func (r Review) CreatedAt() time.Time  { return r.createdAt }
func (r Review) DeletedAt() *time.Time { return r.deletedAt }
func (r Review) IsDeleted() bool       { return r.deletedAt != nil }
func (r Review) Status() Status        { return r.status }

// IsApproved is the convenience guard the storefront uses to decide whether
// a review participates in the public list / aggregate. It's the only state
// that is visible to non-admin users.
func (r Review) IsApproved() bool { return r.status == StatusApproved }

// Aggregate is the per-product summary used on the product page (e.g. the
// "★★★★☆ 4.2 (12)" badge). Count is the number of active reviews; Average is
// the mean rating across them. When Count is 0 Average is 0 (the badge is
// hidden by the template).
type Aggregate struct {
	productID string
	average   float64
	count     int
}

// RebuildAggregate reconstructs an Aggregate from a storage row (the avg /
// count returned by AggregateForProducts).
func RebuildAggregate(productID string, avg float64, count int) Aggregate {
	return Aggregate{productID: productID, average: avg, count: count}
}

func (a Aggregate) ProductID() string { return a.productID }
func (a Aggregate) Average() float64  { return a.average }
func (a Aggregate) Count() int        { return a.count }
