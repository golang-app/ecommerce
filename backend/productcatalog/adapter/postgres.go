package adapter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

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

func (db postgres) AddOptionType(ctx context.Context, productID string, position int, ot domain.OptionType) error {
	values, err := json.Marshal(ot.Values())
	if err != nil {
		return fmt.Errorf("marshal option values: %w", err)
	}
	id := fmt.Sprintf("opt-%s-%d", productID, position)
	_, err = db.db.ExecContext(ctx, `
		INSERT INTO productcatalog_option_type (id, product_id, name, position, values)
		VALUES ($1, $2, $3, $4, $5)
	`, id, productID, ot.Name(), position, values)
	if err != nil {
		return fmt.Errorf("add option type: %w", err)
	}
	return nil
}

func (db postgres) AddVariant(ctx context.Context, productID string, position int, v domain.Variant) error {
	options, err := json.Marshal(v.Options())
	if err != nil {
		return fmt.Errorf("marshal variant options: %w", err)
	}
	_, err = db.db.ExecContext(ctx, `
		INSERT INTO productcatalog_variant (id, product_id, sku, image_url, price_amount, price_currency, position, options, stock)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, v.ID(), productID, v.SKU(), v.Image(), v.Price().Amount(), string(v.Price().Currency()), position, options, v.Stock())
	if err != nil {
		return fmt.Errorf("add variant: %w", err)
	}
	return nil
}

func (db postgres) optionTypes(ctx context.Context, productID string) ([]domain.OptionType, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT name, values FROM productcatalog_option_type
		WHERE product_id = $1 ORDER BY position
	`, productID)
	if err != nil {
		return nil, fmt.Errorf("query option types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.OptionType
	for rows.Next() {
		var name string
		var valuesRaw []byte
		if err := rows.Scan(&name, &valuesRaw); err != nil {
			return nil, fmt.Errorf("scan option type: %w", err)
		}
		var values []string
		if err := json.Unmarshal(valuesRaw, &values); err != nil {
			return nil, fmt.Errorf("unmarshal option values: %w", err)
		}
		out = append(out, domain.NewOptionType(name, values))
	}
	return out, rows.Err()
}

func (db postgres) variants(ctx context.Context, productID string) ([]domain.Variant, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT id, sku, image_url, price_amount, price_currency, options, stock FROM productcatalog_variant
		WHERE product_id = $1 ORDER BY position
	`, productID)
	if err != nil {
		return nil, fmt.Errorf("query variants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Variant
	for rows.Next() {
		v, err := scanVariant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (db postgres) withCatalog(ctx context.Context, p domain.Product) (domain.Product, error) {
	ots, err := db.optionTypes(ctx, string(p.ID()))
	if err != nil {
		return domain.Product{}, err
	}
	vs, err := db.variants(ctx, string(p.ID()))
	if err != nil {
		return domain.Product{}, err
	}
	return p.WithCatalog(ots, vs), nil
}

func (db postgres) All(ctx context.Context) ([]domain.Product, error) {
	q := `SELECT id, name, description, thumbnail, price_amount, price_currency FROM productcatalog_product ORDER BY id`

	rows, err := db.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("cannot query products: %w", err)
	}

	var base []domain.Product
	func() {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			p, err := scanProduct(rows)
			if err != nil {
				return
			}
			base = append(base, p)
		}
	}()
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot fetch products: %w", err)
	}

	products := make([]domain.Product, 0, len(base))
	for _, p := range base {
		full, err := db.withCatalog(ctx, p)
		if err != nil {
			return nil, err
		}
		products = append(products, full)
	}
	return products, nil
}

func (db postgres) Find(ctx context.Context, id string) (domain.Product, error) {
	q := `SELECT id, name, description, thumbnail, price_amount, price_currency FROM productcatalog_product WHERE id = $1`
	p, err := scanProduct(db.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Product{}, domain.ErrProductNotFound
	}
	if err != nil {
		return domain.Product{}, fmt.Errorf("cannot scan product: %w", err)
	}
	return db.withCatalog(ctx, p)
}

// FindVariant resolves a variant id to its variant and owning product.
func (db postgres) FindVariant(ctx context.Context, variantID string) (domain.Product, domain.Variant, error) {
	var productID string
	err := db.db.QueryRowContext(ctx, `SELECT product_id FROM productcatalog_variant WHERE id = $1`, variantID).Scan(&productID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Product{}, domain.Variant{}, domain.ErrProductNotFound
	}
	if err != nil {
		return domain.Product{}, domain.Variant{}, fmt.Errorf("find variant: %w", err)
	}
	p, err := db.Find(ctx, productID)
	if err != nil {
		return domain.Product{}, domain.Variant{}, err
	}
	v, ok := p.Variant(variantID)
	if !ok {
		return domain.Product{}, domain.Variant{}, domain.ErrProductNotFound
	}
	return p, v, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProduct(s rowScanner) (domain.Product, error) {
	var id, name, description, thumbnail, currency string
	var amount int64
	if err := s.Scan(&id, &name, &description, &thumbnail, &amount, &currency); err != nil {
		return domain.Product{}, err
	}
	pid, err := domain.NewProductId(id)
	if err != nil {
		return domain.Product{}, fmt.Errorf("rebuild product id: %w", err)
	}
	cur, err := domain.NewCurrency(currency)
	if err != nil {
		return domain.Product{}, fmt.Errorf("rebuild currency: %w", err)
	}
	price, err := domain.NewPrice(amount, cur)
	if err != nil {
		return domain.Product{}, fmt.Errorf("rebuild price: %w", err)
	}
	return domain.NewProduct(pid, name, description, price, thumbnail)
}

func scanVariant(s rowScanner) (domain.Variant, error) {
	var id, sku, image, currency string
	var amount int64
	var stock int
	var optionsRaw []byte
	if err := s.Scan(&id, &sku, &image, &amount, &currency, &optionsRaw, &stock); err != nil {
		return domain.Variant{}, err
	}
	cur, err := domain.NewCurrency(currency)
	if err != nil {
		return domain.Variant{}, fmt.Errorf("rebuild variant currency: %w", err)
	}
	price, err := domain.NewPrice(amount, cur)
	if err != nil {
		return domain.Variant{}, fmt.Errorf("rebuild variant price: %w", err)
	}
	var options map[string]string
	if err := json.Unmarshal(optionsRaw, &options); err != nil {
		return domain.Variant{}, fmt.Errorf("unmarshal variant options: %w", err)
	}
	return domain.NewVariant(id, sku, image, options, price, stock), nil
}

// Reserve atomically decrements stock for every variant in quantities. It is
// all-or-nothing: if any variant lacks sufficient stock the whole change is
// rolled back and ErrInsufficientStock is returned. Variants are locked in a
// stable (sorted) order to avoid deadlocks between concurrent reservations.
func (db postgres) Reserve(ctx context.Context, quantities map[string]int) error {
	if len(quantities) == 0 {
		return nil
	}

	ids := make([]string, 0, len(quantities))
	for id := range quantities {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, id := range ids {
		qty := quantities[id]
		if qty <= 0 {
			continue
		}
		var res sql.Result
		res, err = tx.ExecContext(ctx, `
			UPDATE productcatalog_variant SET stock = stock - $2
			WHERE id = $1 AND stock >= $2
		`, id, qty)
		if err != nil {
			return fmt.Errorf("reserve %s: %w", id, err)
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			err = domain.ErrInsufficientStock
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit reservation: %w", err)
	}
	return nil
}

// Release returns previously-reserved stock (best-effort, e.g. when a payment
// fails after the reservation succeeded).
func (db postgres) Release(ctx context.Context, quantities map[string]int) error {
	for id, qty := range quantities {
		if qty <= 0 {
			continue
		}
		if _, err := db.db.ExecContext(ctx, `UPDATE productcatalog_variant SET stock = stock + $2 WHERE id = $1`, id, qty); err != nil {
			return fmt.Errorf("release %s: %w", id, err)
		}
	}
	return nil
}
