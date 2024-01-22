package shipment

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
)

type boundedContext struct{}

func New(db *sql.DB) (application.BoundedContext, appService) {
	adapt := newPostgres(db)
	appServ := newAppService(adapt)

	return &boundedContext{}, appServ
}
