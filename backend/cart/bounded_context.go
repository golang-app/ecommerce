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
	pcApp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	pcDomain "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/gorilla/mux"
)

func New(db *sql.DB, pc pcApp.ProductService) application.BoundedContext {
	storage := adapter.NewPostgres(db)

	return &boundedContext{
		httpHandler: port.NewHTTP(storage, transformProductCatalog{pc}),
	}
}

// transformProductCatalog is part of Anti-Corruption Layer that prevents leaking
// productcatalog's types into the cart
type transformProductCatalog struct {
	pc pcApp.ProductService
}

func (tpc transformProductCatalog) Find(ctx context.Context, productID string) (domain.Product, error) {
	p, err := tpc.pc.Find(ctx, productID)

	if errors.Is(err, pcDomain.ErrProductNotFound) {
		return domain.Product{}, domain.ErrProductNotFound
	}

	if err != nil {
		return domain.Product{}, err
	}

	return domain.NewProduct(string(p.ID()), p.Name(), p.Price().Amount(), p.Price().Currency()), nil
}

type boundedContext struct {
	httpHandler port.HTTP
}

func (m boundedContext) MuxRegister(r *mux.Router) {
	r.HandleFunc("/api/v1/cart", port.EnusreCartID(https.WrapPanic(m.httpHandler.AddToCart))).Methods("POST")
	r.HandleFunc("/api/v1/cart", port.EnusreCartID(https.WrapPanic(m.httpHandler.ShowCart))).Methods("GET")
}
