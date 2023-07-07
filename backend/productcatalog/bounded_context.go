package productcatalog

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/gorilla/mux"
)

func New(db *sql.DB) (application.BoundedContext, ProductService) {
	storage := NewPostgres(db)
	appServ := NewProductService(storage)

	return &boundedContext{
		httpHandler: NewHTTP(appServ),
	}, appServ
}

type boundedContext struct {
	httpHandler HTTP
}

func (m boundedContext) MuxRegister(r *mux.Router) {
	r.HandleFunc("/api/v1/products", m.httpHandler.AllProducts)
	r.HandleFunc("/api/v1/product/{productID}", m.httpHandler.Product)
}
