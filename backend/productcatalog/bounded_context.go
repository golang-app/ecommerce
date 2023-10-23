package productcatalog

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
)

func New(db *sql.DB) (application.BoundedContext, ProductService) {
	storage := NewPostgres(db)
	appServ := NewProductService(storage)

	return &boundedContext{}, appServ
}

type boundedContext struct {
}
