//go:build !integration

package tests

import "github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"

func init() {
	storage = adapter.NewInMemory()
}
