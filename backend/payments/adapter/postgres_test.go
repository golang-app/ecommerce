//go:build integration

package adapter_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/bkielbasa/go-ecommerce/backend/payments/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/payments/app"
	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
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

func wipePaymentsTables(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`TRUNCATE TABLE payments_charge CASCADE`); err != nil {
		t.Fatalf("wipe payments_charge: %v", err)
	}
	if _, err := db.Exec(`TRUNCATE TABLE payments_provider_config CASCADE`); err != nil {
		t.Fatalf("wipe payments_provider_config: %v", err)
	}
	// Re-insert initial seeds as per migration 000042
	_, err := db.Exec(`
		INSERT INTO payments_provider_config (id, enabled, config) VALUES
		('stripe', true, '{"fail_card_ending_in":"0000","publishable_key":"","secret_key":"","webhook_secret":"whsec_dev_only_do_not_use_in_production"}'::jsonb),
		('fake', true, '{"name":"Fake Payment Simulator","description":"Interactive payment simulator for testing payment outcomes."}'::jsonb)
	`)
	if err != nil {
		t.Fatalf("seed payments_provider_config: %v", err)
	}
}

func TestPostgres_ProviderConfig(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	wipePaymentsTables(t, db)

	storage := adapter.NewPostgresStorage(db)
	ctx := context.Background()

	// 1. ListProviders returns initial default providers
	providers, err := storage.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders failed: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("ListProviders returned %d providers, want 2", len(providers))
	}
	// Check ordering: fake, stripe (alphabetical by id)
	if providers[0].ID() != "fake" || providers[1].ID() != "stripe" {
		t.Errorf("ListProviders ordering mismatch: [0]=%s, [1]=%s", providers[0].ID(), providers[1].ID())
	}
	if !providers[1].IsEnabled() {
		t.Errorf("stripe should be enabled")
	}
	if providers[1].ConfigValue("fail_card_ending_in") != "0000" {
		t.Errorf("stripe fail_card_ending_in = %q, want 0000", providers[1].ConfigValue("fail_card_ending_in"))
	}

	// 2. FindProvider
	stripe, err := storage.FindProvider(ctx, "stripe")
	if err != nil {
		t.Fatalf("FindProvider(stripe) failed: %v", err)
	}
	if stripe.ID() != "stripe" {
		t.Errorf("stripe.ID() = %q, want stripe", stripe.ID())
	}

	_, err = storage.FindProvider(ctx, "nonexistent")
	if !errors.Is(err, app.ErrProviderNotFound) {
		t.Errorf("FindProvider(nonexistent) err = %v, want ErrProviderNotFound", err)
	}

	// 3. SaveProvider (upsert existing)
	updatedAt := time.Now().UTC().Truncate(time.Microsecond)
	updatedStripe := stripe.WithEnabled(false).WithConfig(map[string]string{
		"fail_card_ending_in": "9999",
	}, updatedAt)
	if err := storage.SaveProvider(ctx, updatedStripe); err != nil {
		t.Fatalf("SaveProvider(updatedStripe) failed: %v", err)
	}

	reloaded, err := storage.FindProvider(ctx, "stripe")
	if err != nil {
		t.Fatalf("FindProvider after SaveProvider failed: %v", err)
	}
	if reloaded.IsEnabled() {
		t.Errorf("reloaded stripe should be disabled")
	}
	if reloaded.ConfigValue("fail_card_ending_in") != "9999" {
		t.Errorf("reloaded fail_card_ending_in = %q, want 9999", reloaded.ConfigValue("fail_card_ending_in"))
	}

	// 4. SaveProvider (insert new)
	customCfg := domain.NewProviderConfig("custom", true, map[string]string{"key": "val"}, updatedAt)
	if err := storage.SaveProvider(ctx, customCfg); err != nil {
		t.Fatalf("SaveProvider(customCfg) failed: %v", err)
	}
	loadedCustom, err := storage.FindProvider(ctx, "custom")
	if err != nil {
		t.Fatalf("FindProvider(custom) failed: %v", err)
	}
	if loadedCustom.ConfigValue("key") != "val" {
		t.Errorf("loadedCustom config[key] = %q, want val", loadedCustom.ConfigValue("key"))
	}
}

func TestPostgres_Charge_Provider_And_OrderID(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	wipePaymentsTables(t, db)

	storage := adapter.NewPostgresStorage(db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	charge := domain.NewCharge("ch-pg-1", "idem-pg-1", 4500, "USD", domain.ProviderFake, "ord-pg-100", now)

	if err := storage.Insert(ctx, charge); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Find by ID
	found, err := storage.Find(ctx, "ch-pg-1")
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if found.Provider() != domain.ProviderFake {
		t.Errorf("found Provider() = %q, want %q", found.Provider(), domain.ProviderFake)
	}
	if found.OrderID() != "ord-pg-100" {
		t.Errorf("found OrderID() = %q, want ord-pg-100", found.OrderID())
	}

	// Find by Idempotency Key
	foundKey, ok, err := storage.FindByIdempotencyKey(ctx, "idem-pg-1")
	if err != nil || !ok {
		t.Fatalf("FindByIdempotencyKey failed: ok=%v, err=%v", ok, err)
	}
	if foundKey.Provider() != domain.ProviderFake || foundKey.OrderID() != "ord-pg-100" {
		t.Errorf("foundKey mismatch: provider=%s, orderID=%s", foundKey.Provider(), foundKey.OrderID())
	}

	// Update status and provider ref
	if err := storage.UpdateStatus(ctx, "ch-pg-1", domain.StatusSucceeded, "ref-pg-1", now.Add(time.Second)); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Find by ProviderRef
	foundRef, err := storage.FindByProviderRef(ctx, "ref-pg-1")
	if err != nil {
		t.Fatalf("FindByProviderRef failed: %v", err)
	}
	if foundRef.Provider() != domain.ProviderFake || foundRef.OrderID() != "ord-pg-100" {
		t.Errorf("foundRef mismatch: provider=%s, orderID=%s", foundRef.Provider(), foundRef.OrderID())
	}

	// FindByOrderID
	foundOrder, err := storage.FindByOrderID(ctx, "ord-pg-100")
	if err != nil {
		t.Fatalf("FindByOrderID failed: %v", err)
	}
	if foundOrder.ID() != "ch-pg-1" {
		t.Errorf("foundOrder ID = %q, want ch-pg-1", foundOrder.ID())
	}

	// Missing OrderID returns app.ErrChargeNotFound
	_, err = storage.FindByOrderID(ctx, "ord-missing")
	if !errors.Is(err, app.ErrChargeNotFound) {
		t.Errorf("FindByOrderID(ord-missing) err = %v, want ErrChargeNotFound", err)
	}

	// Test ordering: insert newer charge for same order
	later := now.Add(time.Hour)
	charge2 := domain.NewCharge("ch-pg-2", "idem-pg-2", 4500, "USD", domain.ProviderFake, "ord-pg-100", later)
	if err := storage.Insert(ctx, charge2); err != nil {
		t.Fatalf("Insert charge2 failed: %v", err)
	}

	latestOrderCharge, err := storage.FindByOrderID(ctx, "ord-pg-100")
	if err != nil {
		t.Fatalf("FindByOrderID after second insert failed: %v", err)
	}
	if latestOrderCharge.ID() != "ch-pg-2" {
		t.Errorf("latestOrderCharge ID = %q, want ch-pg-2 (most recent)", latestOrderCharge.ID())
	}
}
