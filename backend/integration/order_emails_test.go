package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	checkoutdomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutintegration "github.com/bkielbasa/go-ecommerce/backend/checkout/integration"
	checkoutquery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	fulfillmentintegration "github.com/bkielbasa/go-ecommerce/backend/fulfillment/integration"
	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
	"github.com/bkielbasa/go-ecommerce/backend/internal/inbox"
	"github.com/bkielbasa/go-ecommerce/backend/internal/mailer"
	"github.com/bkielbasa/go-ecommerce/backend/layout"
)

// decodeAllOutbox mirrors the decode function in cmd/web/main.go.
func decodeAllOutbox(kind string, payload []byte) (eventbus.Event, error) {
	switch kind {
	case checkoutintegration.OrderPaid{}.EventName():
		var e checkoutintegration.OrderPaid
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("decode OrderPaid: %w", err)
		}
		return e, nil
	case checkoutintegration.OrderPlaced{}.EventName():
		var e checkoutintegration.OrderPlaced
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("decode OrderPlaced: %w", err)
		}
		return e, nil
	case checkoutintegration.OrderCancelled{}.EventName():
		var e checkoutintegration.OrderCancelled
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("decode OrderCancelled: %w", err)
		}
		return e, nil
	case checkoutintegration.OrderPaymentFailed{}.EventName():
		var e checkoutintegration.OrderPaymentFailed
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("decode OrderPaymentFailed: %w", err)
		}
		return e, nil
	}
	return nil, fmt.Errorf("unknown outbox kind: %s", kind)
}

func wireEmailLifecycleSubscribers(
	bus *eventbus.Bus,
	ib *inMemoryInbox,
	query checkoutquery.Service,
	rec *recordingMailer,
	baseURL string,
) {
	// email.order-placed
	bus.SubscribeWithID(
		checkoutintegration.OrderPlaced{}.EventName(),
		inbox.Wrap("email.order-placed", ib,
			func(ctx context.Context, _ int64, e eventbus.Event) error {
				placed := e.(checkoutintegration.OrderPlaced)
				if placed.CustomerID == "" {
					return nil
				}
				view, err := query.Find(ctx, placed.OrderID)
				if err != nil {
					return fmt.Errorf("order placed: load view: %w", err)
				}
				msg, err := layout.RenderOrderPlaced(view, baseURL)
				if err != nil {
					return fmt.Errorf("order placed: render: %w", err)
				}
				msg.To = placed.CustomerID
				return rec.Send(ctx, msg)
			},
		),
	)

	// email.order-cancelled
	bus.SubscribeWithID(
		checkoutintegration.OrderCancelled{}.EventName(),
		inbox.Wrap("email.order-cancelled", ib,
			func(ctx context.Context, _ int64, e eventbus.Event) error {
				cancelled := e.(checkoutintegration.OrderCancelled)
				if cancelled.CustomerID == "" {
					return nil
				}
				view, err := query.Find(ctx, cancelled.OrderID)
				if err != nil {
					return fmt.Errorf("order cancelled: load view: %w", err)
				}
				msg, err := layout.RenderOrderCancelled(view, cancelled.Reason, baseURL)
				if err != nil {
					return fmt.Errorf("order cancelled: render: %w", err)
				}
				msg.To = cancelled.CustomerID
				return rec.Send(ctx, msg)
			},
		),
	)

	// email.order-payment-failed
	bus.SubscribeWithID(
		checkoutintegration.OrderPaymentFailed{}.EventName(),
		inbox.Wrap("email.order-payment-failed", ib,
			func(ctx context.Context, _ int64, e eventbus.Event) error {
				failed := e.(checkoutintegration.OrderPaymentFailed)
				if failed.CustomerID == "" {
					return nil
				}
				view, err := query.Find(ctx, failed.OrderID)
				if err != nil {
					return fmt.Errorf("order payment failed: load view: %w", err)
				}
				msg, err := layout.RenderPaymentFailed(view, failed.Reason, baseURL)
				if err != nil {
					return fmt.Errorf("order payment failed: render: %w", err)
				}
				msg.To = failed.CustomerID
				return rec.Send(ctx, msg)
			},
		),
	)

	// email.order-delivered
	bus.Subscribe(
		fulfillmentintegration.OrderDelivered{}.EventName(),
		func(ctx context.Context, e eventbus.Event) error {
			delivered := e.(fulfillmentintegration.OrderDelivered)
			view, err := query.Find(ctx, delivered.OrderID)
			if err != nil {
				return fmt.Errorf("order delivered: load view: %w", err)
			}
			if view.CustomerID() == "" {
				return nil
			}
			msg, err := layout.RenderOrderDelivered(view, baseURL)
			if err != nil {
				return fmt.Errorf("order delivered: render: %w", err)
			}
			msg.To = view.CustomerID()
			return rec.Send(ctx, msg)
		},
	)

	// email.order-refunded
	bus.Subscribe(
		fulfillmentintegration.OrderRefunded{}.EventName(),
		func(ctx context.Context, e eventbus.Event) error {
			refunded := e.(fulfillmentintegration.OrderRefunded)
			view, err := query.Find(ctx, refunded.OrderID)
			if err != nil {
				return fmt.Errorf("order refunded: load view: %w", err)
			}
			if view.CustomerID() == "" {
				return nil
			}
			msg, err := layout.RenderOrderRefunded(view, refunded.Reason, baseURL)
			if err != nil {
				return fmt.Errorf("order refunded: render: %w", err)
			}
			msg.To = view.CustomerID()
			return rec.Send(ctx, msg)
		},
	)
}

func TestOrderLifecycleEmails_EndToEnd(t *testing.T) {
	ctx := context.Background()
	logger := discardLogger()
	bus := eventbus.New(logger)
	ib := newInMemoryInbox()
	out := newInMemoryOutbox()
	orderStore := newInMemoryOrderStorage(out)
	query := checkoutquery.NewService(orderStore)
	mailerRec := &recordingMailer{}
	baseURL := "https://shop.example.com"

	wireEmailLifecycleSubscribers(bus, ib, query, mailerRec, baseURL)

	// Seed an order in orderStore
	testOrder := checkoutdomain.NewOrder(
		"ord-lifecycle-100",
		"session-1",
		"customer@example.com",
		checkoutdomain.RebuildAddress("John Doe", "1 Market St", "", "San Francisco", "94105", "US"),
		checkoutdomain.RebuildShippingMethod("standard", "Standard Shipping", 1500),
		checkoutdomain.RebuildPaymentMethod("stripe", "Credit Card"),
		[]checkoutdomain.Line{
			checkoutdomain.NewLine("prod-1", "Pro Laptop", 1, 120000, "USD"),
		},
		checkoutdomain.StatusPending,
		time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
	)
	if err := orderStore.Save(ctx, &testOrder); err != nil {
		t.Fatalf("save initial order: %v", err)
	}

	t.Run("OrderPlaced transactional email with inbox idempotency", func(t *testing.T) {
		// Stage OrderPlaced in outbox
		payload, err := json.Marshal(checkoutintegration.OrderPlaced{
			OrderID:    testOrder.ID(),
			CustomerID: testOrder.CustomerID(),
			At:         time.Now(),
		})
		if err != nil {
			t.Fatalf("marshal OrderPlaced: %v", err)
		}
		out.append(checkoutintegration.OrderPlaced{}.EventName(), payload)

		// Dispatch once
		dispatchOnce(t, ctx, out, bus, decodeAllOutbox)

		sends := mailerRec.sent()
		if len(sends) != 1 {
			t.Fatalf("expected 1 email sent, got %d", len(sends))
		}
		msg := sends[0]
		if msg.Kind != mailer.KindOrderPlaced {
			t.Errorf("expected KindOrderPlaced, got %s", msg.Kind)
		}
		if msg.To != "customer@example.com" {
			t.Errorf("expected to customer@example.com, got %s", msg.To)
		}
		if !strings.Contains(msg.Subject, "ord-lifecycle-100") {
			t.Errorf("subject missing order ID: %s", msg.Subject)
		}
		if !strings.Contains(msg.HTMLBody, "Pro Laptop") {
			t.Errorf("HTMLBody missing item name")
		}

		// Dispatch again with same outbox row ID to verify inbox idempotency
		bus.PublishWithID(ctx, 1, checkoutintegration.OrderPlaced{
			OrderID:    testOrder.ID(),
			CustomerID: testOrder.CustomerID(),
			At:         time.Now(),
		})
		if len(mailerRec.sent()) != 1 {
			t.Errorf("duplicate message sent! inbox idempotency failed")
		}
	})

	t.Run("OrderPaymentFailed transactional email", func(t *testing.T) {
		payload, err := json.Marshal(checkoutintegration.OrderPaymentFailed{
			OrderID:    testOrder.ID(),
			CustomerID: testOrder.CustomerID(),
			Reason:     "Card was declined",
			At:         time.Now(),
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out.append(checkoutintegration.OrderPaymentFailed{}.EventName(), payload)

		dispatchOnce(t, ctx, out, bus, decodeAllOutbox)

		sends := mailerRec.sent()
		if len(sends) != 2 {
			t.Fatalf("expected 2 emails sent so far, got %d", len(sends))
		}
		msg := sends[1]
		if msg.Kind != mailer.KindOrderPaymentFailed {
			t.Errorf("expected KindOrderPaymentFailed, got %s", msg.Kind)
		}
		if !strings.Contains(msg.Subject, "Payment failed") {
			t.Errorf("expected 'Payment failed' in subject, got %s", msg.Subject)
		}
		if !strings.Contains(msg.HTMLBody, "Card was declined") {
			t.Errorf("expected failure reason in HTMLBody")
		}
	})

	t.Run("OrderCancelled transactional email", func(t *testing.T) {
		payload, err := json.Marshal(checkoutintegration.OrderCancelled{
			OrderID:    testOrder.ID(),
			CustomerID: testOrder.CustomerID(),
			Reason:     "Cancelled by customer",
			At:         time.Now(),
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out.append(checkoutintegration.OrderCancelled{}.EventName(), payload)

		dispatchOnce(t, ctx, out, bus, decodeAllOutbox)

		sends := mailerRec.sent()
		if len(sends) != 3 {
			t.Fatalf("expected 3 emails sent so far, got %d", len(sends))
		}
		msg := sends[2]
		if msg.Kind != mailer.KindOrderCancelled {
			t.Errorf("expected KindOrderCancelled, got %s", msg.Kind)
		}
		if !strings.Contains(msg.Subject, "cancelled") {
			t.Errorf("expected 'cancelled' in subject, got %s", msg.Subject)
		}
		if !strings.Contains(msg.HTMLBody, "Cancelled by customer") {
			t.Errorf("expected reason in HTMLBody")
		}
	})

	t.Run("OrderDelivered transactional email", func(t *testing.T) {
		bus.Publish(ctx, fulfillmentintegration.OrderDelivered{
			OrderID: testOrder.ID(),
			At:      time.Now(),
		})

		sends := mailerRec.sent()
		if len(sends) != 4 {
			t.Fatalf("expected 4 emails sent so far, got %d", len(sends))
		}
		msg := sends[3]
		if msg.Kind != mailer.KindOrderDelivered {
			t.Errorf("expected KindOrderDelivered, got %s", msg.Kind)
		}
		if !strings.Contains(msg.Subject, "delivered") {
			t.Errorf("expected 'delivered' in subject, got %s", msg.Subject)
		}
	})

	t.Run("OrderRefunded transactional email", func(t *testing.T) {
		bus.Publish(ctx, fulfillmentintegration.OrderRefunded{
			OrderID: testOrder.ID(),
			Reason:  "Item defective",
			At:      time.Now(),
		})

		sends := mailerRec.sent()
		if len(sends) != 5 {
			t.Fatalf("expected 5 emails sent so far, got %d", len(sends))
		}
		msg := sends[4]
		if msg.Kind != mailer.KindOrderRefunded {
			t.Errorf("expected KindOrderRefunded, got %s", msg.Kind)
		}
		if !strings.Contains(msg.Subject, "refunded") {
			t.Errorf("expected 'refunded' in subject, got %s", msg.Subject)
		}
		if !strings.Contains(msg.HTMLBody, "Item defective") {
			t.Errorf("expected refund reason in HTMLBody")
		}
	})
}
