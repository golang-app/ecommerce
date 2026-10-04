package adapter_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
)

func TestInMemoryAuthStorage_List(t *testing.T) {
	ctx := context.Background()
	store := adapter.NewInMemoryAuthStorage()

	if err := store.Create(ctx, "charlie@example.com", "hash"); err != nil {
		t.Fatalf("Create charlie: %v", err)
	}
	if err := store.Create(ctx, "alice@example.com", "hash"); err != nil {
		t.Fatalf("Create alice: %v", err)
	}
	if err := store.Create(ctx, "bob@example.com", "hash"); err != nil {
		t.Fatalf("Create bob: %v", err)
	}

	customers, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	expected := []string{"alice@example.com", "bob@example.com", "charlie@example.com"}
	if !reflect.DeepEqual(customers, expected) {
		t.Fatalf("expected %v, got %v", expected, customers)
	}
}
