//go:build !integration

package app_test

import (
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
)

func init() {
	storage = adapter.NewInMemory()
}
