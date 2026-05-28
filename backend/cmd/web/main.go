package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/conf"
	"github.com/bkielbasa/go-ecommerce/backend/auth"
	"github.com/bkielbasa/go-ecommerce/backend/cart"
	"github.com/bkielbasa/go-ecommerce/backend/checkout"
	checkoutapp "github.com/bkielbasa/go-ecommerce/backend/checkout/app"
	checkoutintegration "github.com/bkielbasa/go-ecommerce/backend/checkout/integration"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/sweeper"
	"github.com/bkielbasa/go-ecommerce/backend/internal"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/dependency"
	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
	"github.com/bkielbasa/go-ecommerce/backend/internal/imagestore"
	"github.com/bkielbasa/go-ecommerce/backend/internal/mailer"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/bkielbasa/go-ecommerce/backend/layout"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	"github.com/bkielbasa/go-ecommerce/backend/shippinginfo"
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
	pcBD, catalogService := productcatalog.New(db)
	cartBD, cartSrv := cart.New(db, logger, catalogService)
	authBD, authService := auth.New(db)
	pricing := checkoutapp.PricingPolicy{
		TaxRatePercent:        cfg.TaxRatePercent,
		FreeShippingThreshold: cfg.FreeShippingThreshold,
	}
	checkoutBD, checkoutSrv, checkoutQry := checkout.New(db, cartSrv, bus, catalogService, catalogService, pricing)
	shipSrv := shippinginfo.New(db)

	// Mailer is the outbound-email abstraction. When SMTP_HOST is empty
	// (the dev default), New() returns a LogMailer that writes each email
	// to the structured log instead of dialling — keeping the app bootable
	// with no MailHog/SMTP relay running. Production always sets SMTP_HOST.
	mailerSrv := mailer.New(mailer.Config{
		Host:     cfg.SMTPHost,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.MailFrom,
	}, logger)

	// Cross-context integration: when checkout reports an order paid, the cart
	// context empties the basket it was placed from.
	bus.Subscribe(checkoutintegration.OrderPaid{}.EventName(), func(ctx context.Context, e eventbus.Event) error {
		return cartSrv.Clear(ctx, e.(checkoutintegration.OrderPaid).SessionID)
	})

	// Second OrderPaid subscriber: render and dispatch the order
	// confirmation email. Anonymous orders (CustomerID == "") are
	// skipped — there is no inbox to mail. Any failure inside the
	// subscriber is returned (and logged by the bus) but never aborts
	// the publisher's own transaction; the cart-clearing subscriber
	// above is unaffected.
	bus.Subscribe(checkoutintegration.OrderPaid{}.EventName(), func(ctx context.Context, e eventbus.Event) error {
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
		// Make sure the recipient is the actual customer email even
		// if RenderOrderConfirmation derived it from the view; the
		// integration event is the authoritative source.
		msg.To = paid.CustomerID
		if err := mailerSrv.Send(ctx, msg); err != nil {
			return fmt.Errorf("order confirmation: send: %w", err)
		}
		return nil
	})

	app.AddBoundedContext(cartBD)

	imgStore := imagestore.NewDisk(cfg.UploadsDir, "/uploads")

	app.AddBoundedContext(layout.New(logger, cartSrv, catalogService, authService, checkoutSrv, checkoutQry, shipSrv, imgStore, cfg.UploadsDir, []byte(cfg.SessionSecret), cfg.CookieSecure, cfg.CSRFEnabled, mailerSrv, cfg.BaseURL))
	// CSRF protection wraps every route on the application router. It must be
	// installed after layout.New has set up the session store (which the
	// middleware reads from) but before app.Run() begins serving.
	app.Use(layout.CSRFMiddleware)
	app.AddBoundedContext(pcBD)
	app.AddBoundedContext(authBD)
	app.AddBoundedContext(checkoutBD)

	// Reservation TTL sweeper: releases stock held by pending orders whose
	// confirmation never arrived (process crash, abandoned cart after stock
	// reserve, hung async payment). Bound to the application's lifecycle
	// context; cancel triggers a clean exit.
	reservationSweeper := sweeper.New(checkoutQry, checkoutSrv, cfg.ReservationTTL, cfg.ReservationSweepInterval, logger)
	go reservationSweeper.Run(ctx)

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
