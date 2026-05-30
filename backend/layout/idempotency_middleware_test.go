package layout

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/internal/idempotency"
	"github.com/matryer/is"
)

// fakeStore is an in-memory IdempotencyStore. It stays in the layout
// package so the test does not have to widen any interface — the
// middleware depends on the IdempotencyStore seam exactly so this
// fake exists.
type fakeStore struct {
	mu       sync.Mutex
	rows     map[string]idempotency.StoredResponse
	findErr  error
	saveErr  error
	calls    struct{ find, save int }
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]idempotency.StoredResponse{}} }

func (s *fakeStore) Find(_ context.Context, key string) (idempotency.StoredResponse, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls.find++
	if s.findErr != nil {
		return idempotency.StoredResponse{}, false, s.findErr
	}
	r, ok := s.rows[key]
	return r, ok, nil
}

func (s *fakeStore) Save(_ context.Context, key, _, _ string, resp idempotency.StoredResponse, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls.save++
	if s.saveErr != nil {
		return s.saveErr
	}
	if _, exists := s.rows[key]; exists {
		// Mirror ON CONFLICT (key) DO NOTHING: first writer wins.
		return nil
	}
	s.rows[key] = resp
	return nil
}

func TestIdempotencyMiddleware_NoHeader_PassesThrough(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	var calls int
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})

	mw := IdempotencyMiddleware(store)(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/products", nil)
	mw.ServeHTTP(rec, req)

	is.Equal(calls, 1)
	is.Equal(rec.Code, http.StatusCreated)
	is.Equal(rec.Body.String(), "ok")
	is.Equal(store.calls.find, 0) // no key, no lookup
	is.Equal(store.calls.save, 0) // no key, no record
}

func TestIdempotencyMiddleware_SafeMethod_NeverConsultsStore(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("page"))
	})

	mw := IdempotencyMiddleware(store)(handler)
	rec := httptest.NewRecorder()
	// Safe method + an Idempotency-Key: the middleware MUST ignore it.
	req := httptest.NewRequest(http.MethodGet, "/admin/products", nil)
	req.Header.Set("Idempotency-Key", "k-safe")
	mw.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	is.Equal(store.calls.find, 0)
	is.Equal(store.calls.save, 0)
}

func TestIdempotencyMiddleware_FirstCallRecordsResponse(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	var calls int
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":42}`))
	})

	mw := IdempotencyMiddleware(store)(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/orders/o-1/refund", nil)
	req.Header.Set("Idempotency-Key", "k-first")
	mw.ServeHTTP(rec, req)

	is.Equal(calls, 1)
	is.Equal(rec.Code, http.StatusAccepted)
	is.Equal(rec.Body.String(), `{"id":42}`)
	is.Equal(store.calls.find, 1)
	is.Equal(store.calls.save, 1)

	// The recorded tuple matches the response that went out on the wire.
	saved := store.rows["k-first"]
	is.Equal(saved.StatusCode, http.StatusAccepted)
	is.Equal(string(saved.Body), `{"id":42}`)
	is.Equal(saved.Headers["Content-Type"], []string{"application/json"})
}

func TestIdempotencyMiddleware_SameKey_ReplaysWithoutCallingHandler(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	store.rows["k-replay"] = idempotency.StoredResponse{
		StatusCode: http.StatusCreated,
		Body:       []byte("replayed"),
		Headers:    map[string][]string{"Content-Type": {"text/plain"}, "X-Demo": {"yes"}},
	}
	var calls int
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	})

	mw := IdempotencyMiddleware(store)(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/promo-codes", nil)
	req.Header.Set("Idempotency-Key", "k-replay")
	mw.ServeHTTP(rec, req)

	is.Equal(calls, 0) // handler MUST NOT run on replay
	is.Equal(rec.Code, http.StatusCreated)
	is.Equal(rec.Body.String(), "replayed")
	is.Equal(rec.Header().Get("Content-Type"), "text/plain")
	is.Equal(rec.Header().Get("X-Demo"), "yes")
	is.Equal(store.calls.find, 1)
	is.Equal(store.calls.save, 0) // nothing new to record on a replay
}

func TestIdempotencyMiddleware_DifferentKeys_DoNotCollide(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	var calls int
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		// Echo the key so we can prove the per-key response is distinct.
		_, _ = w.Write([]byte("body-for-" + r.Header.Get("Idempotency-Key")))
	})

	mw := IdempotencyMiddleware(store)(handler)

	for _, key := range []string{"k-a", "k-b"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin/products", nil)
		req.Header.Set("Idempotency-Key", key)
		mw.ServeHTTP(rec, req)
		is.Equal(rec.Code, http.StatusOK)
		is.Equal(rec.Body.String(), "body-for-"+key)
	}

	// Both keys reached the handler — dedupe is per-key, exactly the
	// table shape says.
	is.Equal(calls, 2)
	is.Equal(string(store.rows["k-a"].Body), "body-for-k-a")
	is.Equal(string(store.rows["k-b"].Body), "body-for-k-b")
}

func TestIdempotencyMiddleware_FindError_FallsThroughToHandler(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	store.findErr = errors.New("transient db")
	var calls int
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("served"))
	})

	mw := IdempotencyMiddleware(store)(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/products", nil)
	req.Header.Set("Idempotency-Key", "k-error")
	mw.ServeHTTP(rec, req)

	// Storage error is observable in logs but MUST NOT block the request.
	is.Equal(calls, 1)
	is.Equal(rec.Code, http.StatusOK)
	is.Equal(rec.Body.String(), "served")
}

func TestIdempotencyMiddleware_HandlerWritesWithoutExplicitWriteHeader(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No WriteHeader call — net/http defaults to 200 on the first
		// Write. The recorder must capture that default so a replay
		// produces an identical response.
		_, _ = w.Write([]byte("implicit-200"))
	})

	mw := IdempotencyMiddleware(store)(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/products", nil)
	req.Header.Set("Idempotency-Key", "k-implicit")
	mw.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	saved := store.rows["k-implicit"]
	is.Equal(saved.StatusCode, http.StatusOK)
	is.Equal(string(saved.Body), "implicit-200")
}
