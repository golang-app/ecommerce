//go:build !integration

package auth_test

import "github.com/bkielbasa/go-ecommerce/backend/auth/adapter"

func init() {
	authStorage = adapter.NewInMemoryAuthStorage()
	sessStorage = adapter.NewInMemorySessionStorage()
}
