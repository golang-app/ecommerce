package adapter_test

import (
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/banner/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/banner/app"
)

func TestInMemoryRepository(t *testing.T) {
	repo := adapter.NewInMemory()
	app.TestBannerRepository(t, repo)
}
