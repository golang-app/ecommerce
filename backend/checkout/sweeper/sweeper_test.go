package sweeper

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

type fakeQueries struct {
	ids        []string
	err        error
	lastCutoff time.Time
	calls      int
}

func (f *fakeQueries) ListExpiredPending(_ context.Context, olderThan time.Time) ([]string, error) {
	f.calls++
	f.lastCutoff = olderThan
	if f.err != nil {
		return nil, f.err
	}
	return f.ids, nil
}

type fakeCommand struct {
	expired []string
	errs    map[string]error
}

func (f *fakeCommand) ExpirePending(_ context.Context, orderID string) error {
	if err, ok := f.errs[orderID]; ok {
		return err
	}
	f.expired = append(f.expired, orderID)
	return nil
}

func discardLogger() logrus.FieldLogger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}

func TestSweep_ExpiresEveryReturnedID(t *testing.T) {
	is := is.New(t)
	q := &fakeQueries{ids: []string{"ord-1", "ord-2", "ord-3"}}
	c := &fakeCommand{}

	s := newSweeper(q, c, 30*time.Minute, 5*time.Minute, discardLogger())
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	s.sweep(context.Background(), now)

	is.Equal(c.expired, []string{"ord-1", "ord-2", "ord-3"})
	is.Equal(q.lastCutoff, now.Add(-30*time.Minute))
	is.Equal(q.calls, 1)
}

func TestSweep_NoExpired_NoCommands(t *testing.T) {
	is := is.New(t)
	q := &fakeQueries{ids: nil}
	c := &fakeCommand{}

	s := newSweeper(q, c, 30*time.Minute, 5*time.Minute, discardLogger())
	s.sweep(context.Background(), time.Now())

	is.Equal(len(c.expired), 0)
}

func TestSweep_ListErrorDoesNotPanicAndSkipsExpiry(t *testing.T) {
	is := is.New(t)
	q := &fakeQueries{err: errors.New("db down")}
	c := &fakeCommand{}

	s := newSweeper(q, c, 30*time.Minute, 5*time.Minute, discardLogger())
	s.sweep(context.Background(), time.Now())

	is.Equal(len(c.expired), 0)
}

func TestSweep_ContinuesOnPerOrderError(t *testing.T) {
	is := is.New(t)
	q := &fakeQueries{ids: []string{"ord-1", "ord-2", "ord-3"}}
	c := &fakeCommand{errs: map[string]error{"ord-2": errors.New("boom")}}

	s := newSweeper(q, c, 30*time.Minute, 5*time.Minute, discardLogger())
	s.sweep(context.Background(), time.Now())

	is.Equal(c.expired, []string{"ord-1", "ord-3"})
}

func TestSweep_StopsWhenContextCancelled(t *testing.T) {
	is := is.New(t)
	q := &fakeQueries{ids: []string{"ord-1", "ord-2"}}
	c := &fakeCommand{}

	s := newSweeper(q, c, 30*time.Minute, 5*time.Minute, discardLogger())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.sweep(ctx, time.Now())

	is.Equal(len(c.expired), 0)
}

func TestRun_DisabledOnNonPositiveInterval(t *testing.T) {
	is := is.New(t)
	q := &fakeQueries{ids: []string{"ord-1"}}
	c := &fakeCommand{}

	s := newSweeper(q, c, 30*time.Minute, 0, discardLogger())
	// Run returns immediately when disabled — no goroutine, no ticker, no panic.
	s.Run(context.Background())

	is.Equal(q.calls, 0)
	is.Equal(len(c.expired), 0)
}

func TestRun_DisabledOnNonPositiveTTL(t *testing.T) {
	is := is.New(t)
	q := &fakeQueries{ids: []string{"ord-1"}}
	c := &fakeCommand{}

	s := newSweeper(q, c, 0, time.Minute, discardLogger())
	s.Run(context.Background())

	is.Equal(q.calls, 0)
	is.Equal(len(c.expired), 0)
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	q := &fakeQueries{}
	c := &fakeCommand{}

	s := newSweeper(q, c, time.Minute, 50*time.Millisecond, discardLogger())
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}
