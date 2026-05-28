package layout

import (
	"context"
	_ "embed"

	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/imagestore"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	pcdomain "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	shipDomain "github.com/bkielbasa/go-ecommerce/backend/shippinginfo/domain"
	"github.com/sirupsen/logrus"
)

type catalogService interface {
	AllProducts(ctx context.Context) ([]pcdomain.Product, error)
	Newest(ctx context.Context, limit int) ([]pcdomain.Product, error)
	Find(ctx context.Context, id string) (pcdomain.Product, error)
	List(ctx context.Context, q pcapp.ProductQuery) ([]pcdomain.Product, error)
	Categories(ctx context.Context) ([]pcdomain.Category, error)
	Facets(ctx context.Context, categorySlug string) ([]pcapp.Facet, error)

	Add(ctx context.Context, id, name, desc string, priceMinorUnits int64, currency, thumbnail string) error
	AddVariantProduct(ctx context.Context, id, name, desc, currency, thumbnail string, optionTypes []pcapp.OptionTypeInput, variants []pcapp.VariantInput) error
	AddVariantToProduct(ctx context.Context, productID, sku, image string, priceMinor int64, currency string, stock int, options map[string]string) error
	AddOptionType(ctx context.Context, productID, name string, values []string, variantDefault string) error
	UpdateOptionType(ctx context.Context, productID, currentName, newName string, values []string) error
	DeleteOptionType(ctx context.Context, productID, name string) error
	UpdateVariant(ctx context.Context, variantID, sku, image string, priceMinor int64, currency string, stock int) error
	DeleteVariant(ctx context.Context, productID, variantID string) error
	UpdateProduct(ctx context.Context, id, name, desc string, priceMinorUnits int64, currency, thumbnail string) error
	DeleteProduct(ctx context.Context, id string) error
	SetVariantStock(ctx context.Context, variantID string, stock int) error
	SetProductCategories(ctx context.Context, productID string, categoryIDs []string) error
	SetProductAttributes(ctx context.Context, productID string, values []pcapp.AttributeAssignment) error
	SetProductAttributeSet(ctx context.Context, productID, setID string) error
	ProductAttributeTypes(ctx context.Context, productID string) ([]pcdomain.AttributeType, error)

	CreateCategory(ctx context.Context, name, slug string) error
	UpdateCategory(ctx context.Context, id, name, slug string, position int) error
	DeleteCategory(ctx context.Context, id string) error

	AttributeTypes(ctx context.Context) ([]pcdomain.AttributeType, error)
	AllAttributeTypes(ctx context.Context) ([]pcdomain.AttributeType, error)
	CreateAttributeType(ctx context.Context, name, unit string, kind pcdomain.AttributeKind, filterable bool) error
	UpdateAttributeType(ctx context.Context, id, name, unit string, kind pcdomain.AttributeKind, filterable bool, position int) error
	DeleteAttributeType(ctx context.Context, id string) error

	AttributeSets(ctx context.Context) ([]pcdomain.AttributeSet, error)
	FindAttributeSet(ctx context.Context, id string) (pcdomain.AttributeSet, error)
	CreateAttributeSet(ctx context.Context, name string, attributeTypeIDs []string) error
	UpdateAttributeSet(ctx context.Context, id, name string, attributeTypeIDs []string) error
	DeleteAttributeSet(ctx context.Context, id string) error
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
	MustChangePassword(ctx context.Context, email string) (bool, error)
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
	AdminCancel(ctx context.Context, orderID string) error
}

// checkoutQueries is the read side of the checkout context (CQRS); it returns
// dedicated read models, not the write aggregate.
type checkoutQueries interface {
	Find(ctx context.Context, id string) (checkoutQuery.OrderView, error)
	ListByCustomer(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error)
	ListAll(ctx context.Context) ([]checkoutQuery.OrderSummary, error)
}

// New wires the layout bounded context. It also initialises the process-wide
// session cookie store from the supplied secret and Secure flag. Callers must
// supply a non-empty sessionSecret; main.go enforces the production-vs-default
// policy before calling here.
//
// csrfEnabled toggles the request-level CSRF check; production always wants
// true, and only local debugging should ever flip it to false (see
// cmd/web/config.go CSRFEnabled for the operator-facing knob).
func New(logger logrus.FieldLogger, cartSrv cartService, catalogSrv catalogService, authSrv authService, checkoutSrv checkoutCommands, checkoutQry checkoutQueries, shipSrv shippingService, imageStore imagestore.Store, uploadsDir string, sessionSecret []byte, cookieSecure, csrfEnabled bool) application.BoundedContext {
	store = newCookieStore(sessionSecret, cookieSecure)
	setCSRFEnabled(csrfEnabled)
	return &boundedContext{
		handler: httpHandler{
			cartSrv:     cartSrv,
			catalogSrv:  catalogSrv,
			authSrv:     authSrv,
			checkoutSrv: checkoutSrv,
			checkoutQry: checkoutQry,
			shipSrv:     shipSrv,
			imageStore:  imageStore,
			logger:      logger,
		},
		uploadsDir: uploadsDir,
		logger:     logger,
	}
}

type boundedContext struct {
	handler    httpHandler
	uploadsDir string
	logger     logrus.FieldLogger
}
