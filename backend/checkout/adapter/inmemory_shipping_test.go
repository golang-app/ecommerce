package adapter

import (
	"context"
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/app"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

func TestInMemoryShippingStorage_Seed(t *testing.T) {
	storage := NewInMemoryShippingStorage()
	ctx := context.Background()

	methods, err := storage.ListShippingMethods(ctx)
	if err != nil {
		t.Fatalf("ListShippingMethods failed: %v", err)
	}

	if len(methods) != 3 {
		t.Fatalf("expected 3 seed methods, got %d", len(methods))
	}

	expectedCodes := map[string]bool{
		"flat":    false,
		"pickup":  false,
		"courier": false,
	}

	for _, m := range methods {
		expectedCodes[m.Code()] = true
		switch m.Code() {
		case "flat":
			if m.Label() != "Flat rate" || m.Cost() != 500 || !m.RequiresAddress() || m.Carrier() != "Standard Post" || !m.IsEnabled() {
				t.Errorf("flat rate attributes mismatch: %+v", m)
			}
		case "pickup":
			if m.Label() != "Personal pickup" || m.Cost() != 0 || m.RequiresAddress() || m.Carrier() != "Store Pickup" || !m.IsEnabled() {
				t.Errorf("pickup attributes mismatch: %+v", m)
			}
		case "courier":
			if m.Label() != "Courier" || m.Cost() != 1500 || !m.RequiresAddress() || m.Carrier() != "Express Courier" || !m.IsEnabled() {
				t.Errorf("courier attributes mismatch: %+v", m)
			}
		}
	}

	for code, found := range expectedCodes {
		if !found {
			t.Errorf("expected seed method %s not found in list", code)
		}
	}
}

func TestInMemoryShippingStorage_FindShippingMethod(t *testing.T) {
	storage := NewInMemoryShippingStorage()
	ctx := context.Background()

	// Find existing
	m, err := storage.FindShippingMethod(ctx, "flat")
	if err != nil {
		t.Fatalf("FindShippingMethod(flat) failed: %v", err)
	}
	if m.Code() != "flat" || m.Cost() != 500 {
		t.Errorf("unexpected method: %+v", m)
	}

	// Find non-existent
	_, err = storage.FindShippingMethod(ctx, "nonexistent")
	if !errors.Is(err, app.ErrShippingMethodNotFound) {
		t.Errorf("expected ErrShippingMethodNotFound, got %v", err)
	}
}

func TestInMemoryShippingStorage_SaveShippingMethod(t *testing.T) {
	storage := NewInMemoryShippingStorage()
	ctx := context.Background()

	// Update existing method
	updatedFlat := domain.NewShippingMethod("flat", "Flat rate express", 750, true, "New Post", false)
	err := storage.SaveShippingMethod(ctx, updatedFlat)
	if err != nil {
		t.Fatalf("SaveShippingMethod failed: %v", err)
	}

	m, err := storage.FindShippingMethod(ctx, "flat")
	if err != nil {
		t.Fatalf("FindShippingMethod(flat) failed: %v", err)
	}
	if m.Label() != "Flat rate express" || m.Cost() != 750 || m.Carrier() != "New Post" || m.IsEnabled() {
		t.Errorf("updated flat rate mismatch: %+v", m)
	}

	// Insert new method
	drone := domain.NewShippingMethod("drone", "Drone Delivery", 3000, true, "SkyDrop", true)
	err = storage.SaveShippingMethod(ctx, drone)
	if err != nil {
		t.Fatalf("SaveShippingMethod(drone) failed: %v", err)
	}

	methods, err := storage.ListShippingMethods(ctx)
	if err != nil {
		t.Fatalf("ListShippingMethods failed: %v", err)
	}
	if len(methods) != 4 {
		t.Fatalf("expected 4 methods, got %d", len(methods))
	}

	foundDrone, err := storage.FindShippingMethod(ctx, "drone")
	if err != nil {
		t.Fatalf("FindShippingMethod(drone) failed: %v", err)
	}
	if foundDrone.Code() != "drone" || foundDrone.Cost() != 3000 || foundDrone.Carrier() != "SkyDrop" {
		t.Errorf("drone mismatch: %+v", foundDrone)
	}
}
