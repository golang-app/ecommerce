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

	r.Use(otelmux.Middleware("go-ecommerce"))
	r.HandleFunc("/healthyz", deps.Healthy)
	r.HandleFunc("/readyz", deps.Ready)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", port),
		Handler: r,
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
