package productcatalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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

func (db postgres) Add(ctx context.Context, p Product) error {
	q := `INSERT INTO productcatalog_product (id, name, description, thumbnail, price_amount, price_currency) 
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := db.db.ExecContext(ctx, q, p.ID(), p.Name(), p.Description(), p.Thumbnail(), p.Price().Amount(), p.Price().Currency())
	if err != nil {
		return fmt.Errorf("cannot add the product: %w", err)
	}

	return nil
}

func (db postgres) All(ctx context.Context) ([]Product, error) {
	q := `SELECT id, name, description, thumbnail, price_amount, price_currency FROM productcatalog_product`

	rows, err := db.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("cannot query products: %w", err)
	}

	products := []Product{}

	for rows.Next() {
		var id, name, description, thumbnail string
		var amount int64
		var currency string

		err = rows.Scan(&id, &name, &description, &thumbnail, &amount, &currency)
		if err != nil {
			return nil, fmt.Errorf("cannot scan product: %w", err)
		}

		pid, err := NewProductId(id)
		if err != nil {
			return nil, fmt.Errorf("cannot rebuild the product ID: %w", err)
		}

		cur, err := NewCurrency(currency)
		if err != nil {
			return nil, fmt.Errorf("cannot rebuild the product currency: %w", err)
		}

		priceVO, err := NewPrice(amount, cur)
		if err != nil {
			return nil, fmt.Errorf("cannot rebuild the product price: %w", err)
		}

		prod, err := NewProduct(pid, name, description, priceVO, thumbnail)
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

func (db postgres) Find(ctx context.Context, id string) (Product, error) {
	q := `SELECT name, description, thumbnail, price_amount, price_currency FROM productcatalog_product WHERE id = $1`

	row := db.db.QueryRowContext(ctx, q, id)
	var name, description, thumbnail string
	var amount int64
	var currency string

	err := row.Scan(&name, &description, &thumbnail, &amount, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, ErrProductNotFound
	}

	if err != nil {
		return Product{}, fmt.Errorf("cannot scan product: %w", err)
	}

	pId, err := NewProductId(id)
	if err != nil {
		return Product{}, fmt.Errorf("cannot build product: %w", err)
	}

	cur, err := NewCurrency(currency)
	if err != nil {
		return Product{}, fmt.Errorf("cannot rebuild product currency: %w", err)
	}

	priceVO, err := NewPrice(amount, cur)
	if err != nil {
		return Product{}, fmt.Errorf("cannot rebuild product price: %w", err)
	}

	return NewProduct(pId, name, description, priceVO, thumbnail)
}
