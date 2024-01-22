package auth

import (
	"context"
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/auth/app"
	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
)

type appService interface {
	CreateNewCustomer(ctx context.Context, email, password string) error
	Login(ctx context.Context, username string, password string) (*domain.Session, error)
	Logout(ctx context.Context, sesionID string) error
	FindByToken(ctx context.Context, sessToken string) (*domain.Session, error)
}

func New(db *sql.DB) (application.BoundedContext, appService) {
	authStorage := adapter.NewPostgresAuthStorage(db)
	sessStorage := adapter.NewPostgresSessionStorage(db)
	appServ := app.NewAuth(authStorage, sessStorage)

	return &boundedContext{}, appServ
}

type boundedContext struct {
}
