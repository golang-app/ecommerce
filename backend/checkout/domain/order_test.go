package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

func paidOrderEvents() []domain.Event {
	at := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	return []domain.Event{
		domain.OrderPlaced{
			OrderID:    "ord-1",
			UserID:     "cart-1",
			CustomerID: "jane@example.com",
			ShipTo:     domain.RebuildAddress("Jane", "1 Main St", "", "PDX", "97201", "USA"),
			ShipMethod: domain.RebuildShippingMethod("courier", "Courier", 1500),
			PayMethod:  domain.RebuildPaymentMethod("card", "Credit / debit card"),
			Lines:      []domain.Line{domain.NewLine("v1", "Item", 2, 1000, "USD")},
			At:         at,
		},
		domain.PaymentSucceeded{OrderID: "ord-1", At: at},
	}
}

func TestRehydrate_FoldsToPaid(t *testing.T) {
	o := domain.RehydrateOrder(paidOrderEvents())

	if o.Status() != domain.StatusPaid {
		t.Fatalf("status = %q, want paid", o.Status())
	}
	if len(o.PendingEvents()) != 0 {
		t.Errorf("rehydrated order should have no pending events, got %d", len(o.PendingEvents()))
	}
	// subtotal 2*1000 + 1500 shipping
	if o.TotalAmount() != 3500 {
		t.Errorf("total = %d, want 3500", o.TotalAmount())
	}
}

func TestCancel_RehydrateCommandAppend(t *testing.T) {
	o := domain.RehydrateOrder(paidOrderEvents())

	if err := o.Cancel("changed mind", time.Now()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	if o.Status() != domain.StatusCancelled {
		t.Errorf("status = %q, want cancelled", o.Status())
	}
	pending := o.PendingEvents()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(pending))
	}
	if _, ok := pending[0].(domain.OrderCancelled); !ok {
		t.Errorf("pending event = %T, want OrderCancelled", pending[0])
	}
	// Two events were folded in; the cancellation must append at sequence 3.
	if o.ExpectedVersion() != 2 {
		t.Errorf("ExpectedVersion = %d, want 2 (cancellation appends at seq 3)", o.ExpectedVersion())
	}
}

func TestCancel_NotAllowedTwice(t *testing.T) {
	o := domain.RehydrateOrder(paidOrderEvents())
	if err := o.Cancel("first", time.Now()); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	if err := o.Cancel("again", time.Now()); !errors.Is(err, domain.ErrOrderNotCancellable) {
		t.Errorf("second cancel err = %v, want ErrOrderNotCancellable", err)
	}
}

func TestCancel_FailedOrderNotCancellable(t *testing.T) {
	at := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	o := domain.RehydrateOrder([]domain.Event{
		domain.OrderPlaced{OrderID: "ord-2", Lines: []domain.Line{domain.NewLine("v1", "Item", 1, 1000, "USD")}, ShipMethod: domain.RebuildShippingMethod("pickup", "Personal pickup", 0), At: at},
		domain.PaymentFailed{OrderID: "ord-2", Reason: "declined", At: at},
	})

	if o.Status() != domain.StatusFailed {
		t.Fatalf("status = %q, want failed", o.Status())
	}
	if err := o.Cancel("nope", time.Now()); !errors.Is(err, domain.ErrOrderNotCancellable) {
		t.Errorf("cancel of failed order err = %v, want ErrOrderNotCancellable", err)
	}
}
