//go:build integration

package adapter_test

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/bkielbasa/go-ecommerce/backend/banner/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/banner/app"
)

func TestPostgresRepository(t *testing.T) {
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	wipeBannerTable(t, db)

	repo := adapter.NewPostgres(db)
	app.TestBannerRepository(t, repo)
}

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

func ensureBannerTable(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS banner (
			id VARCHAR(36) PRIMARY KEY,
			slot VARCHAR(50) NOT NULL,
			title VARCHAR(255) NOT NULL,
			body TEXT NOT NULL DEFAULT '',
			link_url VARCHAR(500) NOT NULL DEFAULT '',
			link_text VARCHAR(100) NOT NULL DEFAULT '',
			promo_code VARCHAR(50) NOT NULL DEFAULT '',
			icon VARCHAR(50) NOT NULL DEFAULT '',
			image_url VARCHAR(500) NOT NULL DEFAULT '',
			display_order INT NOT NULL DEFAULT 0,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_banner_slot_active_order ON banner (slot, is_active, display_order ASC);
	`)
	if err != nil {
		t.Fatalf("ensure banner table: %v", err)
	}
}

func wipeBannerTable(t *testing.T, db *sql.DB) {
	t.Helper()
	ensureBannerTable(t, db)
	if _, err := db.Exec(`TRUNCATE TABLE banner CASCADE`); err != nil {
		t.Fatalf("wipe banner table: %v", err)
	}
}

func getEnv(name, def string) string {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	return v
}
