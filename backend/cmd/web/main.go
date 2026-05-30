package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/conf"
	"github.com/bkielbasa/go-ecommerce/backend/auth"
	"github.com/bkielbasa/go-ecommerce/backend/cart"
	"github.com/bkielbasa/go-ecommerce/backend/checkout"
	checkoutdomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutintegration "github.com/bkielbasa/go-ecommerce/backend/checkout/integration"
	checkoutquery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/sweeper"
	"github.com/bkielbasa/go-ecommerce/backend/fulfillment"
	fulfillmentapp "github.com/bkielbasa/go-ecommerce/backend/fulfillment/app"
	fulfillmentintegration "github.com/bkielbasa/go-ecommerce/backend/fulfillment/integration"
	"github.com/bkielbasa/go-ecommerce/backend/internal"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/dependency"
	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/bkielbasa/go-ecommerce/backend/internal/imagestore"
	"github.com/bkielbasa/go-ecommerce/backend/internal/inbox"
	"github.com/bkielbasa/go-ecommerce/backend/internal/mailer"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/bkielbasa/go-ecommerce/backend/internal/outbox"
	"github.com/bkielbasa/go-ecommerce/backend/layout"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	"github.com/bkielbasa/go-ecommerce/backend/promo"
	"github.com/bkielbasa/go-ecommerce/backend/reviews"
	"github.com/bkielbasa/go-ecommerce/backend/search"
	"github.com/bkielbasa/go-ecommerce/backend/shippinginfo"
	"github.com/bkielbasa/go-ecommerce/backend/store"
	"github.com/bkielbasa/go-ecommerce/backend/wishlist"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"github.com/uptrace/opentelemetry-go-extra/otelsql"
)

const tearDownTimeout = 5 * time.Second
const appName = "go-ecommerce"

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		logrus.WithError(err).Fatal("failed to load .env file")
	}

	cfg := config{}

	err := conf.Parse([]string{}, "", &cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Println(conf.Usage("", &cfg))
			return
		}
		logrus.Fatal(err)
	}

	logger := newLogger(logrus.DebugLevel, appName)

	// Session secret hygiene: in production the operator MUST override the
	// default. In any other env we still log a loud WARN so a forgotten
	// SESSION_SECRET in staging/dev is impossible to miss.
	if cfg.SessionSecret == defaultSessionSecret {
		switch cfg.Env {
		case "prod", "production":
			logger.Fatal("SESSION_SECRET is set to the insecure default; refusing to start in production. Set SESSION_SECRET to a strong random value.")
		default:
			logger.Warn("SESSION_SECRET is set to the insecure default; this is acceptable only for local development. Set SESSION_SECRET to a strong random value before deploying.")
		}
	}

	ctx, cancel := internal.Context()
	defer cancel()

	tracerClose, _, err := observability.InitTracer(ctx, observability.TracerOptions{
		AppName: appName,
		Env:     cfg.Env,
	})
	if err != nil {
		logger.WithError(err).Fatal("failed to initialize tracer")
	}

	defer func() {
		if err = tracerClose(context.Background()); err != nil {
			logger.WithError(err).Error("failed to close tracer")
		}
	}()

	metricsClose, err := observability.RuntimeMetrics(ctx, appName)
	if err != nil {
		logger.WithError(err).Fatal("failed to initialize runtime metrics")
	}
	defer func() {
		if err = metricsClose(context.Background()); err != nil {
			logger.WithError(err).Error("failed to close metrics provider")
		}
	}()

	// Construct package-level application metric instruments AFTER the
	// MeterProvider is installed by RuntimeMetrics — otherwise the handles
	// would be bound to the no-op default provider and every increment
	// would silently drop. Run it once at boot; the helpers in
	// observability/appmetrics.go read the resulting handles unconditionally.
	observability.InitMetrics()

	// Bridge logrus into the OTLP log pipeline. Returns a noop closer when
	// OTEL_EXPORTER_OTLP_ENDPOINT is empty; the app keeps logging to
	// stderr unchanged in that case.
	logsClose, err := observability.InitLogs(ctx, appName, logger)
	if err != nil {
		logger.WithError(err).Fatal("failed to initialize OTel logs")
	}
	defer func() {
		if err = logsClose(context.Background()); err != nil {
			logger.WithError(err).Error("failed to close OTel log provider")
		}
	}()

	app := application.New(ctx, cfg.ServerPort)

	connString := cfg.Postgres.connectionString()
	db, err := otelsql.Open("postgres", connString)
	if err != nil {
		logrus.Fatalf("cannot open connection to the DB: %s", err)
	}

	app.AddDependency(dependency.NewSQL(db))
	bus := eventbus.New(logger)
	// Transactional Outbox store: same DB, used both as the writer (the
	// checkout adapter calls AppendTx inside its own tx) and as the
	// dispatcher's source of unsent rows.
	outboxStore := outbox.NewPostgres(db)
	// Inbox store: per-subscriber dedupe of redelivered Outbox events.
	// Each Outbox-driven subscriber is wrapped via inbox.Wrap below so
	// the at-least-once contract from the Outbox becomes effectively
	// exactly-once at the subscriber boundary. See internal/inbox.
	inboxStore := inbox.NewPostgres(db)
	// Search OHS: published-language storage + service. The same *app.Service
	// instance is wired as both productcatalog's SearchIndexer (write side)
	// and layout's searchService (read side) — one struct, two roles.
	searchBD, searchSrv := search.New(db)
	pcBD, catalogService := productcatalog.New(db, searchSrv)
	cartBD, cartSrv := cart.New(db, logger, catalogService)
	authBD, authService, adminAuthService := auth.New(db)
	// Pricing policies are pluggable Strategies — the composition root
	// chooses concrete implementations and the checkout service stays
	// agnostic. The defaults (FlatTaxStrategy / ThresholdShippingStrategy)
	// preserve the historical "flat percent tax, free shipping above a
	// configurable threshold" behaviour while leaving the door open for
	// per-jurisdiction tax, weight-based shipping etc. by swapping in a
	// different strategy here.
	taxStrategy := checkoutdomain.FlatTaxStrategy{RatePercent: cfg.TaxRatePercent}
	shippingStrategy := checkoutdomain.ThresholdShippingStrategy{FreeShippingThreshold: cfg.FreeShippingThreshold}
	checkoutBD, checkoutSrv, checkoutQry := checkout.New(db, cartSrv, outboxStore, catalogService, catalogService, taxStrategy, shippingStrategy)
	// Fulfillment Process Manager: subscribes to OrderPaid, spawns a
	// state-stored Fulfillment, and owns the operational lifecycle
	// (ship/deliver/refund). Stock release on refund goes through the
	// productcatalog port wired below — same seam checkout used to
	// call directly before that flow moved.
	//
	// orderDetailReaderAdapter is the ACL onto checkout's read side
	// the Ship command pulls through when it publishes the
	// OrderShippedECST integration event (event-carried state
	// transfer, alongside the notification-style OrderShipped). The
	// translation lives in this composition root so fulfillment
	// itself stays unaware of checkout's internal types.
	fulfillmentBD, fulfillmentSrv := fulfillment.New(db, bus, orderDetailReaderAdapter{q: checkoutQry})
	fulfillmentSrv = fulfillmentSrv.
		WithStockReleaser(catalogService).
		WithOrderLines(orderLinesAdapter{q: checkoutQry}).
		WithLogger(logger)
	// Promo bounded context: owns promo_code + promo_redemption. The
	// checkout service redeems through its narrow PromoRedeemer seam so
	// the math stays inside checkout while the ledger lives here.
	promoBD, promoSrv := promo.New(db)
	checkoutSrv = checkoutSrv.WithPromoRedeemer(promoSrv)
	shipSrv := shippinginfo.New(db)
	// Reviews context: depends on productcatalog (via FK in storage) and on
	// checkout's HasPurchasedProduct (wired through a tiny ACL on the
	// reviews side). Returns both the BoundedContext envelope and the
	// concrete service which layout consumes.
	reviewsBD, reviewsSrv := reviews.New(db, checkoutQry)
	// Wishlist context: owns its data wholly, depends only on
	// productcatalog_variant through the table's FK. Returns both the
	// BoundedContext envelope and the concrete service which layout consumes.
	wishlistBD, wishlistSrv := wishlist.New(db)
	// Store bounded context: owns the `store` table and powers the
	// per-request active-store resolution. The service is consumed by
	// layout (the per-request middleware + admin CRUD + footer
	// switcher); no other context depends on it.
	storeBD, storeSrv := store.New(db)

	// Mailer is the outbound-email abstraction. When SMTP_HOST is empty
	// (the dev default), New() returns a LogMailer that writes each email
	// to the structured log instead of dialling — keeping the app bootable
	// with no MailHog/SMTP relay running. Production always sets SMTP_HOST.
	//
	// The leaf mailer is wrapped in three service-level decorators. The
	// composition order matters — read it innermost-out:
	//
	//   1. RetryingMailer (innermost wrap): on transient SMTP failures,
	//      reissue up to 3 attempts with exponential backoff (200ms,
	//      400ms). Sits closest to the leaf because retries should be
	//      transparent to everything above.
	//   2. MetricsMailer: records gocommerce_emails_sent_total exactly
	//      once per logical Send. It MUST sit OUTSIDE retries — otherwise
	//      a single Message that succeeds on attempt 3 would emit two
	//      "failure" + one "success" observation instead of the single
	//      "success" that callers actually care about.
	//   3. LoggingMailer (outermost): one INFO breadcrumb per call,
	//      promoted to ERROR if the (already-retried, already-counted)
	//      send still failed. Outermost so a single noisy retry storm
	//      does not pollute the log with one entry per attempt.
	mailerSrv := mailer.New(mailer.Config{
		Host:     cfg.SMTPHost,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.MailFrom,
	}, logger)
	mailerSrv = mailer.NewRetrying(mailerSrv, 3, 200*time.Millisecond, time.Now)
	mailerSrv = mailer.NewMetrics(mailerSrv)
	mailerSrv = mailer.NewLogging(mailerSrv, logger)

	// Cross-context integration: the three OrderPaid subscribers are
	// registered with SubscribeWithID and wrapped through inbox.Wrap
	// so the outbox dispatcher's at-least-once redelivery becomes
	// effectively exactly-once at each subscriber. The subscriber
	// names are stable strings — they are the natural key in the
	// inbox_handled table, so renaming them is a migration concern.

	// 1. cart.clear-on-orderpaid — empty the basket the order was
	//    placed from. Clear() is a natural-id no-op the second time
	//    around, so the inbox wrap is belt-and-braces here: cheaper
	//    than the cart round-trip, and uniform with the other two.
	bus.SubscribeWithID(
		checkoutintegration.OrderPaid{}.EventName(),
		inbox.Wrap("cart.clear-on-orderpaid", inboxStore,
			func(ctx context.Context, _ int64, e eventbus.Event) error {
				return cartSrv.Clear(ctx, e.(checkoutintegration.OrderPaid).SessionID)
			},
		),
	)

	// 2. fulfillment.on-orderpaid — the fulfillment Process Manager
	//    spawns a new Fulfillment record in StatusScheduled. The
	//    handler is itself idempotent (FindByOrder + ErrAlreadyExists
	//    no-op); the inbox wrap stops the redelivery before it ever
	//    reaches the service.
	bus.SubscribeWithID(
		checkoutintegration.OrderPaid{}.EventName(),
		inbox.Wrap("fulfillment.on-orderpaid", inboxStore,
			func(ctx context.Context, _ int64, e eventbus.Event) error {
				paid := e.(checkoutintegration.OrderPaid)
				return fulfillmentSrv.OnOrderPaid(ctx, paid.OrderID, paid.At)
			},
		),
	)

	// 3. email.order-confirmation — render and dispatch the order
	//    confirmation email. Anonymous orders (CustomerID == "") are
	//    skipped — there is no inbox to mail. Any failure inside the
	//    subscriber is returned (and logged by the bus) but never
	//    aborts the publisher's own transaction; the cart-clearing
	//    subscriber above is unaffected.
	//
	//    IDEMPOTENCY. The Outbox dispatcher delivers integration
	//    events at-least-once: a process crash between Publish and
	//    MarkSent will republish the same OrderPaid on the next tick.
	//    Sending the confirmation email twice would be user-visible
	//    spam. Previously this subscriber kept an in-memory
	//    sync.Map dedupe set which did not survive process restarts
	//    (the only window where the redelivery actually happens).
	//    inbox.Wrap replaces it with a persistent
	//    (subscriber, event_id) row in inbox_handled keyed on the
	//    outbox row id, so the dedupe survives crashes.
	bus.SubscribeWithID(
		checkoutintegration.OrderPaid{}.EventName(),
		inbox.Wrap("email.order-confirmation", inboxStore,
			func(ctx context.Context, _ int64, e eventbus.Event) error {
				paid := e.(checkoutintegration.OrderPaid)
				if paid.CustomerID == "" {
					return nil
				}
				view, err := checkoutQry.Find(ctx, paid.OrderID)
				if err != nil {
					return fmt.Errorf("order confirmation: load view: %w", err)
				}
				msg, err := layout.RenderOrderConfirmation(view, cfg.BaseURL)
				if err != nil {
					return fmt.Errorf("order confirmation: render: %w", err)
				}
				// Make sure the recipient is the actual customer
				// email even if RenderOrderConfirmation derived it
				// from the view; the integration event is the
				// authoritative source.
				msg.To = paid.CustomerID
				if err := mailerSrv.Send(ctx, msg); err != nil {
					return fmt.Errorf("order confirmation: send: %w", err)
				}
				return nil
			},
		),
	)

	// 4. email.order-shipped — the ECST subscriber. This is the
	//    counterpart to email.order-confirmation: it renders a "your
	//    order has shipped" email and dispatches it via the same
	//    Mailer. The subscriber is wired to fulfillment.OrderShippedECST
	//    rather than the notification-style fulfillment.OrderShipped
	//    — the whole point of the ECST pattern is to make this
	//    subscriber operationally independent of checkout. Compare
	//    with the order-confirmation subscriber above which has to
	//    call back into checkoutQry.Find to materialise its render
	//    data; here the event itself is sufficient.
	//
	//    The inbox.Wrap key matches the cross-context naming
	//    convention used by the other subscribers — it is the
	//    natural key in inbox_handled so renaming it is a migration
	//    concern.
	bus.SubscribeWithID(
		fulfillmentintegration.OrderShippedECST{}.EventName(),
		inbox.Wrap("email.order-shipped", inboxStore,
			func(ctx context.Context, _ int64, e eventbus.Event) error {
				shipped := e.(fulfillmentintegration.OrderShippedECST)
				if shipped.Email == "" {
					return nil
				}
				msg, err := layout.RenderOrderShipped(shipped)
				if err != nil {
					return fmt.Errorf("order shipped: render: %w", err)
				}
				if err := mailerSrv.Send(ctx, msg); err != nil {
					return fmt.Errorf("order shipped: send: %w", err)
				}
				return nil
			},
		),
	)

	app.AddBoundedContext(cartBD)

	imgStore := imagestore.NewDisk(cfg.UploadsDir, "/uploads")

	// fxRates are static, operator-configured. They are NOT a live feed —
	// upgrading to a real provider only requires a different implementation
	// of fx.Rates. The conversion is purely a render transformation: orders
	// remain stored and charged in DefaultCurrency (USD).
	fxRates := fx.New(cfg.DefaultCurrency, cfg.SupportedCurrencies, cfg.FXRates, logger)

	app.AddBoundedContext(layout.New(logger, cartSrv, catalogService, authService, adminAuthService, checkoutSrv, checkoutQry, fulfillmentSrv, shipSrv, reviewsSrv, wishlistSrv, promoSrv, searchSrv, storeSrv, imgStore, cfg.UploadsDir, []byte(cfg.SessionSecret), cfg.CookieSecure, cfg.CSRFEnabled, mailerSrv, cfg.BaseURL, fxRates))
	// StoreMiddleware resolves the active store per request and binds
	// it on the request context. It MUST run before the CSRF middleware
	// so the store is available to every handler/template — including
	// the renders that mint the CSRF token.
	app.Use(layout.StoreMiddleware(storeSrv))
	// CSRF protection wraps every route on the application router. It must be
	// installed after layout.New has set up the session store (which the
	// middleware reads from) but before app.Run() begins serving.
	app.Use(layout.CSRFMiddleware)
	app.AddBoundedContext(pcBD)
	app.AddBoundedContext(authBD)
	app.AddBoundedContext(checkoutBD)
	app.AddBoundedContext(fulfillmentBD)
	app.AddBoundedContext(reviewsBD)
	app.AddBoundedContext(wishlistBD)
	app.AddBoundedContext(promoBD)
	app.AddBoundedContext(searchBD)
	app.AddBoundedContext(storeBD)

	// Reservation TTL sweeper: releases stock held by pending orders whose
	// confirmation never arrived (process crash, abandoned cart after stock
	// reserve, hung async payment). Bound to the application's lifecycle
	// context; cancel triggers a clean exit.
	reservationSweeper := sweeper.New(checkoutQry, checkoutSrv, cfg.ReservationTTL, cfg.ReservationSweepInterval, logger)
	go reservationSweeper.Run(ctx)

	// Transactional Outbox dispatcher. The decode closure is the
	// content-aware bridge from a stored (kind, payload) row back to
	// the integration event the bus's subscribers consume — the
	// outbox package itself stays type-agnostic so it can serve any
	// bounded context without imports.
	decode := func(kind string, payload []byte) (eventbus.Event, error) {
		switch kind {
		case checkoutintegration.OrderPaid{}.EventName():
			var e checkoutintegration.OrderPaid
			if err := json.Unmarshal(payload, &e); err != nil {
				return nil, fmt.Errorf("decode OrderPaid: %w", err)
			}
			return e, nil
		}
		return nil, fmt.Errorf("unknown outbox kind: %s", kind)
	}
	outboxDispatcher := outbox.NewDispatcher(outboxStore, bus, decode, logger, cfg.OutboxInterval)
	go outboxDispatcher.Run(ctx)

	go func() {
		_ = app.Run()
	}()

	logrus.Printf("server started on port %d", cfg.ServerPort)

	// we are waiting for the cancellation signal
	<-ctx.Done()

	logrus.Info("stopping application")

	// we give some time to close all opened connection and tidy up everything
	shutDownCtx, shutDownCancel := context.WithTimeout(context.Background(), tearDownTimeout)
	defer shutDownCancel()

	err = app.Close(shutDownCtx)
	if err != nil {
		logrus.Errorf("cannot clearly close the application: %s", err)
	}

	logrus.Infof("application stopped")
}

// orderLinesAdapter bridges the checkout query service to the
// fulfillment service's OrderLineSource port. Refund needs to know
// which variants/quantities to release back to the catalogue; the
// authoritative list lives on the order's read-side projection
// (OrderView.Items).
type orderLinesAdapter struct {
	q interface {
		Find(ctx context.Context, id string) (checkoutquery.OrderView, error)
	}
}

func (a orderLinesAdapter) OrderQuantities(ctx context.Context, orderID string) (map[string]int, error) {
	view, err := a.q.Find(ctx, orderID)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, ln := range view.Items() {
		out[ln.ProductID()] += ln.Quantity()
	}
	return out, nil
}

// orderDetailReaderAdapter bridges the checkout query service to the
// fulfillment service's OrderDetailReader port — the seam Ship's
// ECST publication path pulls through to assemble an
// OrderShippedECST event. The translation lives here, in the
// composition root, so the fulfillment package never imports
// checkout/query and downstream subscribers stay bound to
// fulfillment's published-language DTOs (see
// fulfillment/integration/events.go) rather than to checkout's
// internal value objects.
//
// CustomerID on the OrderView is the customer's email today
// (single-field identity); the adapter copies it into both
// CustomerID and Email on the fulfillment-owned OrderDetail. If the
// two ever diverge — e.g. a future change splits the columns — the
// fix is local to this adapter.
type orderDetailReaderAdapter struct {
	q interface {
		Find(ctx context.Context, id string) (checkoutquery.OrderView, error)
	}
}

func (a orderDetailReaderAdapter) OrderDetail(ctx context.Context, orderID string) (fulfillmentapp.OrderDetail, error) {
	view, err := a.q.Find(ctx, orderID)
	if err != nil {
		return fulfillmentapp.OrderDetail{}, err
	}
	items := make([]fulfillmentapp.OrderDetailLine, 0, len(view.Items()))
	for _, ln := range view.Items() {
		items = append(items, fulfillmentapp.OrderDetailLine{
			ProductID:     ln.ProductID(),
			ProductName:   ln.ProductName(),
			Quantity:      ln.Quantity(),
			PriceAmount:   ln.PriceAmount(),
			PriceCurrency: ln.PriceCurrency(),
		})
	}
	ship := view.ShipTo()
	return fulfillmentapp.OrderDetail{
		CustomerID: view.CustomerID(),
		Email:      view.CustomerID(),
		ShipTo: fulfillmentapp.OrderDetailAddress{
			Name:    ship.Name(),
			Street1: ship.Street1(),
			Street2: ship.Street2(),
			City:    ship.City(),
			Zip:     ship.Zip(),
			Country: ship.Country(),
		},
		Items:        items,
		Subtotal:     view.Subtotal(),
		Tax:          view.TaxAmount(),
		ShippingCost: view.ShippingCost(),
		Total:        view.TotalAmount(),
		Currency:     view.TotalCurrency(),
	}, nil
}

// newLogger builds the process-wide structured logger.
//
// Output: JSON to stderr. Every entry carries the service.name field so
// downstream tooling (kubectl logs, fluentd, etc.) can attribute the line
// to this app without parsing.
//
// In addition to stderr, log records are exported via OTLP when
// OTEL_EXPORTER_OTLP_ENDPOINT is configured — the bridge is installed by
// observability.InitLogs after construction (it attaches a logrus.Hook to
// the same underlying *logrus.Logger).
//
// The previous logstash TCP hook was removed: it tried to dial
// "logstash:50000" with a 1s timeout on every boot, which always failed in
// the standard dev compose-up and produced a spurious "lookup logstash: no
// such host" error in the logs. The OTel log pipeline supersedes it.
func newLogger(lvl logrus.Level, appName string) logrus.FieldLogger {
	instance := &logrus.Logger{
		Out:          os.Stderr,
		Formatter:    new(logrus.JSONFormatter),
		Hooks:        make(logrus.LevelHooks),
		Level:        lvl,
		ExitFunc:     os.Exit,
		ReportCaller: false,
	}

	return instance.WithField("service.name", appName)
}
