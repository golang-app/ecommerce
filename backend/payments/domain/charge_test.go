package domain_test

import (
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

func TestNewCharge(t *testing.T) {
	now := time.Now().UTC()
	c := domain.NewCharge("ch-123", "idem-456", 2500, "USD", domain.ProviderFake, "ord-789", now)

	if c.ID() != "ch-123" {
		t.Errorf("ID() = %q, want ch-123", c.ID())
	}
	if c.IdempotencyKey() != "idem-456" {
		t.Errorf("IdempotencyKey() = %q, want idem-456", c.IdempotencyKey())
	}
	if c.Amount() != 2500 {
		t.Errorf("Amount() = %d, want 2500", c.Amount())
	}
	if c.Currency() != "USD" {
		t.Errorf("Currency() = %q, want USD", c.Currency())
	}
	if c.Status() != domain.StatusPending {
		t.Errorf("Status() = %q, want %q", c.Status(), domain.StatusPending)
	}
	if c.ProviderRef() != "" {
		t.Errorf("ProviderRef() = %q, want empty", c.ProviderRef())
	}
	if c.Provider() != domain.ProviderFake {
		t.Errorf("Provider() = %q, want %q", c.Provider(), domain.ProviderFake)
	}
	if c.OrderID() != "ord-789" {
		t.Errorf("OrderID() = %q, want ord-789", c.OrderID())
	}
	if !c.CreatedAt().Equal(now) {
		t.Errorf("CreatedAt() = %v, want %v", c.CreatedAt(), now)
	}
	if !c.UpdatedAt().Equal(now) {
		t.Errorf("UpdatedAt() = %v, want %v", c.UpdatedAt(), now)
	}
}

func TestNewCharge_DefaultProvider(t *testing.T) {
	now := time.Now().UTC()
	c := domain.NewCharge("ch-123", "idem-456", 2500, "USD", "", "", now)

	if c.Provider() != domain.ProviderStripe {
		t.Errorf("Provider() = %q, want default %q", c.Provider(), domain.ProviderStripe)
	}
	if c.OrderID() != "" {
		t.Errorf("OrderID() = %q, want empty", c.OrderID())
	}
}

func TestRebuildCharge(t *testing.T) {
	created := time.Now().UTC().Add(-time.Hour)
	updated := time.Now().UTC()

	c := domain.RebuildCharge("ch-1", "idem-1", 1000, "EUR", domain.StatusSucceeded, "ref-1", domain.ProviderFake, "ord-1", created, updated)

	if c.ID() != "ch-1" {
		t.Errorf("ID() = %q, want ch-1", c.ID())
	}
	if c.IdempotencyKey() != "idem-1" {
		t.Errorf("IdempotencyKey() = %q, want idem-1", c.IdempotencyKey())
	}
	if c.Amount() != 1000 {
		t.Errorf("Amount() = %d, want 1000", c.Amount())
	}
	if c.Currency() != "EUR" {
		t.Errorf("Currency() = %q, want EUR", c.Currency())
	}
	if c.Status() != domain.StatusSucceeded {
		t.Errorf("Status() = %q, want %q", c.Status(), domain.StatusSucceeded)
	}
	if c.ProviderRef() != "ref-1" {
		t.Errorf("ProviderRef() = %q, want ref-1", c.ProviderRef())
	}
	if c.Provider() != domain.ProviderFake {
		t.Errorf("Provider() = %q, want %q", c.Provider(), domain.ProviderFake)
	}
	if c.OrderID() != "ord-1" {
		t.Errorf("OrderID() = %q, want ord-1", c.OrderID())
	}
	if !c.CreatedAt().Equal(created) {
		t.Errorf("CreatedAt() = %v, want %v", c.CreatedAt(), created)
	}
	if !c.UpdatedAt().Equal(updated) {
		t.Errorf("UpdatedAt() = %v, want %v", c.UpdatedAt(), updated)
	}
}

func TestRebuildCharge_DefaultProvider(t *testing.T) {
	created := time.Now().UTC().Add(-time.Hour)
	updated := time.Now().UTC()

	c := domain.RebuildCharge("ch-1", "idem-1", 1000, "EUR", domain.StatusPending, "", "", "", created, updated)

	if c.Provider() != domain.ProviderStripe {
		t.Errorf("Provider() = %q, want default %q", c.Provider(), domain.ProviderStripe)
	}
}

func TestCharge_WithProvider_And_WithOrderID(t *testing.T) {
	now := time.Now().UTC()
	c := domain.NewCharge("ch-1", "idem-1", 1000, "USD", domain.ProviderStripe, "ord-1", now)

	c2 := c.WithProvider(domain.ProviderFake)
	if c2.Provider() != domain.ProviderFake {
		t.Errorf("WithProvider: Provider() = %q, want %q", c2.Provider(), domain.ProviderFake)
	}
	// Original unchanged
	if c.Provider() != domain.ProviderStripe {
		t.Errorf("Original c.Provider() changed: got %q, want %q", c.Provider(), domain.ProviderStripe)
	}

	// Empty provider defaults to stripe
	c3 := c2.WithProvider("")
	if c3.Provider() != domain.ProviderStripe {
		t.Errorf("WithProvider(\"\"): Provider() = %q, want %q", c3.Provider(), domain.ProviderStripe)
	}

	c4 := c.WithOrderID("ord-new")
	if c4.OrderID() != "ord-new" {
		t.Errorf("WithOrderID: OrderID() = %q, want ord-new", c4.OrderID())
	}
	if c.OrderID() != "ord-1" {
		t.Errorf("Original c.OrderID() changed: got %q, want ord-1", c.OrderID())
	}
}

func TestCharge_WithStatusPreservesProviderAndOrderID(t *testing.T) {
	now := time.Now().UTC()
	c := domain.NewCharge("ch-1", "idem-1", 1000, "USD", domain.ProviderFake, "ord-1", now)

	later := now.Add(time.Minute)
	settled := c.WithStatus(domain.StatusSucceeded, "ref-abc", later)

	if settled.Status() != domain.StatusSucceeded {
		t.Errorf("Status() = %q, want %q", settled.Status(), domain.StatusSucceeded)
	}
	if settled.ProviderRef() != "ref-abc" {
		t.Errorf("ProviderRef() = %q, want ref-abc", settled.ProviderRef())
	}
	if settled.Provider() != domain.ProviderFake {
		t.Errorf("Provider() = %q, want %q", settled.Provider(), domain.ProviderFake)
	}
	if settled.OrderID() != "ord-1" {
		t.Errorf("OrderID() = %q, want ord-1", settled.OrderID())
	}
}
