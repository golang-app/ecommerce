package adapter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/lib/pq"
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

// AddProductOptionType inserts a new option type row and seeds the chosen
// default value onto every existing variant's options jsonb (keyed by the
// option-type name) so they remain resolvable. Both writes run in one
// transaction.
func (db postgres) AddProductOptionType(ctx context.Context, productID, optionTypeID, name string, position int, values []string, variantDefault string) error {
	valuesJSON, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshal option values: %w", err)
	}

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO productcatalog_option_type (id, product_id, name, position, values)
		VALUES ($1, $2, $3, $4, $5)
	`, optionTypeID, productID, name, position, valuesJSON); err != nil {
		return fmt.Errorf("add option type: %w", err)
	}

	if _, err = tx.ExecContext(ctx, `
		UPDATE productcatalog_variant
		SET options = options || jsonb_build_object($2::text, $3::text)
		WHERE product_id = $1
	`, productID, name, variantDefault); err != nil {
		return fmt.Errorf("seed option default on variants: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit add option type: %w", err)
	}
	return nil
}

// UpdateProductOptionType renames/re-values an option type and, when the name
// changes, rekeys the option in every variant's options jsonb. Runs in one
// transaction.
func (db postgres) UpdateProductOptionType(ctx context.Context, productID, currentName, newName string, values []string) error {
	valuesJSON, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshal option values: %w", err)
	}

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `
		UPDATE productcatalog_option_type
		SET name = $3, values = $4
		WHERE product_id = $1 AND name = $2
	`, productID, currentName, newName, valuesJSON); err != nil {
		return fmt.Errorf("update option type: %w", err)
	}

	if newName != currentName {
		if _, err = tx.ExecContext(ctx, `
			UPDATE productcatalog_variant
			SET options = (options - $2) || jsonb_build_object($3::text, options->>$2)
			WHERE product_id = $1 AND options ? $2
		`, productID, currentName, newName); err != nil {
			return fmt.Errorf("rekey option on variants: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit update option type: %w", err)
	}
	return nil
}

// DeleteProductOptionType removes an option type and strips its key from every
// variant's options jsonb. Runs in one transaction.
func (db postgres) DeleteProductOptionType(ctx context.Context, productID, name string) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `
		DELETE FROM productcatalog_option_type
		WHERE product_id = $1 AND name = $2
	`, productID, name); err != nil {
		return fmt.Errorf("delete option type: %w", err)
	}

	if _, err = tx.ExecContext(ctx, `
		UPDATE productcatalog_variant
		SET options = options - $2
		WHERE product_id = $1
	`, productID, name); err != nil {
		return fmt.Errorf("strip option from variants: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit delete option type: %w", err)
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

// UpdateVariant updates a single variant row (sku, image, price, stock) by id.
func (db postgres) UpdateVariant(ctx context.Context, variantID, sku, image string, priceAmount int64, currency string, stock int) error {
	_, err := db.db.ExecContext(ctx, `
		UPDATE productcatalog_variant
		SET sku = $2, image_url = $3, price_amount = $4, price_currency = $5, stock = $6
		WHERE id = $1
	`, variantID, sku, image, priceAmount, currency, stock)
	if err != nil {
		return fmt.Errorf("update variant: %w", err)
	}
	return nil
}

// DeleteVariant removes a single variant row by id.
func (db postgres) DeleteVariant(ctx context.Context, variantID string) error {
	_, err := db.db.ExecContext(ctx, `DELETE FROM productcatalog_variant WHERE id = $1`, variantID)
	if err != nil {
		return fmt.Errorf("delete variant: %w", err)
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
	cats, err := db.productCategories(ctx, string(p.ID()))
	if err != nil {
		return domain.Product{}, err
	}
	attrs, err := db.productAttributes(ctx, string(p.ID()))
	if err != nil {
		return domain.Product{}, err
	}
	return p.WithCatalog(ots, vs).WithClassification(cats, attrs), nil
}

func (db postgres) productCategories(ctx context.Context, productID string) ([]domain.Category, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT c.id, c.name, c.slug, c.position
		FROM productcatalog_category c
		JOIN productcatalog_product_category pc ON pc.category_id = c.id
		WHERE pc.product_id = $1
		ORDER BY c.position, c.name
	`, productID)
	if err != nil {
		return nil, fmt.Errorf("query product categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Category
	for rows.Next() {
		var id, name, slug string
		var position int
		if err := rows.Scan(&id, &name, &slug, &position); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		out = append(out, domain.RebuildCategory(id, name, slug, position))
	}
	return out, rows.Err()
}

func (db postgres) productAttributes(ctx context.Context, productID string) ([]domain.AttributeValue, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.unit, t.kind, t.filterable, t.position, pa.num_value, pa.text_value
		FROM productcatalog_product_attribute pa
		JOIN productcatalog_attribute_type t ON t.id = pa.attribute_type_id
		WHERE pa.product_id = $1
		ORDER BY t.position, t.name
	`, productID)
	if err != nil {
		return nil, fmt.Errorf("query product attributes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.AttributeValue
	for rows.Next() {
		var id, name, unit, kind string
		var filterable bool
		var position int
		var num sql.NullFloat64
		var text sql.NullString
		if err := rows.Scan(&id, &name, &unit, &kind, &filterable, &position, &num, &text); err != nil {
			return nil, fmt.Errorf("scan attribute: %w", err)
		}
		t := domain.RebuildAttributeType(id, name, unit, domain.AttributeKind(kind), filterable, position)
		if t.IsNumeric() {
			out = append(out, domain.NewNumericValue(t, num.Float64))
		} else {
			out = append(out, domain.NewEnumValue(t, text.String))
		}
	}
	return out, rows.Err()
}

func (db postgres) All(ctx context.Context) ([]domain.Product, error) {
	q := `SELECT id, name, description, thumbnail, price_amount, price_currency, attribute_set_id FROM productcatalog_product ORDER BY id`

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

// Newest returns up to limit products ordered newest-first by created_at
// (ties broken by id), each hydrated with its catalog/classification the same
// way All does.
func (db postgres) Newest(ctx context.Context, limit int) ([]domain.Product, error) {
	q := `SELECT id, name, description, thumbnail, price_amount, price_currency, attribute_set_id
		FROM productcatalog_product
		ORDER BY created_at DESC, id DESC
		LIMIT $1`

	rows, err := db.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("cannot query newest products: %w", err)
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
		return nil, fmt.Errorf("cannot fetch newest products: %w", err)
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
	q := `SELECT id, name, description, thumbnail, price_amount, price_currency, attribute_set_id FROM productcatalog_product WHERE id = $1`
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
	var attributeSetID sql.NullString
	if err := s.Scan(&id, &name, &description, &thumbnail, &amount, &currency, &attributeSetID); err != nil {
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
	p, err := domain.NewProduct(pid, name, description, price, thumbnail)
	if err != nil {
		return domain.Product{}, err
	}
	return p.WithAttributeSet(attributeSetID.String), nil
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

// Categories returns every catalog category in display order.
func (db postgres) Categories(ctx context.Context) ([]domain.Category, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT id, name, slug, position FROM productcatalog_category ORDER BY position, name
	`)
	if err != nil {
		return nil, fmt.Errorf("query categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Category
	for rows.Next() {
		var id, name, slug string
		var position int
		if err := rows.Scan(&id, &name, &slug, &position); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		out = append(out, domain.RebuildCategory(id, name, slug, position))
	}
	return out, rows.Err()
}

// CreateCategory inserts a new category.
func (db postgres) CreateCategory(ctx context.Context, c domain.Category) error {
	_, err := db.db.ExecContext(ctx, `
		INSERT INTO productcatalog_category (id, name, slug, position)
		VALUES ($1, $2, $3, $4)
	`, c.ID(), c.Name(), c.Slug(), c.Position())
	if err != nil {
		return fmt.Errorf("create category: %w", err)
	}
	return nil
}

// UpdateCategory updates an existing category by id.
func (db postgres) UpdateCategory(ctx context.Context, c domain.Category) error {
	_, err := db.db.ExecContext(ctx, `
		UPDATE productcatalog_category SET name = $2, slug = $3, position = $4 WHERE id = $1
	`, c.ID(), c.Name(), c.Slug(), c.Position())
	if err != nil {
		return fmt.Errorf("update category: %w", err)
	}
	return nil
}

// DeleteCategory removes a category; its product links cascade.
func (db postgres) DeleteCategory(ctx context.Context, id string) error {
	_, err := db.db.ExecContext(ctx, `DELETE FROM productcatalog_category WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	return nil
}

// AllAttributeTypes returns every attribute type in display order.
func (db postgres) AllAttributeTypes(ctx context.Context) ([]domain.AttributeType, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT id, name, unit, kind, filterable, position
		FROM productcatalog_attribute_type
		ORDER BY position, name
	`)
	if err != nil {
		return nil, fmt.Errorf("query attribute types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.AttributeType
	for rows.Next() {
		var id, name, unit, kind string
		var filterable bool
		var position int
		if err := rows.Scan(&id, &name, &unit, &kind, &filterable, &position); err != nil {
			return nil, fmt.Errorf("scan attribute type: %w", err)
		}
		out = append(out, domain.RebuildAttributeType(id, name, unit, domain.AttributeKind(kind), filterable, position))
	}
	return out, rows.Err()
}

// CreateAttributeType inserts a new attribute type.
func (db postgres) CreateAttributeType(ctx context.Context, t domain.AttributeType) error {
	_, err := db.db.ExecContext(ctx, `
		INSERT INTO productcatalog_attribute_type (id, name, unit, kind, filterable, position)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, t.ID(), t.Name(), t.Unit(), string(t.Kind()), t.Filterable(), t.Position())
	if err != nil {
		return fmt.Errorf("create attribute type: %w", err)
	}
	return nil
}

// UpdateAttributeType updates an existing attribute type by id.
func (db postgres) UpdateAttributeType(ctx context.Context, t domain.AttributeType) error {
	_, err := db.db.ExecContext(ctx, `
		UPDATE productcatalog_attribute_type
		SET name = $2, unit = $3, kind = $4, filterable = $5, position = $6
		WHERE id = $1
	`, t.ID(), t.Name(), t.Unit(), string(t.Kind()), t.Filterable(), t.Position())
	if err != nil {
		return fmt.Errorf("update attribute type: %w", err)
	}
	return nil
}

// DeleteAttributeType removes an attribute type; its product links cascade.
func (db postgres) DeleteAttributeType(ctx context.Context, id string) error {
	_, err := db.db.ExecContext(ctx, `DELETE FROM productcatalog_attribute_type WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete attribute type: %w", err)
	}
	return nil
}

// attributeSetMembers loads a set's member attribute types ordered by the join
// table position.
func (db postgres) attributeSetMembers(ctx context.Context, setID string) ([]domain.AttributeType, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.unit, t.kind, t.filterable, t.position
		FROM productcatalog_attribute_set_item i
		JOIN productcatalog_attribute_type t ON t.id = i.attribute_type_id
		WHERE i.set_id = $1
		ORDER BY i.position
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("query attribute set members: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.AttributeType
	for rows.Next() {
		var id, name, unit, kind string
		var filterable bool
		var position int
		if err := rows.Scan(&id, &name, &unit, &kind, &filterable, &position); err != nil {
			return nil, fmt.Errorf("scan attribute set member: %w", err)
		}
		out = append(out, domain.RebuildAttributeType(id, name, unit, domain.AttributeKind(kind), filterable, position))
	}
	return out, rows.Err()
}

// AllAttributeSets returns every attribute set in display order, each hydrated
// with its ordered members.
func (db postgres) AllAttributeSets(ctx context.Context) ([]domain.AttributeSet, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT id, name, position FROM productcatalog_attribute_set ORDER BY position, name
	`)
	if err != nil {
		return nil, fmt.Errorf("query attribute sets: %w", err)
	}
	type setRow struct {
		id       string
		name     string
		position int
	}
	var base []setRow
	func() {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var sr setRow
			if err = rows.Scan(&sr.id, &sr.name, &sr.position); err != nil {
				return
			}
			base = append(base, sr)
		}
	}()
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("scan attribute sets: %w", err)
	}

	out := make([]domain.AttributeSet, 0, len(base))
	for _, sr := range base {
		members, err := db.attributeSetMembers(ctx, sr.id)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.RebuildAttributeSet(sr.id, sr.name, sr.position, members))
	}
	return out, nil
}

// FindAttributeSet returns a single attribute set (with members) by id,
// returning domain.ErrAttributeSetNotFound when missing.
func (db postgres) FindAttributeSet(ctx context.Context, id string) (domain.AttributeSet, error) {
	var name string
	var position int
	err := db.db.QueryRowContext(ctx, `
		SELECT name, position FROM productcatalog_attribute_set WHERE id = $1
	`, id).Scan(&name, &position)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AttributeSet{}, domain.ErrAttributeSetNotFound
	}
	if err != nil {
		return domain.AttributeSet{}, fmt.Errorf("find attribute set: %w", err)
	}
	members, err := db.attributeSetMembers(ctx, id)
	if err != nil {
		return domain.AttributeSet{}, err
	}
	return domain.RebuildAttributeSet(id, name, position, members), nil
}

// CreateAttributeSet inserts a new attribute set row (members are written
// separately via SetAttributeSetItems).
func (db postgres) CreateAttributeSet(ctx context.Context, s domain.AttributeSet) error {
	_, err := db.db.ExecContext(ctx, `
		INSERT INTO productcatalog_attribute_set (id, name, position) VALUES ($1, $2, $3)
	`, s.ID(), s.Name(), s.Position())
	if err != nil {
		return fmt.Errorf("create attribute set: %w", err)
	}
	return nil
}

// UpdateAttributeSet updates an existing attribute set's name and position by id.
func (db postgres) UpdateAttributeSet(ctx context.Context, s domain.AttributeSet) error {
	_, err := db.db.ExecContext(ctx, `
		UPDATE productcatalog_attribute_set SET name = $2, position = $3 WHERE id = $1
	`, s.ID(), s.Name(), s.Position())
	if err != nil {
		return fmt.Errorf("update attribute set: %w", err)
	}
	return nil
}

// DeleteAttributeSet removes an attribute set; its member items cascade.
func (db postgres) DeleteAttributeSet(ctx context.Context, id string) error {
	_, err := db.db.ExecContext(ctx, `DELETE FROM productcatalog_attribute_set WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete attribute set: %w", err)
	}
	return nil
}

// SetAttributeSetItems replaces a set's members: it deletes the existing items
// and inserts the given attribute type ids with position = their index in the
// slice (order matters), in a single transaction.
func (db postgres) SetAttributeSetItems(ctx context.Context, setID string, attributeTypeIDs []string) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM productcatalog_attribute_set_item WHERE set_id = $1`, setID); err != nil {
		return fmt.Errorf("clear attribute set items: %w", err)
	}
	for i, typeID := range attributeTypeIDs {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO productcatalog_attribute_set_item (set_id, attribute_type_id, position)
			VALUES ($1, $2, $3)
		`, setID, typeID, i); err != nil {
			return fmt.Errorf("insert attribute set item: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit attribute set items: %w", err)
	}
	return nil
}

// UpdateProduct updates the core product row (name, description, thumbnail,
// price) by id. Variants, categories and attributes are untouched.
func (db postgres) UpdateProduct(ctx context.Context, p domain.Product) error {
	_, err := db.db.ExecContext(ctx, `
		UPDATE productcatalog_product
		SET name = $2, description = $3, thumbnail = $4, price_amount = $5, price_currency = $6
		WHERE id = $1
	`, string(p.ID()), p.Name(), p.Description(), p.Thumbnail(), p.Price().Amount(), string(p.Price().Currency()))
	if err != nil {
		return fmt.Errorf("update product: %w", err)
	}
	return nil
}

// DeleteProduct removes a product row; variants, category and attribute links
// cascade via ON DELETE CASCADE foreign keys.
func (db postgres) DeleteProduct(ctx context.Context, id string) error {
	_, err := db.db.ExecContext(ctx, `DELETE FROM productcatalog_product WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete product: %w", err)
	}
	return nil
}

// SetVariantStock sets a single variant's stock level.
func (db postgres) SetVariantStock(ctx context.Context, variantID string, stock int) error {
	_, err := db.db.ExecContext(ctx, `UPDATE productcatalog_variant SET stock = $2 WHERE id = $1`, variantID, stock)
	if err != nil {
		return fmt.Errorf("set variant stock: %w", err)
	}
	return nil
}

// SetProductCategories replaces the product's category links: it deletes the
// existing links and inserts the given set in a single transaction.
func (db postgres) SetProductCategories(ctx context.Context, productID string, categoryIDs []string) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM productcatalog_product_category WHERE product_id = $1`, productID); err != nil {
		return fmt.Errorf("clear product categories: %w", err)
	}
	for _, cid := range categoryIDs {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO productcatalog_product_category (product_id, category_id) VALUES ($1, $2)
		`, productID, cid); err != nil {
			return fmt.Errorf("insert product category: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit product categories: %w", err)
	}
	return nil
}

// SetProductAttributeSet sets (or clears) the product's attribute_set_id. An
// empty setID is written as SQL NULL.
func (db postgres) SetProductAttributeSet(ctx context.Context, productID, setID string) error {
	var setVal sql.NullString
	if setID != "" {
		setVal = sql.NullString{String: setID, Valid: true}
	}
	_, err := db.db.ExecContext(ctx, `
		UPDATE productcatalog_product SET attribute_set_id = $2 WHERE id = $1
	`, productID, setVal)
	if err != nil {
		return fmt.Errorf("set product attribute set: %w", err)
	}
	return nil
}

// SetProductAttributes replaces the product's attribute rows: it deletes the
// existing rows and inserts the given set in a single transaction. Numeric
// assignments set num_value (text null); enum assignments set text_value (num
// null).
func (db postgres) SetProductAttributes(ctx context.Context, productID string, values []app.AttributeAssignment) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM productcatalog_product_attribute WHERE product_id = $1`, productID); err != nil {
		return fmt.Errorf("clear product attributes: %w", err)
	}
	for _, v := range values {
		var num sql.NullFloat64
		var text sql.NullString
		if v.Num != nil {
			num = sql.NullFloat64{Float64: *v.Num, Valid: true}
		} else {
			text = sql.NullString{String: v.Text, Valid: true}
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO productcatalog_product_attribute (product_id, attribute_type_id, num_value, text_value)
			VALUES ($1, $2, $3, $4)
		`, productID, v.TypeID, num, text); err != nil {
			return fmt.Errorf("insert product attribute: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit product attributes: %w", err)
	}
	return nil
}

// ListProducts returns the products matching the query. The WHERE clause is
// built dynamically with parameterised placeholders only (never interpolating
// user values), then each matching id is hydrated through the existing Find
// path so the products carry their full catalog/classification.
func (db postgres) ListProducts(ctx context.Context, q app.ProductQuery) ([]domain.Product, error) {
	query := `SELECT p.id FROM productcatalog_product p WHERE 1 = 1`
	var args []any
	next := func() string {
		args = append(args, nil)
		return fmt.Sprintf("$%d", len(args))
	}
	set := func(v any) string {
		ph := next()
		args[len(args)-1] = v
		return ph
	}

	if q.CategorySlug != "" {
		query += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM productcatalog_product_category pc
			JOIN productcatalog_category c ON c.id = pc.category_id
			WHERE pc.product_id = p.id AND c.slug = %s)`, set(q.CategorySlug))
	}

	for _, typeID := range sortedKeys(q.NumericRanges) {
		r := q.NumericRanges[typeID]
		clause := fmt.Sprintf(`SELECT 1 FROM productcatalog_product_attribute pa
			WHERE pa.product_id = p.id AND pa.attribute_type_id = %s`, set(typeID))
		if r.Min != nil {
			clause += fmt.Sprintf(` AND pa.num_value >= %s`, set(*r.Min))
		}
		if r.Max != nil {
			clause += fmt.Sprintf(` AND pa.num_value <= %s`, set(*r.Max))
		}
		query += fmt.Sprintf(` AND EXISTS (%s)`, clause)
	}

	for _, typeID := range sortedEnumKeys(q.EnumSelections) {
		values := q.EnumSelections[typeID]
		if len(values) == 0 {
			continue
		}
		idPH := set(typeID)
		valPH := set(pq.Array(values))
		query += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM productcatalog_product_attribute pa
			WHERE pa.product_id = p.id AND pa.attribute_type_id = %s AND pa.text_value = ANY(%s))`, idPH, valPH)
	}

	if s := strings.TrimSpace(q.Search); s != "" {
		ph := set("%" + s + "%")
		query += fmt.Sprintf(` AND (p.name ILIKE %s OR p.description ILIKE %s)`, ph, ph)
	}

	query += ` ORDER BY p.id`

	rows, err := db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	var ids []string
	func() {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				return
			}
			ids = append(ids, id)
		}
	}()
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("scan product ids: %w", err)
	}

	out := make([]domain.Product, 0, len(ids))
	for _, id := range ids {
		p, err := db.Find(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// Facets returns the filterable attribute types and their available options,
// optionally scoped to the products in a category.
func (db postgres) Facets(ctx context.Context, categorySlug string) ([]app.Facet, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT id, name, unit, kind, filterable, position
		FROM productcatalog_attribute_type
		WHERE filterable = true
		ORDER BY position, name
	`)
	if err != nil {
		return nil, fmt.Errorf("query filterable attribute types: %w", err)
	}
	var types []domain.AttributeType
	func() {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id, name, unit, kind string
			var filterable bool
			var position int
			if err = rows.Scan(&id, &name, &unit, &kind, &filterable, &position); err != nil {
				return
			}
			types = append(types, domain.RebuildAttributeType(id, name, unit, domain.AttributeKind(kind), filterable, position))
		}
	}()
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("scan attribute types: %w", err)
	}

	// scope restricts an attribute query to products in the given category.
	// args are appended in order; the attribute_type_id placeholder is $1.
	scope := ""
	if categorySlug != "" {
		scope = ` AND EXISTS (
			SELECT 1 FROM productcatalog_product_category pc
			JOIN productcatalog_category c ON c.id = pc.category_id
			WHERE pc.product_id = pa.product_id AND c.slug = $2)`
	}

	var facets []app.Facet
	for _, t := range types {
		if t.IsNumeric() {
			var min, max sql.NullFloat64
			q := `SELECT min(num_value), max(num_value) FROM productcatalog_product_attribute pa
				WHERE pa.attribute_type_id = $1` + scope
			args := []any{t.ID()}
			if categorySlug != "" {
				args = append(args, categorySlug)
			}
			if err := db.db.QueryRowContext(ctx, q, args...).Scan(&min, &max); err != nil {
				return nil, fmt.Errorf("facet numeric %s: %w", t.ID(), err)
			}
			if !min.Valid || !max.Valid {
				continue
			}
			lo, hi := min.Float64, max.Float64
			facets = append(facets, app.Facet{Type: t, Min: &lo, Max: &hi})
			continue
		}

		q := `SELECT DISTINCT text_value FROM productcatalog_product_attribute pa
			WHERE pa.attribute_type_id = $1 AND pa.text_value IS NOT NULL` + scope + ` ORDER BY text_value`
		args := []any{t.ID()}
		if categorySlug != "" {
			args = append(args, categorySlug)
		}
		vrows, err := db.db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("facet enum %s: %w", t.ID(), err)
		}
		var values []string
		func() {
			defer func() { _ = vrows.Close() }()
			for vrows.Next() {
				var v string
				if err = vrows.Scan(&v); err != nil {
					return
				}
				values = append(values, v)
			}
		}()
		if err = vrows.Err(); err != nil {
			return nil, fmt.Errorf("scan enum facet %s: %w", t.ID(), err)
		}
		if len(values) == 0 {
			continue
		}
		facets = append(facets, app.Facet{Type: t, Values: values})
	}
	return facets, nil
}

// InsertStockMovement appends a row to the inventory audit log. delta is the
// signed change in stock units; reason describes the cause; refOrderID is the
// triggering order id when the change came from checkout (empty for direct
// admin edits).
func (db postgres) InsertStockMovement(ctx context.Context, variantID string, delta int, reason, refOrderID string) error {
	if _, err := db.db.ExecContext(ctx, `
		INSERT INTO productcatalog_stock_movement (variant_id, delta, reason, ref_order_id)
		VALUES ($1, $2, $3, $4)
	`, variantID, delta, reason, refOrderID); err != nil {
		return fmt.Errorf("insert stock movement: %w", err)
	}
	return nil
}

// ListStockMovements returns up to limit movements newest-first. When
// variantID is empty the full log is returned (admin overview); otherwise
// the log is scoped to a single variant. limit must be positive; the caller
// (app layer) clamps zero/negative to a default.
func (db postgres) ListStockMovements(ctx context.Context, variantID string, limit int) ([]domain.StockMovement, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if variantID == "" {
		rows, err = db.db.QueryContext(ctx, `
			SELECT id, variant_id, delta, reason, ref_order_id, at
			FROM productcatalog_stock_movement
			ORDER BY id DESC
			LIMIT $1
		`, limit)
	} else {
		rows, err = db.db.QueryContext(ctx, `
			SELECT id, variant_id, delta, reason, ref_order_id, at
			FROM productcatalog_stock_movement
			WHERE variant_id = $1
			ORDER BY id DESC
			LIMIT $2
		`, variantID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("query stock movements: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.StockMovement
	for rows.Next() {
		var id int64
		var vID, reason, refOrderID string
		var delta int
		var at time.Time
		if err := rows.Scan(&id, &vID, &delta, &reason, &refOrderID, &at); err != nil {
			return nil, fmt.Errorf("scan stock movement: %w", err)
		}
		out = append(out, domain.NewStockMovement(id, vID, delta, reason, refOrderID, at))
	}
	return out, rows.Err()
}

// sortedKeys returns the map keys sorted, so generated SQL placeholders are
// stable and tests are deterministic.
func sortedKeys(m map[string]app.Range) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedEnumKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
