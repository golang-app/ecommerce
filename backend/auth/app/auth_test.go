package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/auth/app"
	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
)

type fakeCustomerStorage struct {
	findFn func(ctx context.Context, email string) (adapter.Customer, error)
}

func (f *fakeCustomerStorage) Create(ctx context.Context, email, passwordHash string) error {
	return nil
}

func (f *fakeCustomerStorage) Find(ctx context.Context, email string) (adapter.Customer, error) {
	if f.findFn != nil {
		return f.findFn(ctx, email)
	}
	return adapter.Customer{}, domain.ErrCustomerNotFound
}

func (f *fakeCustomerStorage) UpdatePassword(ctx context.Context, email, passwordHash string) error {
	return nil
}

func (f *fakeCustomerStorage) List(ctx context.Context) ([]string, error) {
	return nil, nil
}

func TestAuth_IsRegistered(t *testing.T) {
	ctx := context.Background()

	t.Run("returns true when customer found", func(t *testing.T) {
		storage := &fakeCustomerStorage{
			findFn: func(ctx context.Context, email string) (adapter.Customer, error) {
				if email == "alice@example.com" {
					return adapter.Customer{Username: "alice@example.com"}, nil
				}
				return adapter.Customer{}, domain.ErrCustomerNotFound
			},
		}
		srv := app.NewAuth(storage, nil)

		registered, err := srv.IsRegistered(ctx, "alice@example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !registered {
			t.Errorf("expected registered = true, got false")
		}
	})

	t.Run("returns false when customer not found", func(t *testing.T) {
		storage := &fakeCustomerStorage{
			findFn: func(ctx context.Context, email string) (adapter.Customer, error) {
				return adapter.Customer{}, domain.ErrCustomerNotFound
			},
		}
		srv := app.NewAuth(storage, nil)

		registered, err := srv.IsRegistered(ctx, "bob@example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if registered {
			t.Errorf("expected registered = false, got true")
		}
	})

	t.Run("returns error when storage fails", func(t *testing.T) {
		expectedErr := errors.New("db connection failure")
		storage := &fakeCustomerStorage{
			findFn: func(ctx context.Context, email string) (adapter.Customer, error) {
				return adapter.Customer{}, expectedErr
			},
		}
		srv := app.NewAuth(storage, nil)

		registered, err := srv.IsRegistered(ctx, "bob@example.com")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected wrapped db error, got %v", err)
		}
		if registered {
			t.Errorf("expected registered = false, got true")
		}
	})
}
