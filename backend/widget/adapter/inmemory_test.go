package adapter_test

import (
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/widget/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/widget/app"
)

func TestInMemory_Conformance(t *testing.T) {
	repo := adapter.NewInMemory()
	app.TestWidgetRepository(t, repo)
}
