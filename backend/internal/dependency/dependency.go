package dependency

import (
	"context"
	"net/http"
)

// Dependency is used to tell your container orchestrator if your service
// is ready for the production trafic and it's healthy
// It can be used, for example, by Kubernetes to restart your pod
type Dependency interface {
	Healthy(context.Context) bool
	Ready(context.Context) bool
	Close() error
}

type DependencyManager struct {
	dependencies []Dependency
}

func New() *DependencyManager {
	return &DependencyManager{
		dependencies: []Dependency{},
	}
}

func (h *DependencyManager) Add(dep Dependency) {
	h.dependencies = append(h.dependencies, dep)
}

func (h *DependencyManager) Healthy(w http.ResponseWriter, r *http.Request) {
	for _, s := range h.dependencies {
		if !s.Healthy(r.Context()) {
			http.Error(w, "Unhealthy", http.StatusInternalServerError)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (h *DependencyManager) Ready(w http.ResponseWriter, r *http.Request) {
	for _, s := range h.dependencies {
		if !s.Ready(r.Context()) {
			http.Error(w, "Not ready", http.StatusInternalServerError)
		}
	}
	w.WriteHeader(http.StatusOK)
}
