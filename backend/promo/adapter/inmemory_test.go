package adapter

// In-memory side of the promo storage conformance test. The same suite
// (app.RunStorageConformance) is invoked by the postgres adapter test
// behind the `integration` build tag — both implementations exercise the
// identical contract.

import (
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/promo/app"
)

func TestInMemory_Conformance(t *testing.T) {
	app.RunStorageConformance(t, func() app.Storage {
		return NewInMemory()
	})
}
