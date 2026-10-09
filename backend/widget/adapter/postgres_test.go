//go:build integration

package adapter_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/widget/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/widget/app"
	_ "github.com/lib/pq"
)

func TestPostgres_Conformance(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STRING")
	if connStr == "" {
		connStr = "postgres://postgres:postgres@localhost:5432/ecommerce?sslmode=disable"
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	repo := adapter.NewPostgres(db)
	app.TestWidgetRepository(t, repo)
}
