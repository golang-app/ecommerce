package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ardanlabs/conf"
	"github.com/bkielbasa/go-ecommerce/backend/cart"
	"github.com/bkielbasa/go-ecommerce/backend/internal"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/dependency"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	log "github.com/sirupsen/logrus"
)

const tearDownTimeout = 5 * time.Second

func main() {
	cfg := config{}

	err := conf.Parse([]string{}, "", &cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Println(conf.Usage("", &cfg))
			return
		}
		log.Fatal(err)
	}

	ctx, cancel := internal.Context()
	defer cancel()

	app := application.New(ctx, cfg.ServerPort)

	connString := cfg.Postgres.connectionString()
	db, err := sql.Open("postgres", connString)
	if err != nil {
		log.Fatalf("cannot open connection to the DB: %s", err)
	}

	app.AddDependency(dependency.NewSQL(db))
	pcBD, cartService := productcatalog.New(db)

	app.AddBoundedContext(pcBD)
	app.AddBoundedContext(cart.New(db, cartService))

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
