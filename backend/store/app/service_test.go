package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/store/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/store/app"
	"github.com/bkielbasa/go-ecommerce/backend/store/domain"
)

func mustStore(t *testing.T, id, slug, name, currency, host string, isDefault bool, position int) domain.Store {
	t.Helper()
	s, err := domain.NewStore(id, slug, name, currency, host, isDefault, position)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func seededService(t *testing.T) *app.Service {
	t.Helper()
	storage := adapter.NewInMemory()
	srv := app.NewService(storage)
	ctx := context.Background()
	if err := storage.Create(ctx, mustStore(t, "us", "us", "GoCommerce US", "USD", "localhost:8080", true, 1)); err != nil {
		t.Fatalf("seed us: %v", err)
	}
	if err := storage.Create(ctx, mustStore(t, "eu", "eu", "GoCommerce EU", "EUR", "eu.localhost:8080", false, 2)); err != nil {
		t.Fatalf("seed eu: %v", err)
	}
	return srv
}

func TestResolveByHost_MatchesByHost(t *testing.T) {
	srv := seededService(t)
	s, err := srv.ResolveByHost(context.Background(), "eu.localhost:8080")
	if err != nil {
		t.Fatalf("ResolveByHost: %v", err)
	}
	if s.ID() != "eu" {
		t.Errorf("resolved id = %q, want eu", s.ID())
	}
	if s.Currency() != "EUR" {
		t.Errorf("resolved currency = %q, want EUR", s.Currency())
	}
}

func TestResolveByHost_IsCaseInsensitive(t *testing.T) {
	srv := seededService(t)
	s, err := srv.ResolveByHost(context.Background(), "EU.LocalHost:8080")
	if err != nil {
		t.Fatalf("ResolveByHost: %v", err)
	}
	if s.ID() != "eu" {
		t.Errorf("resolved id = %q, want eu", s.ID())
	}
}

func TestResolveByHost_UnknownHostFallsBackToDefault(t *testing.T) {
	srv := seededService(t)
	s, err := srv.ResolveByHost(context.Background(), "no-such-host:8080")
	if err != nil {
		t.Fatalf("ResolveByHost: %v", err)
	}
	if s.ID() != "us" {
		t.Errorf("fallback id = %q, want us (the seeded default)", s.ID())
	}
}

func TestResolveByHost_ErrorsWhenNoDefault(t *testing.T) {
	storage := adapter.NewInMemory()
	srv := app.NewService(storage)
	// Seed a single store that is NOT the default — exposes the
	// "no default" edge case the production install never hits (the
	// migration + seeds guarantee one default exists).
	if err := storage.Create(context.Background(), mustStore(t, "uk", "uk", "GoCommerce UK", "GBP", "uk.localhost:8080", false, 3)); err != nil {
		t.Fatalf("seed uk: %v", err)
	}
	_, err := srv.ResolveByHost(context.Background(), "no-such-host:8080")
	if !errors.Is(err, app.ErrNoDefaultStore) {
		t.Errorf("err = %v, want ErrNoDefaultStore", err)
	}
}

func TestService_CreateUpdateDefaultUnique(t *testing.T) {
	storage := adapter.NewInMemory()
	srv := app.NewService(storage)
	ctx := context.Background()
	if err := srv.Create(ctx, mustStore(t, "us", "us", "GoCommerce US", "USD", "localhost:8080", true, 1)); err != nil {
		t.Fatalf("create us: %v", err)
	}
	if err := srv.Create(ctx, mustStore(t, "eu", "eu", "GoCommerce EU", "EUR", "eu.localhost:8080", true, 2)); err != nil {
		t.Fatalf("create eu: %v", err)
	}
	// Both inserted as default-true, but the second create must have
	// cleared the first one's default flag.
	def, err := srv.Default(ctx)
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if def.ID() != "eu" {
		t.Errorf("default id = %q, want eu (latest write wins)", def.ID())
	}
	all, _ := srv.ListAll(ctx)
	defaults := 0
	for _, s := range all {
		if s.IsDefault() {
			defaults++
		}
	}
	if defaults != 1 {
		t.Errorf("default count = %d, want exactly 1", defaults)
	}
}
