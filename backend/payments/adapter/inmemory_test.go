package adapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/payments/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/payments/app"
	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

func TestInMemory_ProviderConfig(t *testing.T) {
	storage := adapter.NewInMemoryStorage()
	ctx := context.Background()

	// 1. ListProviders returns initial default providers (stripe and fake enabled)
	providers, err := storage.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders failed: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("ListProviders returned %d providers, want 2", len(providers))
	}

	foundMap := make(map[string]domain.ProviderConfig)
	for _, p := range providers {
		foundMap[p.ID()] = p
	}

	stripe, ok := foundMap[domain.ProviderStripe]
	if !ok {
		t.Fatalf("stripe provider not found in ListProviders")
	}
	if !stripe.IsEnabled() {
		t.Errorf("stripe provider should be enabled by default")
	}
	if stripe.ConfigValue("fail_card_ending_in") != "0000" {
		t.Errorf("stripe fail_card_ending_in = %q, want 0000", stripe.ConfigValue("fail_card_ending_in"))
	}

	fake, ok := foundMap[domain.ProviderFake]
	if !ok {
		t.Fatalf("fake provider not found in ListProviders")
	}
	if !fake.IsEnabled() {
		t.Errorf("fake provider should be enabled by default")
	}
	if fake.ConfigValue("name") == "" {
		t.Errorf("fake provider name is empty")
	}
	if fake.ConfigValue("description") == "" {
		t.Errorf("fake provider description is empty")
	}

	// 2. FindProvider("stripe") returns stripe config
	foundStripe, err := storage.FindProvider(ctx, domain.ProviderStripe)
	if err != nil {
		t.Fatalf("FindProvider(stripe) failed: %v", err)
	}
	if foundStripe.ID() != domain.ProviderStripe {
		t.Errorf("FindProvider(stripe) ID = %q, want stripe", foundStripe.ID())
	}
	if !foundStripe.IsEnabled() {
		t.Errorf("FindProvider(stripe) IsEnabled = false, want true")
	}

	// 3. FindProvider("unknown") returns app.ErrProviderNotFound
	_, err = storage.FindProvider(ctx, "unknown")
	if !errors.Is(err, app.ErrProviderNotFound) {
		t.Errorf("FindProvider(unknown) err = %v, want ErrProviderNotFound", err)
	}

	// 4. SaveProvider updates provider and subsequent FindProvider returns updated state
	updatedAt := time.Now().UTC()
	updatedStripe := stripe.WithEnabled(false).WithConfig(map[string]string{
		"fail_card_ending_in": "9999",
	}, updatedAt)

	if err := storage.SaveProvider(ctx, updatedStripe); err != nil {
		t.Fatalf("SaveProvider failed: %v", err)
	}

	afterSave, err := storage.FindProvider(ctx, domain.ProviderStripe)
	if err != nil {
		t.Fatalf("FindProvider after SaveProvider failed: %v", err)
	}
	if afterSave.IsEnabled() {
		t.Errorf("afterSave IsEnabled = true, want false")
	}
	if afterSave.ConfigValue("fail_card_ending_in") != "9999" {
		t.Errorf("afterSave fail_card_ending_in = %q, want 9999", afterSave.ConfigValue("fail_card_ending_in"))
	}
}

func TestInMemory_FindByOrderID(t *testing.T) {
	storage := adapter.NewInMemoryStorage()
	ctx := context.Background()

	now := time.Now().UTC()
	charge := domain.NewCharge("ch-1", "idem-1", 1000, "USD", domain.ProviderStripe, "ord-123", now)

	if err := storage.Insert(ctx, charge); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Find by existing order ID
	found, err := storage.FindByOrderID(ctx, "ord-123")
	if err != nil {
		t.Fatalf("FindByOrderID(ord-123) failed: %v", err)
	}
	if found.ID() != "ch-1" {
		t.Errorf("found ID = %q, want ch-1", found.ID())
	}
	if found.OrderID() != "ord-123" {
		t.Errorf("found OrderID = %q, want ord-123", found.OrderID())
	}

	// Missing order ID returns app.ErrChargeNotFound
	_, err = storage.FindByOrderID(ctx, "missing")
	if !errors.Is(err, app.ErrChargeNotFound) {
		t.Errorf("FindByOrderID(missing) err = %v, want ErrChargeNotFound", err)
	}

	// Blank order ID returns app.ErrChargeNotFound
	_, err = storage.FindByOrderID(ctx, "")
	if !errors.Is(err, app.ErrChargeNotFound) {
		t.Errorf("FindByOrderID(\"\") err = %v, want ErrChargeNotFound", err)
	}
}
