package dependency_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/internal/dependency"
	"github.com/matryer/is"
)

type mockPinger struct {
	pingErr  error
	closeErr error
	closed   bool
}

func (m *mockPinger) PingContext(ctx context.Context) error {
	return m.pingErr
}

func (m *mockPinger) Close() error {
	m.closed = true
	return m.closeErr
}

func TestSQLDependency_HealthyDoesNotPingDatabase(t *testing.T) {
	is := is.New(t)

	// Even when the database ping fails with an error, Healthy must return true
	// so that Kubernetes does not kill/restart the pod during transient DB degradation.
	mock := &mockPinger{pingErr: errors.New("connection refused")}
	dep := dependency.NewSQL(mock)

	is.True(dep.Healthy(context.Background()))
}

func TestSQLDependency_ReadyReflectsDatabasePing(t *testing.T) {
	is := is.New(t)

	// DB healthy -> Ready returns true
	mockHealthy := &mockPinger{pingErr: nil}
	depHealthy := dependency.NewSQL(mockHealthy)
	is.True(depHealthy.Ready(context.Background()))

	// DB unhealthy -> Ready returns false
	mockUnhealthy := &mockPinger{pingErr: errors.New("timeout")}
	depUnhealthy := dependency.NewSQL(mockUnhealthy)
	is.True(!depUnhealthy.Ready(context.Background()))
}

func TestDependencyManager_Healthy_PassesEvenIfDBDown(t *testing.T) {
	is := is.New(t)

	mgr := dependency.New()
	mock := &mockPinger{pingErr: errors.New("db down")}
	mgr.Add(dependency.NewSQL(mock))

	req := httptest.NewRequest(http.MethodGet, "/healthyz", nil)
	rec := httptest.NewRecorder()

	mgr.Healthy(rec, req)

	is.Equal(rec.Code, http.StatusOK)
}

func TestDependencyManager_Ready_FailsWhenDBDown(t *testing.T) {
	is := is.New(t)

	mgr := dependency.New()
	mock := &mockPinger{pingErr: errors.New("db down")}
	mgr.Add(dependency.NewSQL(mock))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	mgr.Ready(rec, req)

	is.Equal(rec.Code, http.StatusInternalServerError)
}

func TestDependencyManager_Ready_SucceedsWhenDBUp(t *testing.T) {
	is := is.New(t)

	mgr := dependency.New()
	mock := &mockPinger{pingErr: nil}
	mgr.Add(dependency.NewSQL(mock))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	mgr.Ready(rec, req)

	is.Equal(rec.Code, http.StatusOK)
}
