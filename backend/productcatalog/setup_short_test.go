//go:build !integration

package productcatalog_test

import (
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
)

func init() {
	storage = productcatalog.NewInMemory()
}
