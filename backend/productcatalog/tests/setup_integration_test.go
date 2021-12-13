//go:build integration

package tests

import "fmt"
import "github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"

func init() {
	conn := fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable", "localhost", 5432, "bartlomiejklimczak", "ecommerce")
	s, err := adapter.NewPostgres(conn)
	if err != nil {
		panic("cannot establish connection to postgres: " + err.Error())
	}

	storage = s
}
