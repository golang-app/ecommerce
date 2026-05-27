package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/shippinginfo/domain"
)

func TestNewAddress_RequiresMandatoryFields(t *testing.T) {
	now := time.Now()
	// args after id/customerID: name, street1, street2, city, zip, country
	base := []string{"Jane", "1 Main St", "apt 2", "PDX", "97201", "USA"}
	required := map[int]string{0: "name", 1: "street1", 3: "city", 4: "zip", 5: "country"}

	for idx := range required {
		args := append([]string(nil), base...)
		args[idx] = "  "
		_, err := domain.NewAddress("a1", "jane@example.com", args[0], args[1], args[2], args[3], args[4], args[5], false, now)
		if !errors.Is(err, domain.ErrInvalidAddress) {
			t.Errorf("blank field %d: err = %v, want ErrInvalidAddress", idx, err)
		}
	}
}

func TestNewAddress_Street2Optional_AndTrims(t *testing.T) {
	now := time.Now()
	a, err := domain.NewAddress("a1", "jane@example.com", " Jane ", " 1 Main St ", "", " PDX ", " 97201 ", " USA ", true, now)
	if err != nil {
		t.Fatalf("NewAddress: %v", err)
	}
	if a.Name() != "Jane" || a.Street1() != "1 Main St" || a.City() != "PDX" || a.Country() != "USA" {
		t.Errorf("fields not trimmed: %+v", a)
	}
	if !a.IsDefault() {
		t.Error("IsDefault should be true")
	}
	if a.IsZero() {
		t.Error("a populated address should not be IsZero")
	}
}

func TestAddress_ZeroValueIsZero(t *testing.T) {
	if !(domain.Address{}).IsZero() {
		t.Error("zero Address should be IsZero")
	}
}
