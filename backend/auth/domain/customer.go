package domain

import (
	"errors"
)

var (
	ErrCustomerExists   = errors.New("customer already exists")
	ErrCustomerNotFound = errors.New("customer not found")
)

type Customer struct {
	email string
}

func NewCustomer(email string) *Customer {
	return &Customer{
		email: email,
	}
}

func (c Customer) Email() string {
	return c.email
}
