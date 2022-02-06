package productcatalog

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/port"
	"github.com/gorilla/mux"
)

func New(db *sql.DB) application.Module {
	storage := adapter.NewPostgres(db)
	appServ := app.NewProductService(storage)

	return &module{
		httpHandler: port.NewHTTP(appServ),
	}
}

type module struct {
	httpHandler port.HTTP
}

func (m module) MuxRegister(r *mux.Router) {
	r.HandleFunc("/products", m.httpHandler.AllProducts)
	r.HandleFunc("/product/{productID}", m.httpHandler.Product)
}
