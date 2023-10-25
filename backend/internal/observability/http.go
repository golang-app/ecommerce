package observability

import (
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/sirupsen/logrus"
)

func HTTPWrap(h http.HandlerFunc, logger logrus.FieldLogger) http.HandlerFunc {
	return LoggerMiddleware(https.WrapPanic(h), logger)
}
