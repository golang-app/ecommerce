package app

import (
	"context"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/shippinginfo/domain"
)

type Storage interface {
	List(ctx context.Context, customerID string) ([]domain.Address, error)
	Get(ctx context.Context, customerID, id string) (domain.Address, error)
	Save(ctx context.Context, addr domain.Address) error
	Delete(ctx context.Context, customerID, id string) error
	ClearDefault(ctx context.Context, customerID string) error
	MarkDefault(ctx context.Context, customerID, id string) error
	Count(ctx context.Context, customerID string) (int, error)
}

// IDGenerator returns a fresh address id.
type IDGenerator func() string

type Service struct {
	storage Storage
	newID   IDGenerator
	now     func() time.Time
}

func NewService(storage Storage, newID IDGenerator) Service {
	return Service{storage: storage, newID: newID, now: func() time.Time { return time.Now().UTC() }}
}

// Add saves a new address. The customer's first address automatically becomes
// the default.
func (s Service) Add(ctx context.Context, customerID, name, street1, street2, city, zip, country string) error {
	count, err := s.storage.Count(ctx, customerID)
	if err != nil {
		return fmt.Errorf("count addresses: %w", err)
	}

	addr, err := domain.NewAddress(s.newID(), customerID, name, street1, street2, city, zip, country, count == 0, s.now())
	if err != nil {
		return err
	}
	return s.storage.Save(ctx, addr)
}

// Edit updates the editable fields of an existing address, preserving its
// default flag and creation time.
func (s Service) Edit(ctx context.Context, customerID, id, name, street1, street2, city, zip, country string) error {
	existing, err := s.storage.Get(ctx, customerID, id)
	if err != nil {
		return err
	}
	updated, err := domain.NewAddress(existing.ID(), customerID, name, street1, street2, city, zip, country, existing.IsDefault(), existing.CreatedAt())
	if err != nil {
		return err
	}
	return s.storage.Save(ctx, updated)
}

// Remove deletes an address. If the deleted address was the default and other
// addresses remain, the oldest remaining one is promoted to default.
func (s Service) Remove(ctx context.Context, customerID, id string) error {
	target, err := s.storage.Get(ctx, customerID, id)
	if err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, customerID, id); err != nil {
		return fmt.Errorf("delete address: %w", err)
	}

	if !target.IsDefault() {
		return nil
	}

	remaining, err := s.storage.List(ctx, customerID)
	if err != nil {
		return fmt.Errorf("list after delete: %w", err)
	}
	if len(remaining) == 0 {
		return nil
	}
	// List is ordered default-first then oldest-first; after deleting the
	// default, the first entry is the oldest remaining address.
	return s.SetDefault(ctx, customerID, remaining[0].ID())
}

// SetDefault makes one address the customer's default, clearing the flag on
// the others.
func (s Service) SetDefault(ctx context.Context, customerID, id string) error {
	if _, err := s.storage.Get(ctx, customerID, id); err != nil {
		return err
	}
	if err := s.storage.ClearDefault(ctx, customerID); err != nil {
		return fmt.Errorf("clear default: %w", err)
	}
	return s.storage.MarkDefault(ctx, customerID, id)
}

func (s Service) List(ctx context.Context, customerID string) ([]domain.Address, error) {
	return s.storage.List(ctx, customerID)
}

func (s Service) Get(ctx context.Context, customerID, id string) (domain.Address, error) {
	return s.storage.Get(ctx, customerID, id)
}

// Default returns the customer's default address, if any.
func (s Service) Default(ctx context.Context, customerID string) (domain.Address, bool, error) {
	addrs, err := s.storage.List(ctx, customerID)
	if err != nil {
		return domain.Address{}, false, err
	}
	for _, a := range addrs {
		if a.IsDefault() {
			return a, true, nil
		}
	}
	return domain.Address{}, false, nil
}
