package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/ardanlabs/conf"
	"github.com/bkielbasa/go-ecommerce/backend/auth"
	"github.com/bkielbasa/go-ecommerce/backend/cart"
	"github.com/bkielbasa/go-ecommerce/backend/checkout"
	checkoutintegration "github.com/bkielbasa/go-ecommerce/backend/checkout/integration"
	"github.com/bkielbasa/go-ecommerce/backend/shippinginfo"
	"github.com/bkielbasa/go-ecommerce/backend/internal"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/dependency"
	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
	"github.com/bkielbasa/go-ecommerce/backend/internal/imagestore"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/bkielbasa/go-ecommerce/backend/layout"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	logrustash "github.com/bshuster-repo/logrus-logstash-hook"
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

	if err = observability.RuntimeMetrics(ctx, appName); err != nil {
		logger.WithError(err).Fatal("failed to initialize runtime metrics")
	}

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
	checkoutBD, checkoutSrv, checkoutQry := checkout.New(db, cartSrv, bus, catalogService)
	shipSrv := shippinginfo.New(db)

	// Cross-context integration: when checkout reports an order paid, the cart
	// context empties the basket it was placed from.
	bus.Subscribe(checkoutintegration.OrderPaid{}.EventName(), func(ctx context.Context, e eventbus.Event) error {
		return cartSrv.Clear(ctx, e.(checkoutintegration.OrderPaid).SessionID)
	})

	app.AddBoundedContext(cartBD)

	imgStore := imagestore.NewDisk(cfg.UploadsDir, "/uploads")

	app.AddBoundedContext(layout.New(logger, cartSrv, catalogService, authService, checkoutSrv, checkoutQry, shipSrv, imgStore, cfg.UploadsDir, []byte(cfg.SessionSecret), cfg.CookieSecure, cfg.CSRFEnabled))
	// CSRF protection wraps every route on the application router. It must be
	// installed after layout.New has set up the session store (which the
	// middleware reads from) but before app.Run() begins serving.
	app.Use(layout.CSRFMiddleware)
	app.AddBoundedContext(pcBD)
	app.AddBoundedContext(authBD)
	app.AddBoundedContext(checkoutBD)

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

func newLogger(lvl logrus.Level, appName string) logrus.FieldLogger {
	instance := &logrus.Logger{
		Out:          os.Stderr,
		Formatter:    new(logrus.JSONFormatter),
		Hooks:        make(logrus.LevelHooks),
		Level:        lvl,
		ExitFunc:     os.Exit,
		ReportCaller: false,
	}

	conn, err := net.DialTimeout("tcp", "logstash:50000", time.Second)
	if err != nil {
		logrus.Error(err)
	}

	if conn != nil {
		hook := logrustash.New(conn, logrustash.DefaultFormatter(logrus.Fields{"app": appName}))
		instance.Hooks.Add(hook)
	}

	return instance.
		WithField("service.name", appName)
}
