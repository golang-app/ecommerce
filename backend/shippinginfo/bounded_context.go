package shippinginfo

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"

	"github.com/bkielbasa/go-ecommerce/backend/shippinginfo/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/shippinginfo/app"
)

// New wires the shipping-info (address book) service. It has no HTTP surface
// of its own — the account pages in the layout context consume the service.
func New(db *sql.DB) app.Service {
	storage := adapter.NewPostgres(db)
	return app.NewService(storage, newAddressID)
}

func newAddressID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "addr-" + hex.EncodeToString(b)
}
