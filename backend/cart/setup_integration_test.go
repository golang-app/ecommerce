//go:build integration

package cart_test

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/bkielbasa/go-ecommerce/backend/cart/adapter"
	_ "github.com/lib/pq"
)

func init() {
	conn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("POSTGRES_HOST", "localhost"),
		getEnv("POSTGRES_PORT", "5432"),
		getEnv("POSTGRES_USER", "postgres"),
		getEnv("POSTGRES_PASSWORD", "postgres"),
		getEnv("POSTGRES_DB", "ecommerce"))

	db, err := sql.Open("postgres", conn)
	if err != nil {
		panic("cannot establish connection to postgres: " + err.Error())
	}

	storage = adapter.NewPostgres(db)
}

func getEnv(name, def string) string {
	v := os.Getenv(name)
	if v == "" {
		v = def
	}

	return v
}
