package domain_test

import (
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

func TestNewAddress_RequiresMandatoryFields(t *testing.T) {
	// street2 is intentionally omitted from the required set.
	base := []string{"Jane", "1 Main St", "apt 2", "PDX", "97201", "USA"}
	required := map[int]string{0: "name", 1: "street1", 3: "city", 4: "zip", 5: "country"}

	for idx := range required {
		args := append([]string(nil), base...)
		args[idx] = "   " // whitespace-only must be rejected
		_, err := domain.NewAddress(args[0], args[1], args[2], args[3], args[4], args[5])
		if !errors.Is(err, domain.ErrInvalidAddress) {
			t.Errorf("blank field %d: err = %v, want ErrInvalidAddress", idx, err)
		}
	}
}

func TestNewAddress_Street2Optional_AndTrims(t *testing.T) {
	a, err := domain.NewAddress(" Jane ", " 1 Main St ", "", " PDX ", " 97201 ", " USA ")
	if err != nil {
		t.Fatalf("NewAddress: %v", err)
	}
	if a.Name() != "Jane" || a.Street1() != "1 Main St" || a.City() != "PDX" {
		t.Errorf("fields not trimmed: %+v", a)
	}
	if a.Street2() != "" {
		t.Errorf("Street2 = %q, want empty", a.Street2())
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
