package domain

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidAddress = errors.New("invalid shipping address")

// Address is the shipping destination captured at checkout. It is a value
// object: validated on construction and immutable thereafter.
type Address struct {
	name    string
	street1 string
	street2 string
	city    string
	zip     string
	country string
}

// NewAddress validates and constructs a shipping address. street2 is
// optional; everything else is required.
func NewAddress(name, street1, street2, city, zip, country string) (Address, error) {
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
		name:    strings.TrimSpace(name),
		street1: strings.TrimSpace(street1),
		street2: strings.TrimSpace(street2),
		city:    strings.TrimSpace(city),
		zip:     strings.TrimSpace(zip),
		country: strings.TrimSpace(country),
	}, nil
}

// RebuildAddress reconstructs an Address from storage without re-validating.
// Used by the persistence layer, where the data was validated at write time
// (or is a legacy all-empty address).
func RebuildAddress(name, street1, street2, city, zip, country string) Address {
	return Address{
		name:    name,
		street1: street1,
		street2: street2,
		city:    city,
		zip:     zip,
		country: country,
	}
}

func (a Address) Name() string    { return a.name }
func (a Address) Street1() string { return a.street1 }
func (a Address) Street2() string { return a.street2 }
func (a Address) City() string    { return a.city }
func (a Address) Zip() string     { return a.zip }
func (a Address) Country() string { return a.country }

// IsZero reports whether the address is empty (e.g. a legacy order placed
// before shipping addresses were captured).
func (a Address) IsZero() bool { return a == Address{} }
