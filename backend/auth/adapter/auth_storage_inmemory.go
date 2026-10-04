package adapter

import (
	"context"
	"sort"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
)

type inMemory struct {
	mu        sync.RWMutex
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
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, ok := i.customers[email]; ok {
		return domain.ErrCustomerExists
	}

	i.customers[email] = Customer{
		Username:     email,
		PasswordHash: hash,
	}

	return nil
}

func (i *inMemory) UpdatePassword(ctx context.Context, email, hash string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	c, ok := i.customers[email]
	if !ok {
		return domain.ErrCustomerNotFound
	}
	c.PasswordHash = hash
	i.customers[email] = c
	return nil
}

func (i *inMemory) Find(ctx context.Context, email string) (Customer, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	customer, ok := i.customers[email]
	if !ok {
		return customer, domain.ErrCustomerNotFound
	}

	return customer, nil
}

func (i *inMemory) List(ctx context.Context) ([]string, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	var emails []string
	for email := range i.customers {
		emails = append(emails, email)
	}
	sort.Strings(emails)
	return emails, nil
}
