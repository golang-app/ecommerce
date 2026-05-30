// Package app holds the payments bounded context's application
// service. The service is the orchestrator: it owns the
// Charge-lifecycle workflow (look up by idempotency key, insert a
// pending row, call the provider through the ACL, settle the row to a
// terminal status) and the webhook-driven status transitions.
//
// The Provider seam is the contract the ACL satisfies — the service
// never imports the external SDK directly. That is the whole point of
// the anti-corruption layer: payments stays free of provider
// vocabulary; the ACL translates.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

// ErrChargeNotFound is returned by Storage when no Charge exists for
// the lookup. The webhook handler relies on it to distinguish "we
// have not seen this provider_ref" from a real storage failure.
var ErrChargeNotFound = errors.New("payments: charge not found")

// Storage is the persistence seam for Charges. The postgres and
// in-memory adapters both implement it.
//
// Insert creates a pending Charge; a second Insert with the same
// IdempotencyKey is rejected with ErrIdempotencyKeyConflict — callers
// must FindByIdempotencyKey first to skip the call.
//
// FindByProviderRef is the webhook lookup path: an asynchronous
// payment_intent.succeeded / .failed callback identifies the Charge
// via the provider's own reference.
type Storage interface {
	Insert(ctx context.Context, c domain.Charge) error
	Find(ctx context.Context, id string) (domain.Charge, error)
	UpdateStatus(ctx context.Context, id string, status domain.Status, providerRef string, updatedAt time.Time) error
	FindByIdempotencyKey(ctx context.Context, key string) (domain.Charge, bool, error)
	FindByProviderRef(ctx context.Context, providerRef string) (domain.Charge, error)
}

// ErrIdempotencyKeyConflict signals a UNIQUE-violation on the
// idempotency_key column. Callers wanting "give me the existing
// Charge for this key" should use FindByIdempotencyKey first; this
// error only surfaces if a race squeezes between the lookup and the
// Insert. The service handles that race by re-reading and continuing.
var ErrIdempotencyKeyConflict = errors.New("payments: idempotency key already used")

// ChargeRequest is the payments-domain input to a charge attempt.
// Source is a card-token / payment-method identifier in the provider's
// vocabulary; we accept it as opaque text so the provider type doesn't
// leak past the ACL.
//
// IdempotencyKey is included so the SAME key the service tracked in
// payments_charge.idempotency_key also reaches the provider's own
// idempotency table. End-to-end the contract is "the same retry token
// from the caller is the same dedupe key in every layer" — which is
// what makes the whole story safe under concurrent retries.
type ChargeRequest struct {
	Amount         int64
	Currency       string
	Source         string
	IdempotencyKey string
}

// ChargeResult is the payments-domain output of a charge attempt.
// Status is one of the domain.Status values; the ACL has already
// translated the provider's vocabulary into ours.
type ChargeResult struct {
	Status      domain.Status
	ProviderRef string
}

// Provider is the ACL boundary the application service depends on.
// The concrete implementation in payments/adapter/fakestripe_acl.go
// is the ONLY thing that imports the external SDK.
type Provider interface {
	Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
}

// IDGenerator returns a fresh Charge ID. Injected for tests; the
// composition root wires a hex-random generator.
type IDGenerator func() string

// Service is the payments orchestrator.
type Service struct {
	storage  Storage
	provider Provider
	newID    IDGenerator
	now      func() time.Time
}

// NewService constructs the payments application service. provider is
// the ACL onto the external system; pass the same instance that wraps
// fakestripe.Client in production wiring.
func NewService(storage Storage, provider Provider, newID IDGenerator, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{storage: storage, provider: provider, newID: newID, now: now}
}

// Charge runs the full charge workflow:
//
//  1. If idempotencyKey is non-empty and a Charge already exists for
//     it, RETURN that Charge — never call the provider a second time.
//     This is the real-world idempotency contract.
//
//  2. Otherwise insert a pending Charge, call the provider through
//     the ACL, and persist the resulting status + providerRef.
//
//  3. Return the final (terminal or still-pending) Charge so the
//     caller — for the demo, checkout — can decide whether to commit
//     the order.
//
// orderID is threaded in for log/trace correlation; it is NOT
// persisted on the Charge (the charge has its own id) and is not
// required to be globally unique on its own.
func (s *Service) Charge(ctx context.Context, orderID string, amount int64, currency, source, idempotencyKey string) (domain.Charge, error) {
	if idempotencyKey != "" {
		existing, ok, err := s.storage.FindByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			return domain.Charge{}, fmt.Errorf("payments: lookup idempotency key: %w", err)
		}
		if ok {
			// Idempotent re-submission — return the previous
			// outcome without touching the provider.
			return existing, nil
		}
	}

	chargeID := s.newID()
	pending := domain.NewCharge(chargeID, idempotencyKey, amount, currency, s.now())
	if err := s.storage.Insert(ctx, pending); err != nil {
		if errors.Is(err, ErrIdempotencyKeyConflict) && idempotencyKey != "" {
			// A concurrent first-time insert won the race. Read
			// what it wrote and return that, matching the
			// idempotent-re-submission outcome from above.
			existing, ok, ferr := s.storage.FindByIdempotencyKey(ctx, idempotencyKey)
			if ferr != nil {
				return domain.Charge{}, fmt.Errorf("payments: lookup after conflict: %w", ferr)
			}
			if ok {
				return existing, nil
			}
		}
		return domain.Charge{}, fmt.Errorf("payments: insert pending: %w", err)
	}

	result, err := s.provider.Charge(ctx, ChargeRequest{
		Amount:         amount,
		Currency:       currency,
		Source:         source,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		// A provider error is non-terminal as far as the domain is
		// concerned — we record "failed" because we know we have not
		// captured funds. (A retry with the same idempotency key
		// would short-circuit on the existing failed row, which is
		// the safer default: re-trying a charge needs an explicit
		// new attempt in the demo.)
		updated := pending.WithStatus(domain.StatusFailed, "", s.now())
		if uerr := s.storage.UpdateStatus(ctx, chargeID, updated.Status(), updated.ProviderRef(), updated.UpdatedAt()); uerr != nil {
			return domain.Charge{}, fmt.Errorf("payments: settle failed after provider error: %v (original: %w)", uerr, err)
		}
		return updated, fmt.Errorf("payments: provider charge: %w", err)
	}

	settled := pending.WithStatus(result.Status, result.ProviderRef, s.now())
	if err := s.storage.UpdateStatus(ctx, chargeID, settled.Status(), settled.ProviderRef(), settled.UpdatedAt()); err != nil {
		return domain.Charge{}, fmt.Errorf("payments: settle: %w", err)
	}
	return settled, nil
}

// MarkSucceeded transitions the Charge identified by providerRef into
// the succeeded terminal status. Idempotent: a Charge already in that
// status is a no-op. Used by the webhook handler to apply asynchronous
// provider confirmations (delayed captures, etc.).
func (s *Service) MarkSucceeded(ctx context.Context, providerRef string) error {
	return s.transitionByProviderRef(ctx, providerRef, domain.StatusSucceeded)
}

// MarkFailed transitions the Charge identified by providerRef into
// the failed terminal status. Idempotent in the same way as
// MarkSucceeded. reason is accepted for log correlation; the demo
// does not persist it (a real system would store it for the dispute
// trail).
func (s *Service) MarkFailed(ctx context.Context, providerRef, reason string) error {
	_ = reason // reserved: production-grade systems would store this
	return s.transitionByProviderRef(ctx, providerRef, domain.StatusFailed)
}

func (s *Service) transitionByProviderRef(ctx context.Context, providerRef string, target domain.Status) error {
	if providerRef == "" {
		return fmt.Errorf("payments: empty provider_ref")
	}
	existing, err := s.storage.FindByProviderRef(ctx, providerRef)
	if err != nil {
		return fmt.Errorf("payments: find by provider_ref: %w", err)
	}
	if existing.Status() == target {
		// Already in the target state — webhook redelivery
		// no-op. This is the idempotency-at-the-handler boundary.
		return nil
	}
	updated := existing.WithStatus(target, providerRef, s.now())
	if err := s.storage.UpdateStatus(ctx, existing.ID(), updated.Status(), updated.ProviderRef(), updated.UpdatedAt()); err != nil {
		return fmt.Errorf("payments: update status: %w", err)
	}
	return nil
}

// Find returns the Charge by its domain id. Exposed for log /
// observability paths that want to render a row.
func (s *Service) Find(ctx context.Context, id string) (domain.Charge, error) {
	return s.storage.Find(ctx, id)
}
