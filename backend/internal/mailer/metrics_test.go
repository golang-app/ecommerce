package mailer

import (
	"context"
	"errors"
	"testing"
)

// metricObs is one captured emails_sent_total observation.
type metricObs struct {
	kind, outcome string
}

func TestMetricsMailerEmitsSuccessOutcome(t *testing.T) {
	var got []metricObs
	inner := &fakeMailer{}
	m := newMetricsWithEmitter(inner, func(_ context.Context, kind, outcome string) {
		got = append(got, metricObs{kind, outcome})
	})

	err := m.Send(context.Background(), Message{To: "a@b", Kind: KindOrderConfirmation})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("inner should have been called once, got %d", inner.calls)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 metric observation, got %d (%v)", len(got), got)
	}
	if got[0] != (metricObs{KindOrderConfirmation, "success"}) {
		t.Fatalf("unexpected metric: %+v", got[0])
	}
}

func TestMetricsMailerEmitsFailureOutcome(t *testing.T) {
	var got []metricObs
	boom := errors.New("smtp 550")
	inner := &fakeMailer{errs: []error{boom}}
	m := newMetricsWithEmitter(inner, func(_ context.Context, kind, outcome string) {
		got = append(got, metricObs{kind, outcome})
	})

	err := m.Send(context.Background(), Message{To: "a@b", Kind: KindPasswordReset})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom propagated, got %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 metric observation, got %d", len(got))
	}
	if got[0] != (metricObs{KindPasswordReset, "failure"}) {
		t.Fatalf("unexpected metric: %+v", got[0])
	}
}

func TestMetricsMailerDefaultsBlankKindToUnknown(t *testing.T) {
	var got []metricObs
	inner := &fakeMailer{}
	m := newMetricsWithEmitter(inner, func(_ context.Context, kind, outcome string) {
		got = append(got, metricObs{kind, outcome})
	})

	if err := m.Send(context.Background(), Message{To: "a@b"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].kind != KindUnknown {
		t.Fatalf("expected kind=%q, got %+v", KindUnknown, got)
	}
}
