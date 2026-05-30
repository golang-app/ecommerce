package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/repricing/domain"
)

func mustNewReprice(t *testing.T) domain.Reprice {
	t.Helper()
	r, err := domain.NewReprice("rep-1", "tools", 10, 3, time.Now().UTC())
	if err != nil {
		t.Fatalf("NewReprice: %v", err)
	}
	return r
}

func TestNewReprice_Validates(t *testing.T) {
	at := time.Now().UTC()
	cases := []struct {
		name           string
		id             string
		categoryID     string
		percent        float64
		total          int
		wantInvalidArg bool
	}{
		{"empty id", "", "tools", 10, 5, true},
		{"empty categoryID", "rep-1", "", 10, 5, true},
		{"zero items", "rep-1", "tools", 10, 0, true},
		{"negative items", "rep-1", "tools", 10, -1, true},
		{"percent zero", "rep-1", "tools", 0, 5, true},
		{"percent too high", "rep-1", "tools", 51, 5, true},
		{"percent too low", "rep-1", "tools", -51, 5, true},
		{"happy path", "rep-1", "tools", 10, 5, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := domain.NewReprice(c.id, c.categoryID, c.percent, c.total, at)
			if c.wantInvalidArg {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				if !errors.Is(err, domain.ErrInvalidReprice) {
					t.Fatalf("error %q does not wrap ErrInvalidReprice", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewReprice_InitialState(t *testing.T) {
	r := mustNewReprice(t)
	if r.Status() != domain.StatusScheduled {
		t.Errorf("status = %q, want scheduled", r.Status())
	}
	if r.ProcessedItems() != 0 {
		t.Errorf("processed = %d, want 0", r.ProcessedItems())
	}
	if r.Version() != 1 {
		t.Errorf("version = %d, want 1", r.Version())
	}
	if r.ProgressFraction() != 0 {
		t.Errorf("progress = %v, want 0", r.ProgressFraction())
	}
	if r.CompletedAt() != nil {
		t.Errorf("completedAt = %v, want nil", r.CompletedAt())
	}
}

func TestStart_TransitionsToInProgress(t *testing.T) {
	r := mustNewReprice(t)
	if err := r.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if r.Status() != domain.StatusInProgress {
		t.Errorf("status = %q, want in_progress", r.Status())
	}
	if r.Version() != 2 {
		t.Errorf("version = %d, want 2", r.Version())
	}
}

func TestStart_RejectsDoubleStart(t *testing.T) {
	r := mustNewReprice(t)
	_ = r.Start()
	err := r.Start()
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("got %v, want ErrInvalidTransition", err)
	}
}

func TestRecordItemProcessed_RequiresInProgress(t *testing.T) {
	r := mustNewReprice(t)
	err := r.RecordItemProcessed(time.Now())
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("got %v, want ErrInvalidTransition", err)
	}
}

func TestRecordItemProcessed_AdvancesCounter(t *testing.T) {
	r := mustNewReprice(t)
	_ = r.Start()
	now := time.Now().UTC()
	if err := r.RecordItemProcessed(now); err != nil {
		t.Fatalf("RecordItemProcessed: %v", err)
	}
	if r.ProcessedItems() != 1 {
		t.Errorf("processed = %d, want 1", r.ProcessedItems())
	}
	wantFrac := 1.0 / 3.0
	if got := r.ProgressFraction(); got != wantFrac {
		t.Errorf("progress = %v, want %v", got, wantFrac)
	}
}

func TestRecordItemFailed_AdvancesAndRecordsError(t *testing.T) {
	r := mustNewReprice(t)
	_ = r.Start()
	if err := r.RecordItemFailed("boom", time.Now().UTC()); err != nil {
		t.Fatalf("RecordItemFailed: %v", err)
	}
	if r.ProcessedItems() != 1 {
		t.Errorf("processed = %d, want 1", r.ProcessedItems())
	}
	if r.LastError() != "boom" {
		t.Errorf("lastError = %q, want boom", r.LastError())
	}
}

func TestRecordItemFailed_RequiresInProgress(t *testing.T) {
	r := mustNewReprice(t)
	err := r.RecordItemFailed("boom", time.Now())
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("got %v, want ErrInvalidTransition", err)
	}
}

func TestComplete_RequiresAllProcessed(t *testing.T) {
	r := mustNewReprice(t)
	_ = r.Start()
	// only 1 of 3 processed
	_ = r.RecordItemProcessed(time.Now())
	err := r.Complete(time.Now())
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("got %v, want ErrInvalidTransition", err)
	}
}

func TestComplete_HappyPath(t *testing.T) {
	r := mustNewReprice(t)
	_ = r.Start()
	for i := 0; i < 3; i++ {
		_ = r.RecordItemProcessed(time.Now())
	}
	at := time.Now().UTC()
	if err := r.Complete(at); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if r.Status() != domain.StatusCompleted {
		t.Errorf("status = %q, want completed", r.Status())
	}
	if r.CompletedAt() == nil || !r.CompletedAt().Equal(at) {
		t.Errorf("completedAt = %v, want %v", r.CompletedAt(), at)
	}
}

func TestComplete_RejectsFromScheduled(t *testing.T) {
	r := mustNewReprice(t)
	err := r.Complete(time.Now())
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("got %v, want ErrInvalidTransition", err)
	}
}

func TestFail_FromScheduled(t *testing.T) {
	r := mustNewReprice(t)
	at := time.Now().UTC()
	if err := r.Fail("nope", at); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if r.Status() != domain.StatusFailed {
		t.Errorf("status = %q, want failed", r.Status())
	}
	if r.LastError() != "nope" {
		t.Errorf("lastError = %q, want nope", r.LastError())
	}
	if r.CompletedAt() == nil {
		t.Errorf("completedAt is nil")
	}
}

func TestFail_FromInProgress(t *testing.T) {
	r := mustNewReprice(t)
	_ = r.Start()
	if err := r.Fail("nope", time.Now()); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if r.Status() != domain.StatusFailed {
		t.Errorf("status = %q, want failed", r.Status())
	}
}

func TestFail_RejectsFromCompleted(t *testing.T) {
	r := mustNewReprice(t)
	_ = r.Start()
	for i := 0; i < 3; i++ {
		_ = r.RecordItemProcessed(time.Now())
	}
	_ = r.Complete(time.Now())
	err := r.Fail("nope", time.Now())
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("got %v, want ErrInvalidTransition", err)
	}
}

func TestRebuild_PreservesFields(t *testing.T) {
	at := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	completed := at.Add(time.Hour)
	r := domain.Rebuild("rep-x", "cat", -25, domain.StatusCompleted, 5, 5, "", at, &completed, 12)
	if r.ID() != "rep-x" {
		t.Errorf("id mismatch")
	}
	if r.Status() != domain.StatusCompleted {
		t.Errorf("status mismatch")
	}
	if r.Version() != 12 {
		t.Errorf("version mismatch")
	}
	if r.PercentChange() != -25 {
		t.Errorf("percent mismatch")
	}
}
