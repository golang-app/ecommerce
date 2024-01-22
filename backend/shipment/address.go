package shipment

import (
	"fmt"
)

type Address struct {
	id         string
	customerID string
	street     string
	city       string
	state      string
	postalCode string
	country    string
}

type FieldValidationError struct {
	Field string
	Msg   string
}

func (e FieldValidationError) Error() string {
	return fmt.Sprintf("validation error on field '%s': %s", e.Field, e.Msg)
}

func NewAddress(id, customerID, street, city, state, postalCode, country string) (*Address, error) {
	if customerID == "" {
		return nil, FieldValidationError{Field: "customerID", Msg: "invalid customerID"}
	}

	addr := &Address{
		customerID: customerID,
	}

	if err := addr.SetStreet(street); err != nil {
		return nil, err
	}
	if err := addr.SetCity(city); err != nil {
		return nil, err
	}
	if err := addr.SetState(state); err != nil {
		return nil, err
	}
	if err := addr.SetPostalCode(postalCode); err != nil {
		return nil, err
	}
	if err := addr.SetCountry(country); err != nil {
		return nil, err
	}

	return addr, nil
}

func (a *Address) ID() string {
	return a.id
}

func (a *Address) CustomerID() string {
	return a.customerID
}

func (a *Address) Street() string {
	return a.street
}

func (a *Address) SetStreet(street string) error {
	if street == "" {
		return FieldValidationError{Field: "street", Msg: "invalid street"}
	}
	a.street = street
	return nil
}

func (a *Address) City() string {
	return a.city
}

func (a *Address) SetCity(city string) error {
	if city == "" {
		return FieldValidationError{Field: "city", Msg: "invalid city"}
	}
	a.city = city
	return nil
}

func (a *Address) State() string {
	return a.state
}

func (a *Address) SetState(state string) error {
	if state == "" {
		return FieldValidationError{Field: "state", Msg: "invalid state"}
	}
	a.state = state
	return nil
}

func (a *Address) SetPostalCode(postalCode string) error {
	if postalCode == "" {
		return FieldValidationError{Field: "postalCode", Msg: "invalid postal code"}
	}
	a.postalCode = postalCode
	return nil
}

func (a *Address) PostalCode() string {
	return a.postalCode
}

func (a *Address) Country() string {
	return a.country
}

func (a *Address) SetCountry(country string) error {
	if country == "" {
		return FieldValidationError{Field: "country", Msg: "invalid country"}
	}
	a.country = country
	return nil
}
