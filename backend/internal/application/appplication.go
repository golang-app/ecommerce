package application

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/internal/dependency"
	"github.com/gorilla/mux"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// App is an instance of the whole application.
// It holds the basic information about all dependencies it has
// and application-wide configuration.
// Any Module can be registered using the app.AddModule() function
type App struct {
	httpServer *http.Server
	router     *mux.Router
	deps       *dependency.DependencyManager
}

// New creates a new instance of the application.
func New(ctx context.Context, port int) *App {
	r := mux.NewRouter()
	deps := dependency.New()

	// Liveness/readiness endpoints are deliberately excluded from tracing.
	// Kubernetes (and any other supervisor) polls them constantly; emitting
	// a span per probe drowns out real traffic and inflates the trace
	// backend for no diagnostic value.
	r.Use(otelmux.Middleware("go-ecommerce", otelmux.WithFilter(func(req *http.Request) bool {
		switch req.URL.Path {
		case "/healthyz", "/readyz":
			return false
		}
		return true
	})))
	r.HandleFunc("/healthyz", deps.Healthy)
	r.HandleFunc("/readyz", deps.Ready)

	// Auto-instrument the HTTP server: otelhttp emits the standard
	// http.server.duration histogram + request count with http.method /
	// http.status_code attributes, and otelmux (installed above as a router
	// middleware) fills in http.route based on the matched gorilla route so
	// the metrics aren't blown up by raw URL paths. The /healthyz and
	// /readyz endpoints are filtered out at the otelmux layer (above), which
	// keeps the noisy probes from drowning real traffic; otelhttp still
	// observes them but with the same route label, which Grafana can drop.
	handler := otelhttp.NewHandler(r, "http.server")

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}

	return &App{
		httpServer: httpServer,
		router:     r,
		deps:       deps,
	}
}

// For debugging purpose, it exports
func (app *App) Run() error {
	go func() {
		// it is used only for pprof debugging
		_ = http.ListenAndServe("localhost:6060", nil)
	}()

	err := app.httpServer.ListenAndServe()
	if err != http.ErrServerClosed {
		return err
	}

	return nil
}

func (app *App) Close(ctx context.Context) error {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = app.httpServer.Shutdown(ctx)
	}()

	for _, dep := range app.deps.All() {
		wg.Add(1)
		func(dep dependency.Dependency) {
			defer wg.Done()
			_ = dep.Close()
		}(dep)
	}

	wg.Wait()

	return nil
}

type MuxRegister interface {
	MuxRegister(*mux.Router)
}

// Use registers a request-level middleware on the underlying gorilla/mux
// router. Middlewares are applied in registration order and run for every
// route, including the /healthyz and /readyz endpoints registered at New().
// The /static and /uploads handlers also run through them, which is
// harmless for the middlewares we use (CSRF only enforces unsafe methods,
// otelmux is pure observation).
func (app *App) Use(mw mux.MiddlewareFunc) {
	app.router.Use(mw)
}

func (app *App) AddDependency(dep dependency.Dependency) {
	app.deps.Add(dep)
}

func (app *App) AddBoundedContext(bc BoundedContext) {
	if m, ok := bc.(MuxRegister); ok {
		m.MuxRegister(app.router)
	}
}

type BoundedContext interface {
}
