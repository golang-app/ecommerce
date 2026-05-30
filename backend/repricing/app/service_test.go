package app_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/repricing/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/repricing/app"
	"github.com/bkielbasa/go-ecommerce/backend/repricing/domain"
)

// staticReader is a tiny CategoryReader: every slug maps to the
// same product id slice.
type staticReader struct {
	ids []string
	err error
}

func (s staticReader) ProductsInCategory(_ context.Context, _ string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]string, len(s.ids))
	copy(out, s.ids)
	return out, nil
}

// fakeUpdater is the test PriceUpdater: it stores per-product
// minor-unit amounts in a map keyed by product id, allows tests to
// inject per-product errors, and records every call for assertion.
type fakeUpdater struct {
	mu       sync.Mutex
	prices   map[string]int64
	errs     map[string]error
	loadErrs map[string]error
	updates  []update
}

type update struct {
	productID string
	amount    int64
}

func newFakeUpdater(initial map[string]int64) *fakeUpdater {
	cp := map[string]int64{}
	for k, v := range initial {
		cp[k] = v
	}
	return &fakeUpdater{
		prices:   cp,
		errs:     map[string]error{},
		loadErrs: map[string]error{},
	}
}

func (f *fakeUpdater) CurrentPriceMinor(_ context.Context, productID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.loadErrs[productID]; ok {
		return 0, err
	}
	v, ok := f.prices[productID]
	if !ok {
		return 0, fmt.Errorf("unknown product %q", productID)
	}
	return v, nil
}

func (f *fakeUpdater) UpdateProductPrice(_ context.Context, productID string, newAmount int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.errs[productID]; ok {
		return err
	}
	f.prices[productID] = newAmount
	f.updates = append(f.updates, update{productID: productID, amount: newAmount})
	return nil
}

func (f *fakeUpdater) priceOf(productID string) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.prices[productID]
}

func (f *fakeUpdater) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.updates)
}

// newServiceFixture wires a service backed by the in-memory storage
// with a deterministic clock and id generator and a SYNCHRONOUS
// async runner — Start blocks until RunActive completes so tests
// can assert without polling.
func newServiceFixture(t *testing.T, ids []string, prices map[string]int64) (*app.Service, *adapter.InMemory, *fakeUpdater) {
	t.Helper()
	storage := adapter.NewInMemory()
	upd := newFakeUpdater(prices)
	srv := app.NewService(storage, staticReader{ids: ids}, upd).
		WithClock(func() time.Time {
			return time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
		}).
		WithIDGenerator(func() string { return "rep-test-1" }).
		// Synchronous runner: Start invokes RunActive inline.
		WithAsyncRunner(func(fn func()) { fn() })
	return srv, storage, upd
}

func TestStart_RunsToCompletion(t *testing.T) {
	ids := []string{"p-1", "p-2", "p-3"}
	prices := map[string]int64{
		"p-1": 1000, // -> 1100 after +10%
		"p-2": 2000, // -> 2200
		"p-3": 3000, // -> 3300
	}
	srv, storage, upd := newServiceFixture(t, ids, prices)
	ctx := context.Background()

	id, err := srv.Start(ctx, "tools", 10)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if id != "rep-test-1" {
		t.Fatalf("id = %q, want rep-test-1", id)
	}

	r, err := storage.Find(ctx, id)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if r.Status() != domain.StatusCompleted {
		t.Fatalf("status = %q, want completed", r.Status())
	}
	if r.ProcessedItems() != 3 {
		t.Fatalf("processed = %d, want 3", r.ProcessedItems())
	}
	if upd.callCount() != 3 {
		t.Fatalf("updates = %d, want 3", upd.callCount())
	}
	if got := upd.priceOf("p-1"); got != 1100 {
		t.Errorf("p-1 = %d, want 1100", got)
	}
	if got := upd.priceOf("p-2"); got != 2200 {
		t.Errorf("p-2 = %d, want 2200", got)
	}
	if got := upd.priceOf("p-3"); got != 3300 {
		t.Errorf("p-3 = %d, want 3300", got)
	}
}

func TestStart_RejectsWhenActiveExists(t *testing.T) {
	ids := []string{"p-1"}
	prices := map[string]int64{"p-1": 100}
	storage := adapter.NewInMemory()
	upd := newFakeUpdater(prices)

	// First create an active row by hand.
	r, err := domain.NewReprice("rep-existing", "cat", 10, 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("NewReprice: %v", err)
	}
	if err := storage.Create(context.Background(), r); err != nil {
		t.Fatalf("Create: %v", err)
	}

	srv := app.NewService(storage, staticReader{ids: ids}, upd).
		WithAsyncRunner(func(fn func()) { fn() })

	if _, err := srv.Start(context.Background(), "tools", 5); !errors.Is(err, app.ErrAlreadyActive) {
		t.Fatalf("got %v, want ErrAlreadyActive", err)
	}
}

func TestStart_RejectsEmptyCategory(t *testing.T) {
	srv, _, _ := newServiceFixture(t, nil, nil)
	_, err := srv.Start(context.Background(), "tools", 10)
	if err == nil {
		t.Fatal("expected error for empty category, got nil")
	}
}

func TestStart_RejectsInvalidPercent(t *testing.T) {
	srv, _, _ := newServiceFixture(t, []string{"p-1"}, map[string]int64{"p-1": 100})
	_, err := srv.Start(context.Background(), "tools", 0)
	if !errors.Is(err, domain.ErrInvalidReprice) {
		t.Fatalf("got %v, want ErrInvalidReprice", err)
	}
}

func TestRunActive_NoActive(t *testing.T) {
	srv, _, _ := newServiceFixture(t, nil, nil)
	if err := srv.RunActive(context.Background()); err != nil {
		t.Fatalf("RunActive: %v", err)
	}
}

func TestRunActive_ResumesFromProcessed(t *testing.T) {
	// Manually craft a scenario: an in_progress row with 1 of 3
	// items already processed (modelling a crash mid-saga).
	storage := adapter.NewInMemory()
	upd := newFakeUpdater(map[string]int64{
		"p-1": 1000,
		"p-2": 2000,
		"p-3": 3000,
	})

	at := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	r, err := domain.NewReprice("rep-resume", "tools", 10, 3, at)
	if err != nil {
		t.Fatalf("NewReprice: %v", err)
	}
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Pretend p-1 was already done at a previous price (so its
	// stored price is the *new* one, 1100). We mutate the
	// fakeUpdater's table to reflect that.
	upd.mu.Lock()
	upd.prices["p-1"] = 1100
	upd.mu.Unlock()
	if err := r.RecordItemProcessed(at); err != nil {
		t.Fatalf("RecordItemProcessed: %v", err)
	}
	if err := storage.Create(context.Background(), r); err != nil {
		t.Fatalf("Create: %v", err)
	}

	srv := app.NewService(storage, staticReader{ids: []string{"p-1", "p-2", "p-3"}}, upd).
		WithClock(func() time.Time { return at }).
		WithAsyncRunner(func(fn func()) { fn() })

	if err := srv.RunActive(context.Background()); err != nil {
		t.Fatalf("RunActive: %v", err)
	}

	got, err := storage.Find(context.Background(), "rep-resume")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.Status() != domain.StatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status())
	}
	if got.ProcessedItems() != 3 {
		t.Fatalf("processed = %d, want 3", got.ProcessedItems())
	}
	// p-1 must NOT have been re-priced (1100 -> would have
	// become 1210 if the resume failed to skip).
	if v := upd.priceOf("p-1"); v != 1100 {
		t.Errorf("p-1 = %d, want 1100 (no double-apply)", v)
	}
	if v := upd.priceOf("p-2"); v != 2200 {
		t.Errorf("p-2 = %d, want 2200", v)
	}
	if v := upd.priceOf("p-3"); v != 3300 {
		t.Errorf("p-3 = %d, want 3300", v)
	}
}

func TestRunActive_PerItemErrorIsRecordedAndContinues(t *testing.T) {
	ids := []string{"p-1", "p-2", "p-3"}
	prices := map[string]int64{"p-1": 1000, "p-2": 2000, "p-3": 3000}
	storage := adapter.NewInMemory()
	upd := newFakeUpdater(prices)
	upd.errs["p-2"] = errors.New("price rejected")

	srv := app.NewService(storage, staticReader{ids: ids}, upd).
		WithClock(func() time.Time { return time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC) }).
		WithIDGenerator(func() string { return "rep-err" }).
		WithAsyncRunner(func(fn func()) { fn() })

	id, err := srv.Start(context.Background(), "tools", 10)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	r, err := storage.Find(context.Background(), id)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if r.Status() != domain.StatusCompleted {
		t.Fatalf("status = %q, want completed (per-item error must not abort)", r.Status())
	}
	if r.ProcessedItems() != 3 {
		t.Fatalf("processed = %d, want 3", r.ProcessedItems())
	}
	if r.LastError() == "" {
		t.Error("lastError should record the per-item failure")
	}
	if v := upd.priceOf("p-2"); v != 2000 {
		t.Errorf("p-2 = %d, want 2000 (untouched)", v)
	}
}

func TestStartTwice_RejectedInline(t *testing.T) {
	// Use a runner that captures but does NOT invoke the
	// saga, simulating a real background driver. Then the
	// second Start must reject because the first row is still
	// in StatusScheduled.
	ids := []string{"p-1"}
	prices := map[string]int64{"p-1": 1000}
	storage := adapter.NewInMemory()
	upd := newFakeUpdater(prices)

	srv := app.NewService(storage, staticReader{ids: ids}, upd).
		WithIDGenerator(func() string { return "rep-a" }).
		WithAsyncRunner(func(_ func()) {})

	ctx := context.Background()
	if _, err := srv.Start(ctx, "tools", 10); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if _, err := srv.Start(ctx, "tools", 10); !errors.Is(err, app.ErrAlreadyActive) {
		t.Fatalf("second Start: got %v, want ErrAlreadyActive", err)
	}
}

func TestActive_ReturnsCurrent(t *testing.T) {
	srv, _, _ := newServiceFixture(t, []string{"p-1"}, map[string]int64{"p-1": 100})
	ctx := context.Background()

	if _, ok, err := srv.Active(ctx); err != nil || ok {
		t.Fatalf("expected no active, got ok=%v err=%v", ok, err)
	}
	_, err := srv.Start(ctx, "tools", 10)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Synchronous runner completes the saga so Active is now empty.
	if _, ok, err := srv.Active(ctx); err != nil || ok {
		t.Fatalf("expected no active after completion, got ok=%v err=%v", ok, err)
	}
}

func TestListAll_NewestFirst(t *testing.T) {
	// Drive two reprices to completion sequentially.
	storage := adapter.NewInMemory()
	upd := newFakeUpdater(map[string]int64{"p-1": 1000})
	clock := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	idCounter := 0
	srv := app.NewService(storage, staticReader{ids: []string{"p-1"}}, upd).
		WithClock(func() time.Time { return clock }).
		WithIDGenerator(func() string {
			idCounter++
			return fmt.Sprintf("rep-%d", idCounter)
		}).
		WithAsyncRunner(func(fn func()) { fn() })

	ctx := context.Background()
	if _, err := srv.Start(ctx, "tools", 10); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	clock = clock.Add(time.Hour)
	if _, err := srv.Start(ctx, "tools", -5); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	rows, err := srv.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].ID() != "rep-2" {
		t.Errorf("newest-first: got %q", rows[0].ID())
	}
}
