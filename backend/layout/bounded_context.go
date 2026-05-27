package layout

import (
	"context"
	_ "embed"

	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	shipDomain "github.com/bkielbasa/go-ecommerce/backend/shippinginfo/domain"
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
	ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error
}

type shippingService interface {
	List(ctx context.Context, customerID string) ([]shipDomain.Address, error)
	Get(ctx context.Context, customerID, id string) (shipDomain.Address, error)
	Add(ctx context.Context, customerID, name, street1, street2, city, zip, country string) error
	Edit(ctx context.Context, customerID, id, name, street1, street2, city, zip, country string) error
	Remove(ctx context.Context, customerID, id string) error
	SetDefault(ctx context.Context, customerID, id string) error
	Default(ctx context.Context, customerID string) (shipDomain.Address, bool, error)
}

type checkoutService interface {
	Place(ctx context.Context, sessID, customerID, cardNumber string, shipTo checkoutDomain.Address, shipMethod checkoutDomain.ShippingMethod, payMethod checkoutDomain.PaymentMethod) (checkoutDomain.Order, error)
	Find(ctx context.Context, id string) (checkoutDomain.Order, error)
	ListByCustomer(ctx context.Context, customerID string) ([]checkoutDomain.Order, error)
	Cancel(ctx context.Context, orderID, customerID string) error
}

func New(logger logrus.FieldLogger, cartSrv cartService, catalogSrv catalogService, authSrv authService, checkoutSrv checkoutService, shipSrv shippingService) application.BoundedContext {
	return &boundedContext{
		handler: httpHandler{
			cartSrv:     cartSrv,
			catalogSrv:  catalogSrv,
			authSrv:     authSrv,
			checkoutSrv: checkoutSrv,
			shipSrv:     shipSrv,
		},
		logger: logger,
	}
}

type boundedContext struct {
	handler httpHandler
	logger  logrus.FieldLogger
}
