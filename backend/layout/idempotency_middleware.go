package layout

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/internal/idempotency"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/gorilla/mux"
)

// idempotencyHeader is the standard request header clients use to
// stamp a retry-safe key onto an unsafe HTTP request. Values are
// opaque to the server — the middleware only cares that the same
// string arrives on the retry.
const idempotencyHeader = "Idempotency-Key"

// idempotencyTTL bounds how long a recorded response is replayable.
// 24h matches the column default and is documented in the package
// doc on internal/idempotency: long enough to cover a realistic
// human-driven retry window, short enough to keep the cache surface
// modest.
const idempotencyTTL = 24 * time.Hour

// IdempotencyStore is the storage seam the middleware depends on.
// Defined as an interface (rather than the concrete *idempotency.Postgres)
// so the middleware test can drive an in-memory fake without standing
// up a database.
type IdempotencyStore interface {
	Find(ctx context.Context, key string) (idempotency.StoredResponse, bool, error)
	Save(ctx context.Context, key, method, path string, resp idempotency.StoredResponse, ttl time.Duration) error
}

// IdempotencyMiddleware returns a gorilla/mux MiddlewareFunc that
// implements the HTTP-boundary Idempotency-Key pattern.
//
// Wiring. The composition root in cmd/web/main.go must install it on
// the same App.Use pipeline that already registers CSRFMiddleware:
//
//	app.Use(layout.IdempotencyMiddleware(idempotency.NewPostgres(db)))
//
// (cmd/web/main.go is off-limits to the agent that wrote this code; a
// follow-up commit lands the one-line wiring change.)
//
// Behaviour, per request:
//   - Safe methods (GET/HEAD/OPTIONS) are forwarded unchanged. Safe
//     methods MUST NOT mutate state, so a "retry" of a GET is by
//     definition idempotent already; spending a DB hit per render to
//     prove that adds latency for zero correctness gain.
//   - Unsafe methods (POST/PUT/PATCH/DELETE) are inspected for the
//     `Idempotency-Key` header. Empty header => pass-through (the
//     middleware is opt-IN at the client, not the route — see the
//     opt-in handlers' Go doc comments). Non-empty header =>
//     Find first; replay on hit; otherwise wrap the ResponseWriter
//     with a recorder, call next, and Save what the handler emitted.
//   - Find / Save errors are logged but never block the request. The
//     point of the table is retry safety, not availability.
//
// Why scope by method, not by route. The middleware is content-
// agnostic: it captures status + headers + body and replays them
// faithfully. Route-scoped opt-in would require either a registry of
// "idempotent" routes (drifts out of sync with the router) or a per-
// handler call (defeats the middleware shape). The header-presence
// gate keeps every route eligible without ever recording a response
// the client did not opt-in to.
func IdempotencyMiddleware(store IdempotencyStore) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get(idempotencyHeader)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			log := observability.Logger(r.Context())

			if existing, ok, err := store.Find(r.Context(), key); err != nil {
				log.WithError(err).WithField("idempotency.key", key).
					Warn("idempotency: find failed; falling through to handler")
			} else if ok {
				replayStoredResponse(w, existing)
				return
			}

			rec := newIdempotencyRecorder(w)
			next.ServeHTTP(rec, r)

			resp := idempotency.StoredResponse{
				StatusCode: rec.statusCode,
				Body:       rec.body.Bytes(),
				Headers:    cloneHeader(rec.capturedHeaders),
			}
			if err := store.Save(r.Context(), key, r.Method, r.URL.Path, resp, idempotencyTTL); err != nil {
				log.WithError(err).WithField("idempotency.key", key).
					Warn("idempotency: save failed; retries will re-execute the handler")
			}
		})
	}
}

// replayStoredResponse writes a previously recorded response back to
// the wire. Headers go first (Write* will commit them on the first
// body byte), then the status, then the body. Empty Body is fine —
// http.ResponseWriter happily accepts a zero-length Write.
func replayStoredResponse(w http.ResponseWriter, resp idempotency.StoredResponse) {
	dst := w.Header()
	for k, vs := range resp.Headers {
		dst[k] = append(dst[k][:0], vs...)
	}
	status := resp.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	if len(resp.Body) > 0 {
		_, _ = w.Write(resp.Body)
	}
}

// idempotencyRecorder is the ResponseWriter wrapper that captures
// what the downstream handler emits so the middleware can both
// (a) forward it to the real wire and (b) snapshot it for Save.
//
// It deliberately mirrors net/http's contract: WriteHeader is called
// at most once by the handler (subsequent calls are dropped by the
// underlying ResponseWriter), Header() returns a live map the
// handler may mutate until the first WriteHeader/Write, and Write
// implicitly defaults the status to 200 if no WriteHeader call has
// happened yet.
type idempotencyRecorder struct {
	http.ResponseWriter
	statusCode      int
	wroteHeader     bool
	capturedHeaders http.Header
	body            bytes.Buffer
}

func newIdempotencyRecorder(w http.ResponseWriter) *idempotencyRecorder {
	return &idempotencyRecorder{
		ResponseWriter:  w,
		statusCode:      http.StatusOK,
		capturedHeaders: http.Header{},
	}
}

func (r *idempotencyRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		// net/http drops duplicate WriteHeader calls; mirror that so
		// the snapshot matches what the client actually receives.
		return
	}
	r.statusCode = status
	// Snapshot the headers the handler set BEFORE WriteHeader. After
	// WriteHeader the underlying response has committed them; capturing
	// here ensures the replayed response has the same header set the
	// original client saw.
	for k, v := range r.Header() {
		// http.Header is map[string][]string; copy the slice so a
		// later handler mutation doesn't poison the snapshot.
		dup := make([]string, len(v))
		copy(dup, v)
		r.capturedHeaders[k] = dup
	}
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *idempotencyRecorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		// net/http auto-calls WriteHeader(200) on the first Write;
		// route that through our wrapper so the snapshot is consistent.
		r.WriteHeader(http.StatusOK)
	}
	r.body.Write(p)
	return r.ResponseWriter.Write(p)
}

// cloneHeader is a small defensive copy used when constructing the
// StoredResponse so the captured map cannot be mutated by anything
// holding a reference to the recorder after the handler returns.
func cloneHeader(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, v := range h {
		dup := make([]string, len(v))
		copy(dup, v)
		out[k] = dup
	}
	return out
}
