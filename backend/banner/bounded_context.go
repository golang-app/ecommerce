package banner

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/banner/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/banner/app"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
)

func New(db *sql.DB) (application.BoundedContext, *app.Service) {
	repo := adapter.NewPostgres(db)
	svc := app.NewService(repo)
	return &boundedContext{}, svc
}

type boundedContext struct{}
