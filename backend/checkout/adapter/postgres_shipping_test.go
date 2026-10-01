//go:build integration

package adapter_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/app"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	pass := getEnv("POSTGRES_PASSWORD", "postgres")
	conn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("POSTGRES_HOST", "localhost"),
		getEnv("POSTGRES_PORT", "5432"),
		getEnv("POSTGRES_USER", "postgres"),
		pass,
		getEnv("POSTGRES_DB", "ecommerce"),
	)
	db, err := sql.Open("postgres", conn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	return db
}

func getEnv(name, def string) string {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	return v
}

func wipeShippingTable(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`TRUNCATE TABLE shipping_method_config CASCADE`); err != nil {
		t.Fatalf("wipe shipping_method_config: %v", err)
	}
	_, err := db.Exec(`
		INSERT INTO shipping_method_config (code, enabled, label, cost, requires_address, carrier, updated_at)
		VALUES
			('flat', true, 'Flat rate', 500, true, 'Standard Post', NOW()),
			('pickup', true, 'Personal pickup', 0, false, 'Store Pickup', NOW()),
			('courier', true, 'Courier', 1500, true, 'Express Courier', NOW())
		ON CONFLICT (code) DO NOTHING
	`)
	if err != nil {
		t.Fatalf("seed shipping_method_config: %v", err)
	}
}

func TestPostgresShippingStorage_ListShippingMethods(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	wipeShippingTable(t, db)

	storage := adapter.NewPostgresShippingStorage(db)
	ctx := context.Background()

	methods, err := storage.ListShippingMethods(ctx)
	if err != nil {
		t.Fatalf("ListShippingMethods failed: %v", err)
	}

	if len(methods) != 3 {
		t.Fatalf("expected 3 seed methods, got %d", len(methods))
	}

	// Ordered by code ASC: courier, flat, pickup
	if methods[0].Code() != "courier" || methods[1].Code() != "flat" || methods[2].Code() != "pickup" {
		t.Errorf("unexpected ordering: %v, %v, %v", methods[0].Code(), methods[1].Code(), methods[2].Code())
	}

	flat := methods[1]
	if flat.Label() != "Flat rate" || flat.Cost() != 500 || !flat.RequiresAddress() || flat.Carrier() != "Standard Post" || !flat.IsEnabled() {
		t.Errorf("flat method mismatch: %+v", flat)
	}
}

func TestPostgresShippingStorage_FindShippingMethod(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	wipeShippingTable(t, db)

	storage := adapter.NewPostgresShippingStorage(db)
	ctx := context.Background()

	// Existing
	m, err := storage.FindShippingMethod(ctx, "flat")
	if err != nil {
		t.Fatalf("FindShippingMethod(flat) failed: %v", err)
	}
	if m.Code() != "flat" || m.Cost() != 500 || m.Carrier() != "Standard Post" || !m.IsEnabled() || !m.RequiresAddress() {
		t.Errorf("unexpected method: %+v", m)
	}

	// Non-existent
	_, err = storage.FindShippingMethod(ctx, "nonexistent")
	if !errors.Is(err, app.ErrShippingMethodNotFound) {
		t.Errorf("expected ErrShippingMethodNotFound, got %v", err)
	}
}

func TestPostgresShippingStorage_SaveShippingMethod(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	wipeShippingTable(t, db)

	storage := adapter.NewPostgresShippingStorage(db)
	ctx := context.Background()

	// Update existing method
	updatedFlat := domain.NewShippingMethod("flat", "Flat rate express", 750, true, "New Post", false)
	err := storage.SaveShippingMethod(ctx, updatedFlat)
	if err != nil {
		t.Fatalf("SaveShippingMethod(updatedFlat) failed: %v", err)
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
	if foundDrone.Code() != "drone" || foundDrone.Cost() != 3000 || !foundDrone.RequiresAddress() || foundDrone.Carrier() != "SkyDrop" || !foundDrone.IsEnabled() {
		t.Errorf("drone mismatch: %+v", foundDrone)
	}
}
