package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	_ "github.com/lib/pq"
)

type postgres struct {
	db *sql.DB
}

func NewPostgres(db *sql.DB) postgres {
	return postgres{
		db: db,
	}
}

func (db postgres) Add(ctx context.Context, p domain.Product) error {
	q := `INSERT INTO productcatalog_product (id, name, description, thumbnail, price_amount, price_currency) 
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := db.db.ExecContext(ctx, q, p.ID(), p.Name(), p.Description(), p.Thumbnail(), p.Price().Amount(), p.Price().Currency())
	if err != nil {
		return fmt.Errorf("cannot add the product: %w", err)
	}

	return nil
}

func (db postgres) All(ctx context.Context) ([]domain.Product, error) {
	q := `SELECT id, name, description, thumbnail, price_amount, price_currency FROM productcatalog_product`

	rows, err := db.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("cannot query products: %w", err)
	}

	products := []domain.Product{}

	for rows.Next() {
		var id, name, description, thumbnail string
		var amount int
		var currency string

		err = rows.Scan(&id, &name, &description, &thumbnail, &amount, &currency)
		if err != nil {
			return nil, fmt.Errorf("cannot scan product: %w", err)
		}

		pid, err := domain.NewProductId(id)
		if err != nil {
			return nil, fmt.Errorf("cannot rebuild the product ID: %w", err)
		}

		prod, err := domain.NewProduct(pid, name, description, domain.NewPrice(float32(amount), currency), thumbnail)
		if err != nil {
			return nil, fmt.Errorf("cannot create product from data in the DB: %w", err)
		}
		products = append(products, prod)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot fetch products: %w", err)
	}

	return products, nil
}

func (db postgres) Find(ctx context.Context, id string) (domain.Product, error) {
	q := `SELECT name, description, thumbnail, price_amount, price_currency FROM productcatalog_product WHERE id = $1`

	row := db.db.QueryRowContext(ctx, q, id)
	var name, description, thumbnail string
	var amount int
	var currency string

	err := row.Scan(&name, &description, &thumbnail, &amount, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Product{}, domain.ErrProductNotFound
	}

	if err != nil {
		return domain.Product{}, fmt.Errorf("cannot scan product: %w", err)
	}

	pId, err := domain.NewProductId(id)
	if err != nil {
		return domain.Product{}, fmt.Errorf("cannot build product: %w", err)
	}

	return domain.NewProduct(pId, name, description, domain.NewPrice(float32(amount), currency), thumbnail)
}

func (db postgres) Reserve(ctx context.Context, name string) error {
	q := `SELECT id FROM productcatalog_product WHERE id = $1`
	row := db.db.QueryRowContext(ctx, q, name)

	var id string
	err := row.Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("cannot make the product ID reservation: %w", err)
	}

	return app.ErrIDInUse
}
