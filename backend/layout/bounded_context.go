package layout

import (
	"context"
	_ "embed"

	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	pcdomain "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	shipDomain "github.com/bkielbasa/go-ecommerce/backend/shippinginfo/domain"
	"github.com/sirupsen/logrus"
)

type catalogService interface {
	AllProducts(ctx context.Context) ([]pcdomain.Product, error)
	Find(ctx context.Context, id string) (pcdomain.Product, error)
	List(ctx context.Context, q pcapp.ProductQuery) ([]pcdomain.Product, error)
	Categories(ctx context.Context) ([]pcdomain.Category, error)
	Facets(ctx context.Context, categorySlug string) ([]pcapp.Facet, error)

	CreateCategory(ctx context.Context, name, slug string) error
	UpdateCategory(ctx context.Context, id, name, slug string, position int) error
	DeleteCategory(ctx context.Context, id string) error

	AttributeTypes(ctx context.Context) ([]pcdomain.AttributeType, error)
	CreateAttributeType(ctx context.Context, name, unit string, kind pcdomain.AttributeKind, filterable bool) error
	UpdateAttributeType(ctx context.Context, id, name, unit string, kind pcdomain.AttributeKind, filterable bool, position int) error
	DeleteAttributeType(ctx context.Context, id string) error
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
	IsAdmin(ctx context.Context, email string) (bool, error)
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

// checkoutCommands is the write side of the checkout context (CQRS).
type checkoutCommands interface {
	Place(ctx context.Context, sessID, customerID, cardNumber string, shipTo checkoutDomain.Address, shipMethod checkoutDomain.ShippingMethod, payMethod checkoutDomain.PaymentMethod) (checkoutDomain.Order, error)
	Cancel(ctx context.Context, orderID, customerID string) error
}

// checkoutQueries is the read side of the checkout context (CQRS); it returns
// dedicated read models, not the write aggregate.
type checkoutQueries interface {
	Find(ctx context.Context, id string) (checkoutQuery.OrderView, error)
	ListByCustomer(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error)
}

func New(logger logrus.FieldLogger, cartSrv cartService, catalogSrv catalogService, authSrv authService, checkoutSrv checkoutCommands, checkoutQry checkoutQueries, shipSrv shippingService) application.BoundedContext {
	return &boundedContext{
		handler: httpHandler{
			cartSrv:     cartSrv,
			catalogSrv:  catalogSrv,
			authSrv:     authSrv,
			checkoutSrv: checkoutSrv,
			checkoutQry: checkoutQry,
			shipSrv:     shipSrv,
		},
		logger: logger,
	}
}

type boundedContext struct {
	handler httpHandler
	logger  logrus.FieldLogger
}
