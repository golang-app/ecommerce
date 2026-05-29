package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/app"
	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/domain"
	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/integration"
	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
)

// recordingPublisher captures the integration events the service
// emits so the tests can assert both ordering and payload contents.
type recordingPublisher struct {
	events []eventbus.Event
}

func (r *recordingPublisher) Publish(_ context.Context, e eventbus.Event) {
	r.events = append(r.events, e)
}

// recordingStock captures Release() arguments so tests can verify
// the refund flow drives the productcatalog port.
type recordingStock struct {
	released []map[string]int
}

func (r *recordingStock) Release(_ context.Context, q map[string]int) error {
	r.released = append(r.released, q)
	return nil
}

// staticLines is a tiny OrderLineSource: every order maps to the
// same quantity table.
type staticLines struct {
	q map[string]int
}

func (s staticLines) OrderQuantities(_ context.Context, _ string) (map[string]int, error) {
	return s.q, nil
}

// newServiceFixture builds a service backed by the in-memory adapter
// with a deterministic clock and id generator so assertions are
// stable.
func newServiceFixture(t *testing.T) (*app.Service, *adapter.InMemory, *recordingPublisher) {
	t.Helper()
	storage := adapter.NewInMemory()
	pub := &recordingPublisher{}
	srv := app.NewService(storage).
		WithPublisher(pub).
		WithClock(func() time.Time {
			return time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
		}).
		WithIDGenerator(func() string { return "ful-test-1" })
	return srv, storage, pub
}

func TestOnOrderPaid_CreatesScheduledFulfillment(t *testing.T) {
	srv, storage, pub := newServiceFixture(t)
	ctx := context.Background()

	if err := srv.OnOrderPaid(ctx, "ord-1", time.Now()); err != nil {
		t.Fatalf("OnOrderPaid: %v", err)
	}
	f, err := storage.FindByOrder(ctx, "ord-1")
	if err != nil {
		t.Fatalf("FindByOrder: %v", err)
	}
	if f.Status() != domain.StatusScheduled {
		t.Errorf("status = %q, want scheduled", f.Status())
	}
	if f.ID() != "ful-test-1" {
		t.Errorf("id = %q, want ful-test-1", f.ID())
	}
	// FulfillmentScheduled is internal-only and not published.
	if len(pub.events) != 0 {
		t.Errorf("unexpected published events: %v", pub.events)
	}
}

func TestOnOrderPaid_IsIdempotent(t *testing.T) {
	srv, _, _ := newServiceFixture(t)
	ctx := context.Background()

	if err := srv.OnOrderPaid(ctx, "ord-1", time.Now()); err != nil {
		t.Fatalf("first OnOrderPaid: %v", err)
	}
	// A duplicate delivery from the outbox MUST be a no-op.
	if err := srv.OnOrderPaid(ctx, "ord-1", time.Now()); err != nil {
		t.Errorf("second OnOrderPaid (duplicate) returned: %v", err)
	}
}

func TestHappyPath_ScheduleLabelShipDeliver(t *testing.T) {
	srv, _, pub := newServiceFixture(t)
	ctx := context.Background()

	if err := srv.OnOrderPaid(ctx, "ord-1", time.Now()); err != nil {
		t.Fatalf("OnOrderPaid: %v", err)
	}
	if err := srv.Label(ctx, "ord-1", "UPS", "1Z-XYZ"); err != nil {
		t.Fatalf("Label: %v", err)
	}
	if err := srv.Ship(ctx, "ord-1"); err != nil {
		t.Fatalf("Ship: %v", err)
	}
	if err := srv.Deliver(ctx, "ord-1"); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	got := pub.events
	if len(got) != 2 {
		t.Fatalf("published %d events, want 2 (Shipped, Delivered): %v", len(got), got)
	}
	shipped, ok := got[0].(integration.OrderShipped)
	if !ok {
		t.Fatalf("event[0] = %T, want OrderShipped", got[0])
	}
	if shipped.OrderID != "ord-1" || shipped.Carrier != "UPS" || shipped.TrackingCode != "1Z-XYZ" {
		t.Errorf("OrderShipped = %+v, want order ord-1 / UPS / 1Z-XYZ", shipped)
	}
	if _, ok := got[1].(integration.OrderDelivered); !ok {
		t.Errorf("event[1] = %T, want OrderDelivered", got[1])
	}
}

func TestRefund_FromAnyActiveState(t *testing.T) {
	cases := []struct {
		name  string
		prep  func(*testing.T, *app.Service)
	}{
		{
			name: "from scheduled",
			prep: func(t *testing.T, s *app.Service) {},
		},
		{
			name: "from labeled",
			prep: func(t *testing.T, s *app.Service) {
				if err := s.Label(context.Background(), "ord-1", "UPS", "1Z"); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "from shipped",
			prep: func(t *testing.T, s *app.Service) {
				ctx := context.Background()
				if err := s.Label(ctx, "ord-1", "UPS", "1Z"); err != nil {
					t.Fatal(err)
				}
				if err := s.Ship(ctx, "ord-1"); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "from delivered",
			prep: func(t *testing.T, s *app.Service) {
				ctx := context.Background()
				if err := s.Label(ctx, "ord-1", "UPS", "1Z"); err != nil {
					t.Fatal(err)
				}
				if err := s.Ship(ctx, "ord-1"); err != nil {
					t.Fatal(err)
				}
				if err := s.Deliver(ctx, "ord-1"); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, storage, _ := newServiceFixture(t)
			ctx := context.Background()
			if err := srv.OnOrderPaid(ctx, "ord-1", time.Now()); err != nil {
				t.Fatalf("OnOrderPaid: %v", err)
			}
			tc.prep(t, srv)
			if err := srv.Refund(ctx, "ord-1", "test reason"); err != nil {
				t.Fatalf("Refund: %v", err)
			}
			f, err := storage.FindByOrder(ctx, "ord-1")
			if err != nil {
				t.Fatalf("FindByOrder: %v", err)
			}
			if f.Status() != domain.StatusRefunded {
				t.Errorf("status = %q, want refunded", f.Status())
			}
			if f.RefundReason() != "test reason" {
				t.Errorf("reason = %q, want %q", f.RefundReason(), "test reason")
			}
		})
	}
}

func TestRefund_ReleasesStock(t *testing.T) {
	srv, _, _ := newServiceFixture(t)
	stock := &recordingStock{}
	srv.WithStockReleaser(stock).WithOrderLines(staticLines{q: map[string]int{"var-1": 2}})
	ctx := context.Background()

	if err := srv.OnOrderPaid(ctx, "ord-1", time.Now()); err != nil {
		t.Fatalf("OnOrderPaid: %v", err)
	}
	if err := srv.Refund(ctx, "ord-1", "customer return"); err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if len(stock.released) != 1 {
		t.Fatalf("released %d batches, want 1", len(stock.released))
	}
	if stock.released[0]["var-1"] != 2 {
		t.Errorf("released[var-1] = %d, want 2", stock.released[0]["var-1"])
	}
}

func TestRejectedTransitions(t *testing.T) {
	cases := []struct {
		name string
		do   func(*app.Service) error
	}{
		{
			name: "ship before label",
			do: func(s *app.Service) error {
				return s.Ship(context.Background(), "ord-1")
			},
		},
		{
			name: "deliver before ship",
			do: func(s *app.Service) error {
				return s.Deliver(context.Background(), "ord-1")
			},
		},
		{
			name: "label twice",
			do: func(s *app.Service) error {
				ctx := context.Background()
				if err := s.Label(ctx, "ord-1", "UPS", "1Z"); err != nil {
					return err
				}
				return s.Label(ctx, "ord-1", "FedEx", "FX")
			},
		},
		{
			name: "deliver after delivered",
			do: func(s *app.Service) error {
				ctx := context.Background()
				if err := s.Label(ctx, "ord-1", "UPS", "1Z"); err != nil {
					return err
				}
				if err := s.Ship(ctx, "ord-1"); err != nil {
					return err
				}
				if err := s.Deliver(ctx, "ord-1"); err != nil {
					return err
				}
				return s.Deliver(ctx, "ord-1")
			},
		},
		{
			name: "refund twice",
			do: func(s *app.Service) error {
				ctx := context.Background()
				if err := s.Refund(ctx, "ord-1", "first"); err != nil {
					return err
				}
				return s.Refund(ctx, "ord-1", "second")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _, _ := newServiceFixture(t)
			if err := srv.OnOrderPaid(context.Background(), "ord-1", time.Now()); err != nil {
				t.Fatalf("OnOrderPaid: %v", err)
			}
			err := tc.do(srv)
			if !errors.Is(err, domain.ErrInvalidTransition) {
				t.Errorf("got %v, want ErrInvalidTransition", err)
			}
		})
	}
}

func TestByOrder_NotFound(t *testing.T) {
	srv, _, _ := newServiceFixture(t)
	_, err := srv.ByOrder(context.Background(), "missing")
	if !errors.Is(err, app.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}
