package domain_test

import (
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/store/domain"
)

func TestNewStore_Valid(t *testing.T) {
	s, err := domain.NewStore("us", "us", "GoCommerce US", "USD", "localhost:8080", true, 1)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if s.ID() != "us" || s.Slug() != "us" || s.Name() != "GoCommerce US" {
		t.Errorf("unexpected store fields: %#v", s)
	}
	if s.Currency() != "USD" {
		t.Errorf("currency = %q, want USD", s.Currency())
	}
	if !s.IsDefault() {
		t.Errorf("IsDefault = false, want true")
	}
}

func TestNewStore_NormalisesCurrencyCase(t *testing.T) {
	s, err := domain.NewStore("eu", "eu", "GoCommerce EU", "eur", "eu.localhost:8080", false, 2)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if s.Currency() != "EUR" {
		t.Errorf("currency = %q, want EUR (uppercased)", s.Currency())
	}
}

func TestNewStore_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name                                         string
		id, slug, sname, currency, host              string
		position                                     int
	}{
		{name: "empty id", id: "", slug: "us", sname: "n", currency: "USD", host: "h", position: 0},
		{name: "bad slug uppercase", id: "x", slug: "US", sname: "n", currency: "USD", host: "h", position: 0},
		{name: "bad slug underscore", id: "x", slug: "us_uk", sname: "n", currency: "USD", host: "h", position: 0},
		{name: "empty name", id: "x", slug: "us", sname: "", currency: "USD", host: "h", position: 0},
		{name: "currency too short", id: "x", slug: "us", sname: "n", currency: "US", host: "h", position: 0},
		{name: "currency too long", id: "x", slug: "us", sname: "n", currency: "USDX", host: "h", position: 0},
		{name: "currency non-letter", id: "x", slug: "us", sname: "n", currency: "US1", host: "h", position: 0},
		{name: "empty host", id: "x", slug: "us", sname: "n", currency: "USD", host: "", position: 0},
		{name: "negative position", id: "x", slug: "us", sname: "n", currency: "USD", host: "h", position: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewStore(tc.id, tc.slug, tc.sname, tc.currency, tc.host, false, tc.position)
			if !errors.Is(err, domain.ErrInvalidStore) {
				t.Errorf("err = %v, want ErrInvalidStore", err)
			}
		})
	}
}

func TestRebuildStore_BypassesValidation(t *testing.T) {
	// Hydration must succeed even with shapes NewStore would reject so
	// legacy rows keep loading.
	s := domain.RebuildStore("x", "US", "name", "usd", "h", true, 0)
	if s.IsZero() {
		t.Errorf("rebuilt store should not be zero")
	}
}
