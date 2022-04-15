package adapter

import (
	"context"
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
)

type postgres struct {
	db *sql.DB
}

func NewPostgres(db *sql.DB) postgres {
	return postgres{db: db}
}

func (p postgres) Get(ctx context.Context, key string) (*domain.Cart, error) {
	return nil, nil
}
