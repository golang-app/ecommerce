package port

import (
	"context"

	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
)

type HTTP struct {
	auth authService
}

type authService interface {
	CreateNewCustomer(ctx context.Context, email, password string) error
	Login(ctx context.Context, email, password string) (*domain.Session, error)
	Logout(ctx context.Context, token string) error
	FindByToken(ctx context.Context, token string) (*domain.Session, error)
}

func NewHTTP(auth authService) HTTP {
	return HTTP{auth: auth}
}
