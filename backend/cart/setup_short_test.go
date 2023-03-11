//go:build !integration

package cart_test

import "github.com/bkielbasa/go-ecommerce/backend/cart/adapter"

func init() {
	storage = adapter.NewInMemory()
}
