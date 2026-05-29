package inbox

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
	"github.com/matryer/is"
)

// testEvent is a tiny eventbus.Event used to keep the inbox tests
// independent of any bounded context's published language.
type testEvent struct{ name string }

func (e testEvent) EventName() string {
	if e.name == "" {
		return "test.Event"
	}
	return e.name
}

// fakeStore is an in-memory MarkHandler implementation. It stays in
// the same package as inbox.Wrap so the test does not need to widen
// any interface, and it lets us assert calls without a real DB.
type fakeStore struct {
	mu        sync.Mutex
	seen      map[string]struct{} // "subscriber|eventID"
	failNext  error               // returned once, then cleared
	callCount int
}

func newFakeStore() *fakeStore { return &fakeStore{seen: map[string]struct{}{}} }

func (s *fakeStore) MarkHandled(_ context.Context, subscriber string, eventID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callCount++
	if s.failNext != nil {
		err := s.failNext
		s.failNext = nil
		return false, err
	}
	key := keyOf(subscriber, eventID)
	if _, dup := s.seen[key]; dup {
		return true, nil
	}
	s.seen[key] = struct{}{}
	return false, nil
}

func keyOf(subscriber string, eventID int64) string {
	// A tiny inline join keeps the fake dependency-free.
	return subscriber + "|" + itoa(eventID)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func TestWrap_FirstDelivery_HandlerCalled(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	var called int
	wrapped := Wrap("sub", store, func(_ context.Context, _ int64, _ eventbus.Event) error {
		called++
		return nil
	})

	err := wrapped(context.Background(), 42, testEvent{})

	is.NoErr(err)
	is.Equal(called, 1)
	is.Equal(store.callCount, 1)
}

func TestWrap_DuplicateDelivery_HandlerSkipped(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	var called int
	wrapped := Wrap("sub", store, func(_ context.Context, _ int64, _ eventbus.Event) error {
		called++
		return nil
	})

	is.NoErr(wrapped(context.Background(), 42, testEvent{}))
	is.NoErr(wrapped(context.Background(), 42, testEvent{}))

	is.Equal(called, 1)          // handler ran once, duplicate skipped at the wrapper
	is.Equal(store.callCount, 2) // MarkHandled still consulted both times
}

func TestWrap_DifferentEventID_HandlerCalledAgain(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	var called int
	wrapped := Wrap("sub", store, func(_ context.Context, _ int64, _ eventbus.Event) error {
		called++
		return nil
	})

	is.NoErr(wrapped(context.Background(), 1, testEvent{}))
	is.NoErr(wrapped(context.Background(), 2, testEvent{}))

	is.Equal(called, 2)
	is.Equal(store.callCount, 2)
}

func TestWrap_ZeroEventID_PassesThrough(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	var called int
	wrapped := Wrap("sub", store, func(_ context.Context, _ int64, _ eventbus.Event) error {
		called++
		return nil
	})

	// Two zero-id deliveries: both must reach the handler — eventID==0
	// is the "no dedupe id available" sentinel.
	is.NoErr(wrapped(context.Background(), 0, testEvent{}))
	is.NoErr(wrapped(context.Background(), 0, testEvent{}))

	is.Equal(called, 2)
	is.Equal(store.callCount, 0) // store is bypassed entirely
}

func TestWrap_StorageError_HandlerNotCalled(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	store.failNext = errors.New("db transient")
	var called int
	wrapped := Wrap("sub", store, func(_ context.Context, _ int64, _ eventbus.Event) error {
		called++
		return nil
	})

	err := wrapped(context.Background(), 42, testEvent{})

	is.True(err != nil)              // storage error surfaces to the caller
	is.Equal(called, 0)              // handler MUST NOT run on storage failure
	is.Equal(store.callCount, 1)

	// Retry semantics: the next tick from the Outbox dispatcher
	// presents the same row again; the store has cleared its
	// failure and the handler now runs exactly once.
	is.NoErr(wrapped(context.Background(), 42, testEvent{}))
	is.Equal(called, 1)
	is.Equal(store.callCount, 2)
}

func TestWrap_DifferentSubscribers_DedupeIsScopedToSubscriber(t *testing.T) {
	is := is.New(t)
	store := newFakeStore()
	var aCalls, bCalls int
	wrapA := Wrap("sub.a", store, func(_ context.Context, _ int64, _ eventbus.Event) error {
		aCalls++
		return nil
	})
	wrapB := Wrap("sub.b", store, func(_ context.Context, _ int64, _ eventbus.Event) error {
		bCalls++
		return nil
	})

	// Both subscribers see the same originating event id. Each runs
	// once — dedupe is per-(subscriber, event_id), exactly the table
	// shape says.
	is.NoErr(wrapA(context.Background(), 7, testEvent{}))
	is.NoErr(wrapB(context.Background(), 7, testEvent{}))

	is.Equal(aCalls, 1)
	is.Equal(bCalls, 1)
}
