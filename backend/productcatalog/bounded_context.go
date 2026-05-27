package productcatalog

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
)

func New(db *sql.DB) (application.BoundedContext, app.ProductService) {
	storage := adapter.NewPostgres(db)
	appServ := app.NewProductService(storage)

	return &boundedContext{}, appServ
}

type boundedContext struct {
}
