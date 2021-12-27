package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/port"
	"github.com/gorilla/mux"
	"github.com/kelseyhightower/envconfig"
)

func main() {
	cfg := config{}

	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal(err)
	}

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
	log.Printf("started listening on %s", host)
	http.ListenAndServe(host, r)
}
