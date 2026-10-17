package widget

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/widget/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/widget/app"
)

type boundedContext struct{}

func (b boundedContext) Name() string {
	return "widget"
}

func New(db *sql.DB) (application.BoundedContext, *app.Service) {
	repo := adapter.NewPostgres(db)
	srv := app.NewService(repo)
	return boundedContext{}, srv
}
