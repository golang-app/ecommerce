package outbox

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

// testEvent is a tiny eventbus.Event used only by the dispatcher unit
// tests so they stay independent of any bounded context.
type testEvent struct {
	Name string
	ID   string
}

func (e testEvent) EventName() string { return e.Name }

type fakeStore struct {
	mu       sync.Mutex
	rows     []Row
	sent     []int64
	markErrs map[int64]error
	unsentEr error
}

func (s *fakeStore) Unsent(_ context.Context, _ int) ([]Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unsentEr != nil {
		return nil, s.unsentEr
	}
	out := make([]Row, 0, len(s.rows))
	out = append(out, s.rows...)
	return out, nil
}

func (s *fakeStore) MarkSent(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err, ok := s.markErrs[id]; ok {
		return err
	}
	s.sent = append(s.sent, id)
	// Remove the row from the queue so a subsequent Unsent does not
	// re-return it.
	kept := s.rows[:0]
	for _, r := range s.rows {
		if r.ID != id {
			kept = append(kept, r)
		}
	}
	s.rows = kept
	return nil
}

type fakeBus struct {
	mu        sync.Mutex
	published []eventbus.Event
}

func (b *fakeBus) Publish(_ context.Context, e eventbus.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, e)
}

func discardLogger() logrus.FieldLogger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}

func decodeTest(kind string, payload []byte) (eventbus.Event, error) {
	return testEvent{Name: kind, ID: string(payload)}, nil
}

func TestDispatchOnce_PublishesAndMarksSent(t *testing.T) {
	is := is.New(t)
	store := &fakeStore{
		rows: []Row{
			{ID: 1, Kind: "test.A", Payload: []byte("one"), CreatedAt: time.Now()},
			{ID: 2, Kind: "test.A", Payload: []byte("two"), CreatedAt: time.Now()},
		},
	}
	bus := &fakeBus{}
	d := NewDispatcher(store, bus, decodeTest, discardLogger(), time.Second)

	d.dispatchOnce(context.Background())

	is.Equal(len(bus.published), 2)
	is.Equal(bus.published[0].(testEvent).ID, "one")
	is.Equal(bus.published[1].(testEvent).ID, "two")
	is.Equal(store.sent, []int64{1, 2})
	is.Equal(len(store.rows), 0)
}

func TestDispatchOnce_MarkSentFailure_RowReplayedNextTick(t *testing.T) {
	is := is.New(t)
	store := &fakeStore{
		rows: []Row{
			{ID: 7, Kind: "test.A", Payload: []byte("retry-me"), CreatedAt: time.Now()},
		},
		markErrs: map[int64]error{7: errors.New("db transient")},
	}
	bus := &fakeBus{}
	d := NewDispatcher(store, bus, decodeTest, discardLogger(), time.Second)

	// First tick: publish succeeds but MarkSent fails — row stays unsent.
	d.dispatchOnce(context.Background())
	is.Equal(len(bus.published), 1)
	is.Equal(len(store.sent), 0)
	is.Equal(len(store.rows), 1) // row still queued

	// Second tick: clear the transient failure; row gets republished
	// (at-least-once contract) and finally marked sent.
	store.markErrs = nil
	d.dispatchOnce(context.Background())
	is.Equal(len(bus.published), 2) // re-published
	is.Equal(store.sent, []int64{7})
	is.Equal(len(store.rows), 0)
}

func TestDispatchOnce_DecodeError_RowSkippedNotMarked(t *testing.T) {
	is := is.New(t)
	store := &fakeStore{
		rows: []Row{
			{ID: 11, Kind: "unknown", Payload: []byte("x"), CreatedAt: time.Now()},
			{ID: 12, Kind: "test.A", Payload: []byte("ok"), CreatedAt: time.Now()},
		},
	}
	bus := &fakeBus{}
	decode := func(kind string, payload []byte) (eventbus.Event, error) {
		if kind == "unknown" {
			return nil, errors.New("no decoder")
		}
		return testEvent{Name: kind, ID: string(payload)}, nil
	}
	d := NewDispatcher(store, bus, decode, discardLogger(), time.Second)

	d.dispatchOnce(context.Background())

	is.Equal(len(bus.published), 1)
	is.Equal(bus.published[0].(testEvent).ID, "ok")
	is.Equal(store.sent, []int64{12})
	// The broken row is still in the queue (not marked sent), exactly
	// what we want: a code fix later can drain it.
	is.Equal(len(store.rows), 1)
	is.Equal(store.rows[0].ID, int64(11))
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	store := &fakeStore{}
	bus := &fakeBus{}
	d := NewDispatcher(store, bus, decodeTest, discardLogger(), 20*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Run(ctx)
		close(done)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

func TestRun_DisabledOnNonPositiveInterval(t *testing.T) {
	is := is.New(t)
	store := &fakeStore{rows: []Row{{ID: 1, Kind: "test.A", Payload: []byte("x")}}}
	bus := &fakeBus{}
	d := NewDispatcher(store, bus, decodeTest, discardLogger(), 0)

	// Run returns immediately when disabled — no goroutine, no ticker.
	d.Run(context.Background())

	is.Equal(len(bus.published), 0)
	is.Equal(len(store.sent), 0)
}
