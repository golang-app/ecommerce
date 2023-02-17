package productcatalog

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/port"
	"github.com/gorilla/mux"
)

func New(db *sql.DB) (application.BoundedContext, app.ProductService) {
	storage := adapter.NewPostgres(db)
	appServ := app.NewProductService(storage)

	return &boundedContext{
		httpHandler: port.NewHTTP(appServ),
	}, appServ
}

type boundedContext struct {
	httpHandler port.HTTP
}

func (m boundedContext) MuxRegister(r *mux.Router) {
	r.HandleFunc("/api/v1/products", m.httpHandler.AllProducts)
	r.HandleFunc("/api/v1/product/{productID}", m.httpHandler.Product)
}
