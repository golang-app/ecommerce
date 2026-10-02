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

func TestNewApp_PProfDisabledByDefault(t *testing.T) {
	is := is.New(t)
	t.Setenv("PPROF_ENABLED", "")
	app := application.New(context.Background(), 8080)

	is.Equal(app.PProfEnabled(), false)
}

func TestNewApp_PProfEnabledViaOption(t *testing.T) {
	is := is.New(t)
	app := application.New(context.Background(), 8080, application.WithPProf(true))

	is.Equal(app.PProfEnabled(), true)
}

func TestNewApp_PProfEnabledViaEnv(t *testing.T) {
	is := is.New(t)
	t.Setenv("PPROF_ENABLED", "true")
	app := application.New(context.Background(), 8080)

	is.Equal(app.PProfEnabled(), true)
}

