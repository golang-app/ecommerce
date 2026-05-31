package layout

import (
	"context"
	_ "embed"
	"time"

	authAdapter "github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	fulfillmentDomain "github.com/bkielbasa/go-ecommerce/backend/fulfillment/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/bkielbasa/go-ecommerce/backend/internal/imagestore"
	"github.com/bkielbasa/go-ecommerce/backend/internal/mailer"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	pcdomain "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	reviewsDomain "github.com/bkielbasa/go-ecommerce/backend/reviews/domain"
	searchapp "github.com/bkielbasa/go-ecommerce/backend/search/app"
	shipDomain "github.com/bkielbasa/go-ecommerce/backend/shippinginfo/domain"
	storeDomain "github.com/bkielbasa/go-ecommerce/backend/store/domain"
	wishlistDomain "github.com/bkielbasa/go-ecommerce/backend/wishlist/domain"
	"github.com/sirupsen/logrus"
)

type catalogService interface {
	AllProducts(ctx context.Context) ([]pcdomain.Product, error)
	Newest(ctx context.Context, limit int) ([]pcdomain.Product, error)
	Find(ctx context.Context, id string) (pcdomain.Product, error)
	FindVariant(ctx context.Context, variantID string) (pcdomain.Product, pcdomain.Variant, error)
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

	ListStockMovements(ctx context.Context, variantID string, limit int) ([]pcdomain.StockMovement, error)
}

type cartService interface {
	AddToCart(ctx context.Context, sessID string, productID string, qty int) error
	Get(ctx context.Context, sessID string) (*domain.Cart, error)
}

// authService is the customer-side seam onto the auth bounded context.
// As of the customer/admin split it no longer carries IsAdmin /
// MustChangePassword — those belong to adminAuthService below.
type authService interface {
	Login(ctx context.Context, username string, password string) (*authDomain.Session, error)
	Logout(ctx context.Context, seesionID string) error
	CreateNewCustomer(ctx context.Context, email, password string) error
	FindByToken(ctx context.Context, sessToken string) (*authDomain.Session, error)
	ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error
	RequestPasswordReset(ctx context.Context, email string) (string, error)
	ResetPassword(ctx context.Context, rawToken, newPassword string) error
}

// adminAuthService is the operator-side seam onto the auth bounded
// context. It backs requireAdmin, the admin login/logout/change-password
// handlers, and the .IsAdmin template flag.
type adminAuthService interface {
	Login(ctx context.Context, email, password string) (*authDomain.Session, error)
	Logout(ctx context.Context, token string) error
	FindByToken(ctx context.Context, token string) (*authDomain.Session, error)
	ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error
	MustChangePassword(ctx context.Context, email string) (bool, error)
	FindByID(ctx context.Context, id string) (authAdapter.Admin, error)
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
// MarkShipped / MarkDelivered / Refund used to live here too; they
// moved to the fulfillment Process Manager (fulfillmentService below)
// because shipping/delivery/refund are operational concerns, not
// commercial ones. The Order aggregate's matching methods stay on the
// domain for replay/back-compat but the admin UI no longer drives them
// directly.
type checkoutCommands interface {
	Place(ctx context.Context, sessID, customerID, cardNumber string, shipTo checkoutDomain.Address, shipMethod checkoutDomain.ShippingMethod, payMethod checkoutDomain.PaymentMethod, discount promodomain.Discount) (checkoutDomain.Order, error)
	Cancel(ctx context.Context, orderID, customerID string) error
	AdminCancel(ctx context.Context, orderID string) error
}

// fulfillmentService is the narrow seam onto the fulfillment Process
// Manager. The admin's "ship/deliver/refund" controls drive it
// directly; the customer order page reads ByOrder to render the
// fulfillment summary alongside the commercial status.
type fulfillmentService interface {
	OnOrderPaid(ctx context.Context, orderID string, at time.Time) error
	Label(ctx context.Context, orderID, carrier, trackingCode string) error
	Ship(ctx context.Context, orderID string) error
	Deliver(ctx context.Context, orderID string) error
	Refund(ctx context.Context, orderID, reason string) error
	ByOrder(ctx context.Context, orderID string) (fulfillmentDomain.Fulfillment, error)
}

// promoService is the narrow seam the layout package needs from the
// promo bounded context. Resolve runs the live validity / per-customer
// checks for the checkout form; the CRUD methods power the admin pages.
type promoService interface {
	Resolve(ctx context.Context, code, customerID string, subtotal, shippingCost int64) (promodomain.Discount, error)
	Create(ctx context.Context, c promodomain.Code) error
	Update(ctx context.Context, c promodomain.Code) error
	Delete(ctx context.Context, code string) error
	Find(ctx context.Context, code string) (promodomain.Code, error)
	ListAll(ctx context.Context) ([]promodomain.Code, error)
}

// reviewsService is the narrow seam the layout package needs from the
// reviews bounded context: list / aggregate for the product page, submit /
// has-reviewed for the customer-facing review form, and the moderation
// surface (approve / reject / delete / list pending / list all) for the
// admin reviews page. Mirrors *reviews/app.Service.
type reviewsService interface {
	ListForProduct(ctx context.Context, productID string, limit int) ([]reviewsDomain.Review, error)
	AggregateForProducts(ctx context.Context, productIDs []string) (map[string]reviewsDomain.Aggregate, error)
	HasReviewed(ctx context.Context, productID, customerID string) (bool, error)
	Submit(ctx context.Context, productID, customerID, body string, rating int) error
	Delete(ctx context.Context, id string) error
	Approve(ctx context.Context, id string) error
	Reject(ctx context.Context, id string) error
	ListPending(ctx context.Context, limit int) ([]reviewsDomain.Review, error)
	ListAll(ctx context.Context, limit int) ([]reviewsDomain.Review, error)
}

// wishlistService is the narrow seam the layout package needs from the
// wishlist bounded context. Toggle drives the product-page heart button
// (returns the new state for the htmx outerHTML swap); ListByCustomer
// powers /account/wishlist; Contains decides which of the two button
// states (filled vs outline heart) to render on the product page.
type wishlistService interface {
	Toggle(ctx context.Context, customerID, variantID string) (bool, error)
	ListByCustomer(ctx context.Context, customerID string) ([]wishlistDomain.Item, error)
	Contains(ctx context.Context, customerID, variantID string) (bool, error)
}

// searchService is the narrow seam the layout package needs from the
// search OHS. It maps onto search/app.Service.Search — Search returns
// hits keyed by (kind, id) plus a relevance rank; the storefront uses
// it to drive the "product grid when q is set" path in AllProducts.
type searchService interface {
	Search(ctx context.Context, q string, opts searchapp.QueryOptions) ([]searchapp.Hit, error)
}

// storeService is the narrow seam the layout package needs from the
// store bounded context. ResolveByHost drives the per-request
// middleware that binds the active store to the request context; the
// rest of the surface powers the footer switcher / admin CRUD.
type storeService interface {
	ResolveByHost(ctx context.Context, host string) (storeDomain.Store, error)
	Find(ctx context.Context, id string) (storeDomain.Store, error)
	ListAll(ctx context.Context) ([]storeDomain.Store, error)
	Create(ctx context.Context, s storeDomain.Store) error
	Update(ctx context.Context, s storeDomain.Store) error
	Delete(ctx context.Context, id string) error
}

// checkoutQueries is the read side of the checkout context (CQRS); it returns
// dedicated read models, not the write aggregate.
type checkoutQueries interface {
	Find(ctx context.Context, id string) (checkoutQuery.OrderView, error)
	ListByCustomer(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error)
	ListAll(ctx context.Context) ([]checkoutQuery.OrderSummary, error)
	// HasPurchasedProduct gates the verified-buyer rule for the reviews
	// context; layout passes it through a tiny adapter when wiring reviews.
	HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error)
	// TodaysSales returns the per-currency analytics_daily_sales row for
	// today (UTC); the admin dashboard renders the result as a small card.
	TodaysSales(ctx context.Context) (map[string]checkoutQuery.DailySalesRow, error)
}

// New wires the layout bounded context. It also initialises the process-wide
// session cookie store from the supplied secret and Secure flag. Callers must
// supply a non-empty sessionSecret; main.go enforces the production-vs-default
// policy before calling here.
//
// csrfEnabled toggles the request-level CSRF check; production always wants
// true, and only local debugging should ever flip it to false (see
// cmd/web/config.go CSRFEnabled for the operator-facing knob).
//
// paymentsWebhookSecret + paymentsWebhookSrv power the
// POST /webhooks/payments endpoint. Pass an empty secret OR a nil
// service to skip the registration entirely — tests that don't care
// about webhooks then don't need to provide either.
func New(logger logrus.FieldLogger, cartSrv cartService, catalogSrv catalogService, authSrv authService, adminAuthSrv adminAuthService, checkoutSrv checkoutCommands, checkoutQry checkoutQueries, fulfillmentSrv fulfillmentService, shipSrv shippingService, reviewsSrv reviewsService, wishlistSrv wishlistService, promoSrv promoService, searchSrv searchService, storeSrv storeService, imageStore imagestore.Store, uploadsDir string, sessionSecret []byte, cookieSecure, csrfEnabled bool, mailerSrv mailer.Mailer, baseURL string, rates fx.Rates, paymentsWebhookSecret string, paymentsWebhookSrv paymentsWebhookService) application.BoundedContext {
	store = newCookieStore(sessionSecret, cookieSecure)
	setCSRFEnabled(csrfEnabled)
	return &boundedContext{
		handler: httpHandler{
			cartSrv:        cartSrv,
			catalogSrv:     catalogSrv,
			authSrv:        authSrv,
			adminAuthSrv:   adminAuthSrv,
			checkoutSrv:    checkoutSrv,
			checkoutQry:    checkoutQry,
			fulfillmentSrv: fulfillmentSrv,
			shipSrv:        shipSrv,
			reviewsSrv:     reviewsSrv,
			wishlistSrv:    wishlistSrv,
			promoSrv:       promoSrv,
			searchSrv:      searchSrv,
			storeSrv:       storeSrv,
			imageStore:     imageStore,
			mailer:         mailerSrv,
			baseURL:        baseURL,
			rates:          rates,
			logger:         logger,
		},
		uploadsDir:            uploadsDir,
		paymentsWebhookSecret: paymentsWebhookSecret,
		paymentsWebhookSrv:    paymentsWebhookSrv,
		logger:                logger,
	}
}

type boundedContext struct {
	handler               httpHandler
	uploadsDir            string
	paymentsWebhookSecret string
	paymentsWebhookSrv    paymentsWebhookService
	logger                logrus.FieldLogger
}
