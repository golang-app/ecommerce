package layout

import (
	"context"
	_ "embed"

	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	"github.com/sirupsen/logrus"
)

type catalogService interface {
	AllProducts(ctx context.Context) ([]productcatalog.Product, error)
	Find(ctx context.Context, id string) (productcatalog.Product, error)
}

type cartService interface {
	AddToCart(ctx context.Context, sessID string, productID string, qty int) error
	Get(ctx context.Context, sessID string) (*domain.Cart, error)
}

type authService interface {
	Login(ctx context.Context, username string, password string) (*authDomain.Session, error)
	Logout(ctx context.Context, seesionID string) error
	CreateNewCustomer(ctx context.Context, email, password string) error
	FindByToken(ctx context.Context, sessToken string) (*authDomain.Session, error)
}

func New(logger logrus.FieldLogger, cartSrv cartService, catalogSrv catalogService, authSrv authService) application.BoundedContext {
	return &boundedContext{
		handler: httpHandler{
			cartSrv:    cartSrv,
			catalogSrv: catalogSrv,
			authSrv:    authSrv,
		},
		logger: logger,
	}
}

type boundedContext struct {
	handler httpHandler
	logger  logrus.FieldLogger
}
