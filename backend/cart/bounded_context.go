package cart

import (
	"context"
	"database/sql"
	"errors"

	"github.com/bkielbasa/go-ecommerce/backend/cart/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/cart/port"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type productStorage interface {
	Find(ctx context.Context, id string) (productcatalog.Product, error)
}

func New(db *sql.DB, logger logrus.FieldLogger, pc productStorage) application.BoundedContext {
	storage := adapter.NewPostgres(db)

	return &boundedContext{
		logger:      logger,
		httpHandler: port.NewHTTP(storage, transformProductCatalog{pc}),
	}
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
	httpHandler port.HTTP
	logger      logrus.FieldLogger
}

func (m boundedContext) MuxRegister(r *mux.Router) {
	r.HandleFunc("/api/v1/cart", observability.LoggerMiddleware(https.WrapPanic(m.httpHandler.AddToCart), m.logger)).Methods("POST")
	r.HandleFunc("/api/v1/cart", observability.LoggerMiddleware(https.WrapPanic(m.httpHandler.ShowCart), m.logger)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/v1/cart/budge", observability.LoggerMiddleware(https.WrapPanic(m.httpHandler.Budge), m.logger)).Methods("GET", "OPTIONS")
}
