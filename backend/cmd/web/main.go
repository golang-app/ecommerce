package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/bkielbasa/go-ecommerce/backend/internal"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/port"
	"github.com/gorilla/mux"
	"github.com/kelseyhightower/envconfig"
)

const tearDownTimeout = 5 * time.Second

func main() {
	cfg := config{}

	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := internal.Context()
	conn := cfg.Postgres.connectionString()
	storage, err := adapter.NewPostgres(conn)
	if err != nil {
		log.Fatalf("cannot run the application: %s", err)
	}

	httpPort := port.NewHTTP(storage)

	r := mux.NewRouter()
	r.HandleFunc("/products", httpPort.AllProducts)
	r.HandleFunc("/product/{productID}", httpPort.Product)
	host := fmt.Sprintf(":%d", cfg.ServerPort)
	log.Infof("started listening on %s", host)

	serv := http.Server{
		Handler: r,
		Addr:    host,
	}

	go func() {
		err = serv.ListenAndServe()

		if !errors.Is(err, http.ErrServerClosed) {
			log.Infof("cannot start the HTTP server: %s", err)
		}
		cancel()
	}()

	<-ctx.Done()
	log.Info("stopping application")

	// we give some time to close all opened connection and tidy up everything
	shutDownCtx, shutDownCancel := context.WithTimeout(context.Background(), tearDownTimeout)
	defer shutDownCancel()

	err = serv.Shutdown(shutDownCtx)
	if err != nil {
		log.Errorf("cannot clearly close the application: %s", err)
	}

	log.Infof("application stopped")
}
