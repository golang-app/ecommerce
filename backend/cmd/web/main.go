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
	"github.com/bkielbasa/go-ecommerce/backend/internal"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/dependency"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/bkielbasa/go-ecommerce/backend/layout"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	logrustash "github.com/bshuster-repo/logrus-logstash-hook"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/opentelemetry-go-extra/otelsql"
)

const tearDownTimeout = 5 * time.Second
const appName = "go-ecommerce"

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	cfg := config{}

	err = conf.Parse([]string{}, "", &cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Println(conf.Usage("", &cfg))
			return
		}
		log.Fatal(err)
	}

	logger := newLogger(logrus.DebugLevel, appName)

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
		log.Fatalf("cannot open connection to the DB: %s", err)
	}

	app.AddDependency(dependency.NewSQL(db))
	pcBD, catalogService := productcatalog.New(db)
	cartBD, cartSrv := cart.New(db, logger, catalogService)
	authBD, authService := auth.New(db)
	app.AddBoundedContext(cartBD)

	app.AddBoundedContext(layout.New(logger, cartSrv, catalogService, authService))
	app.AddBoundedContext(pcBD)
	app.AddBoundedContext(authBD)

	go func() {
		_ = app.Run()
	}()

	log.Printf("server started on port %d", cfg.ServerPort)

	// we are waiting for the cancellation signal
	<-ctx.Done()

	log.Info("stopping application")

	// we give some time to close all opened connection and tidy up everything
	shutDownCtx, shutDownCancel := context.WithTimeout(context.Background(), tearDownTimeout)
	defer shutDownCancel()

	err = app.Close(shutDownCtx)
	if err != nil {
		log.Errorf("cannot clearly close the application: %s", err)
	}

	log.Infof("application stopped")
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
		log.Error(err)
	}

	if conn != nil {
		hook := logrustash.New(conn, logrustash.DefaultFormatter(log.Fields{"app": appName}))
		instance.Hooks.Add(hook)
	}

	return instance.
		WithField("service.name", appName)
}
