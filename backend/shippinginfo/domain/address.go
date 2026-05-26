package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrAddressNotFound = errors.New("address not found")
	ErrInvalidAddress  = errors.New("invalid address")
)

// Address is a saved shipping address in a customer's address book. Unlike
// the checkout context's per-order Address snapshot, this is an entity with
// identity and a mutable default flag.
type Address struct {
	id         string
	customerID string
	name       string
	street1    string
	street2    string
	city       string
	zip        string
	country    string
	isDefault  bool
	createdAt  time.Time
}

// NewAddress validates and builds a new saved address. street2 is optional.
func NewAddress(id, customerID, name, street1, street2, city, zip, country string, isDefault bool, createdAt time.Time) (Address, error) {
	for _, f := range []struct{ label, val string }{
		{"name", name},
		{"street", street1},
		{"city", city},
		{"zip code", zip},
		{"country", country},
	} {
		if strings.TrimSpace(f.val) == "" {
			return Address{}, fmt.Errorf("%w: %s is required", ErrInvalidAddress, f.label)
		}
	}
	return Address{
		id:         id,
		customerID: customerID,
		name:       strings.TrimSpace(name),
		street1:    strings.TrimSpace(street1),
		street2:    strings.TrimSpace(street2),
		city:       strings.TrimSpace(city),
		zip:        strings.TrimSpace(zip),
		country:    strings.TrimSpace(country),
		isDefault:  isDefault,
		createdAt:  createdAt,
	}, nil
}

// Rebuild reconstructs an address from storage without re-validating.
func Rebuild(id, customerID, name, street1, street2, city, zip, country string, isDefault bool, createdAt time.Time) Address {
	return Address{
		id: id, customerID: customerID, name: name, street1: street1,
		street2: street2, city: city, zip: zip, country: country,
		isDefault: isDefault, createdAt: createdAt,
	}
}

func (a Address) ID() string            { return a.id }
func (a Address) CustomerID() string    { return a.customerID }
func (a Address) Name() string          { return a.name }
func (a Address) Street1() string       { return a.street1 }
func (a Address) Street2() string       { return a.street2 }
func (a Address) City() string          { return a.city }
func (a Address) Zip() string           { return a.zip }
func (a Address) Country() string       { return a.country }
func (a Address) IsDefault() bool       { return a.isDefault }
func (a Address) CreatedAt() time.Time  { return a.createdAt }
func (a Address) IsZero() bool          { return a.id == "" }
