package application

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"

	"github.com/bkielbasa/go-ecommerce/backend/internal/dependency"
	"github.com/gorilla/mux"
)

type App struct {
	httpServer *http.Server
	router     *mux.Router
	deps       *dependency.DependencyManager
}

func New(ctx context.Context, port int) *App {
	r := mux.NewRouter()
	deps := dependency.New()
	r.HandleFunc("/healthyz", deps.Healthy)
	r.HandleFunc("/readyz", deps.Ready)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: r,
	}

	return &App{
		httpServer: httpServer,
		router:     r,
		deps:       deps,
	}
}

func (app *App) Run() error {
	go http.ListenAndServe(":6060", nil)

	err := app.httpServer.ListenAndServe()
	if err != http.ErrServerClosed {
		return err
	}

	return nil
}

func (app *App) Close(ctx context.Context) error {
	return app.httpServer.Shutdown(ctx)
}

type MuxRegister interface {
	MuxRegister(*mux.Router)
}

func (app *App) AddDependency(dep dependency.Dependency) {
	app.deps.Add(dep)
}

func (app *App) AddModule(module Module) {
	if m, ok := module.(MuxRegister); ok {
		m.MuxRegister(app.router)
	}
}

type Module interface {
}
