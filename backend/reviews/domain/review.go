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

// Review is the value object persisted in reviews_review. createdAt is set
// by the application layer at submission time so the in-memory and postgres
// adapters agree on ordering; deletedAt is nil for active reviews and stamped
// by SoftDelete.
type Review struct {
	id         string
	productID  string
	customerID string
	rating     int
	body       string
	createdAt  time.Time
	deletedAt  *time.Time
}

// NewReview validates inputs and returns a freshly-built Review. The body is
// trimmed before length validation so whitespace-only submissions are
// rejected. The createdAt comes from the caller (the service) so tests can
// pin time.
func NewReview(id, productID, customerID, body string, rating int, createdAt time.Time) (Review, error) {
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
	return Review{
		id:         id,
		productID:  productID,
		customerID: customerID,
		rating:     rating,
		body:       trimmed,
		createdAt:  createdAt,
	}, nil
}

// RebuildReview reconstructs a Review from storage rows (skipping validation,
// since the row has already been validated once). deletedAt may be nil.
func RebuildReview(id, productID, customerID, body string, rating int, createdAt time.Time, deletedAt *time.Time) Review {
	return Review{
		id:         id,
		productID:  productID,
		customerID: customerID,
		rating:     rating,
		body:       body,
		createdAt:  createdAt,
		deletedAt:  deletedAt,
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
