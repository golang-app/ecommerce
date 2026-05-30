//go:build integration

package adapter

// Postgres side of the promo storage conformance test. The same suite
// (app.RunStorageConformance) is invoked by the in-memory adapter under
// the default build tag, so any contract clause we add to the suite is
// enforced against both implementations.
//
// This file follows the convention established by cart and productcatalog:
// connect via POSTGRES_* env vars (with sensible local defaults) and wipe
// the promo tables before each adapter is handed back to the suite.

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/bkielbasa/go-ecommerce/backend/promo/app"
)

func TestPostgres_Conformance(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	app.RunStorageConformance(t, func() app.Storage {
		wipePromoTables(t, db)
		return NewPostgres(db)
	})
}

// openTestDB opens a connection against the POSTGRES_* env vars; the
// defaults match the project's docker-compose.yaml. The build-tag gate
// keeps this file (and its DB dependency) out of the default `go test`
// run.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	pass := getEnv("POSTGRES_PASSWORD", "postgres")
	conn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("POSTGRES_HOST", "localhost"),
		getEnv("POSTGRES_PORT", "5432"),
		getEnv("POSTGRES_USER", "postgres"),
		pass,
		getEnv("POSTGRES_DB", "ecommerce"),
	)
	db, err := sql.Open("postgres", conn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	return db
}

// wipePromoTables clears the promo catalogue + ledger before each
// conformance sub-test. ON DELETE CASCADE on promo_redemption means
// truncating promo_code would also clear the ledger, but we wipe both
// explicitly so a future schema change (e.g. dropping the cascade) does
// not silently leave stale rows behind.
func wipePromoTables(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`TRUNCATE TABLE promo_redemption, promo_code CASCADE`); err != nil {
		t.Fatalf("wipe promo tables: %v", err)
	}
}

func getEnv(name, def string) string {
	v := os.Getenv(name)
	if v == "" {
		v = def
	}
	return v
}
