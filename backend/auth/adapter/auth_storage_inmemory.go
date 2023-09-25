package adapter

import (
	"context"

	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
)

type inMemory struct {
	customers map[string]Customer
}

// NewInMemoryAuthStorage creates a new in-memory storage for auth
// it's used for testing purposes
func NewInMemoryAuthStorage() *inMemory {
	return &inMemory{
		customers: make(map[string]Customer),
	}
}

func (i *inMemory) Create(ctx context.Context, email, hash string) error {
	if _, ok := i.customers[email]; ok {
		return domain.ErrCustomerExists
	}

	i.customers[email] = Customer{
		Username:     email,
		PasswordHash: hash,
	}

	return nil
}

func (i *inMemory) Find(ctx context.Context, email string) (Customer, error) {
	customer, ok := i.customers[email]
	if !ok {
		return customer, domain.ErrCustomerNotFound
	}

	return customer, nil
}
