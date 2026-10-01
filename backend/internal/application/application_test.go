package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/matryer/is"
)

func TestNewApp_ConfiguresHTTPServerTimeouts(t *testing.T) {
	is := is.New(t)
	app := application.New(context.Background(), 8080)

	srv := app.HTTPServer()
	is.True(srv != nil)
	is.Equal(srv.ReadHeaderTimeout, 5*time.Second)
	is.Equal(srv.ReadTimeout, 15*time.Second)
	is.Equal(srv.WriteTimeout, 30*time.Second)
	is.Equal(srv.IdleTimeout, 60*time.Second)
}
