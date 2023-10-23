package cart

import (
	"context"
	"database/sql"
	"errors"

	"github.com/bkielbasa/go-ecommerce/backend/cart/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/cart/app"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	"github.com/sirupsen/logrus"
)

type productStorage interface {
	Find(ctx context.Context, id string) (productcatalog.Product, error)
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

func (tpc transformProductCatalog) Find(ctx context.Context, productID string) (domain.Product, error) {
	p, err := tpc.pc.Find(ctx, productID)

	if errors.Is(err, productcatalog.ErrProductNotFound) {
		return domain.Product{}, domain.ErrProductNotFound
	}

	if err != nil {
		return domain.Product{}, err
	}

	return domain.NewProduct(string(p.ID()), p.Name(), p.Price().Amount(), p.Price().Currency()), nil
}

type boundedContext struct {
	logger      logrus.FieldLogger
}
