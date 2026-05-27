package cart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/cart/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/cart/app"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	"github.com/sirupsen/logrus"
)

type productStorage interface {
	FindVariant(ctx context.Context, variantID string) (productcatalog.Product, productcatalog.Variant, error)
}

func New(db *sql.DB, logger logrus.FieldLogger, pc productStorage) (application.BoundedContext, app.CartService) {
	storage := adapter.NewPostgres(db)

	srv := app.NewCartService(storage, transformProductCatalog{pc})

	return &boundedContext{
		logger: logger,
	}, srv
}

// transformProductCatalog is part of Anti-Corruption Layer that prevents leaking
// productcatalog's types into the cart
type transformProductCatalog struct {
	pc productStorage
}

// Find resolves a variant id into the cart's notion of a product. A variant
// is the purchasable unit: the cart line's id is the variant id, its name is
// the product name plus the variant label (e.g. "Ceramic Mug — Red / L"), and
// its price is the variant's price.
func (tpc transformProductCatalog) Find(ctx context.Context, variantID string) (domain.Product, error) {
	p, v, err := tpc.pc.FindVariant(ctx, variantID)

	if errors.Is(err, productcatalog.ErrProductNotFound) {
		return domain.Product{}, domain.ErrProductNotFound
	}

	if err != nil {
		return domain.Product{}, err
	}

	if !v.InStock() {
		return domain.Product{}, domain.ErrOutOfStock
	}

	cur, err := domain.NewCurrency(string(v.Price().Currency()))
	if err != nil {
		return domain.Product{}, fmt.Errorf("cart: invalid currency from product catalog: %w", err)
	}

	name := p.Name()
	if label := v.Label(p.OptionTypes()); label != "" {
		name = name + " — " + label
	}

	return domain.NewProduct(v.ID(), name, v.Price().Amount(), cur), nil
}

type boundedContext struct {
	logger      logrus.FieldLogger
}
