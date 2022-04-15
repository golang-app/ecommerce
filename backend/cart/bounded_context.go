package cart

import (
	"context"
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/cart/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/cart/port"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	pcApp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/gorilla/mux"
)

func New(db *sql.DB, pc pcApp.ProductService) application.BoundedContext {
	storage := adapter.NewPostgres(db)

	return &boundedContext{
		httpHandler: port.NewHTTP(storage, transformProductCatalog{pc}),
	}
}

type transformProductCatalog struct {
	pc pcApp.ProductService
}

func (tpc transformProductCatalog) Find(ctx context.Context, productID string) (domain.Product, error) {
	p, err := tpc.pc.Find(ctx, productID)
	if err != nil {
		return domain.Product{}, err
	}

	return domain.NewProduct(string(p.ID()), p.Price().Amount()), nil
}

type boundedContext struct {
	httpHandler port.HTTP
}

func (m boundedContext) MuxRegister(r *mux.Router) {
	r.HandleFunc("/cart", port.EnsureSessionID(m.httpHandler.ShowCart))
	// r.HandleFunc("/product/{productID}", m.httpHandler.Product)
}
