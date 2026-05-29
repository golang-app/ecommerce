package store

import (
	"context"

	"github.com/bkielbasa/go-ecommerce/backend/store/domain"
)

// storeKey is the unique context key value under which the active
// store for the current request is stored. Using a struct{} type
// (rather than a string) keeps the key collision-free with any other
// package that might also use context.WithValue.
type storeKey struct{}

// With returns a derived context that carries the supplied store. The
// layout's request middleware calls this once per request after
// resolving the store from the Host header; downstream handlers read
// the value back with From.
func With(ctx context.Context, s domain.Store) context.Context {
	return context.WithValue(ctx, storeKey{}, s)
}

// From returns the store the middleware bound to the context, plus a
// boolean indicating whether the binding existed. Handlers that just
// need the currency typically wrap this in a small helper that
// substitutes the default currency when the binding is absent.
func From(ctx context.Context) (domain.Store, bool) {
	s, ok := ctx.Value(storeKey{}).(domain.Store)
	return s, ok
}
