package adapter

import (
	"context"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
)

// inMemoryAdminStorage is the in-memory mirror of adminStoragePostgres,
// used by short tests. Keyed by id (the natural key); a secondary scan
// by email backs FindByEmail.
type inMemoryAdminStorage struct {
	mx     sync.Mutex
	admins map[string]Admin
}

func NewInMemoryAdminStorage() *inMemoryAdminStorage {
	return &inMemoryAdminStorage{admins: make(map[string]Admin)}
}

func (i *inMemoryAdminStorage) Upsert(ctx context.Context, id, email, passwordHash, role string, mustChangePassword bool) error {
	i.mx.Lock()
	defer i.mx.Unlock()
	i.admins[id] = Admin{
		ID:                 id,
		Email:              email,
		PasswordHash:       passwordHash,
		Role:               role,
		MustChangePassword: mustChangePassword,
	}
	return nil
}

func (i *inMemoryAdminStorage) FindByEmail(ctx context.Context, email string) (Admin, error) {
	i.mx.Lock()
	defer i.mx.Unlock()
	for _, a := range i.admins {
		if a.Email == email {
			return a, nil
		}
	}
	return Admin{}, domain.ErrAdminNotFound
}

func (i *inMemoryAdminStorage) FindByID(ctx context.Context, id string) (Admin, error) {
	i.mx.Lock()
	defer i.mx.Unlock()
	a, ok := i.admins[id]
	if !ok {
		return Admin{}, domain.ErrAdminNotFound
	}
	return a, nil
}

func (i *inMemoryAdminStorage) UpdatePassword(ctx context.Context, email, passwordHash string) error {
	i.mx.Lock()
	defer i.mx.Unlock()
	for id, a := range i.admins {
		if a.Email == email {
			a.PasswordHash = passwordHash
			i.admins[id] = a
			return nil
		}
	}
	return domain.ErrAdminNotFound
}

func (i *inMemoryAdminStorage) ClearMustChangePassword(ctx context.Context, email string) error {
	i.mx.Lock()
	defer i.mx.Unlock()
	for id, a := range i.admins {
		if a.Email == email {
			a.MustChangePassword = false
			i.admins[id] = a
			return nil
		}
	}
	return domain.ErrAdminNotFound
}
