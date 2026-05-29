package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

// TestSnapshotEquivalentToFullReplay locks in the invariant that powers the
// adapter's snapshot-aware Load: building an aggregate from a snapshot at
// version V and folding the tail produces the exact same state as folding
// the full event history from scratch. The aggregate compares equal under
// reflect.DeepEqual including the unexported fields, so any drift in apply()
// or the snapshot DTO will break this test.
func TestSnapshotEquivalentToFullReplay(t *testing.T) {
	at := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	allEvents := []domain.Event{
		domain.OrderPlaced{
			OrderID:    "ord-snap",
			UserID:     "cart-1",
			CustomerID: "jane@example.com",
			ShipTo:     domain.RebuildAddress("Jane", "1 Main", "", "PDX", "97201", "USA"),
			ShipMethod: domain.RebuildShippingMethod("courier", "Courier", 1500),
			PayMethod:  domain.RebuildPaymentMethod("card", "Card"),
			Lines:      []domain.Line{domain.NewLine("v1", "Item", 2, 1000, "USD")},
			Tax:        160,
			At:         at,
		},
		domain.PaymentSucceeded{OrderID: "ord-snap", At: at.Add(time.Minute)},
		// snapshot is taken here — version=2 covers OrderPlaced + PaymentSucceeded.
		domain.OrderShipped{OrderID: "ord-snap", Carrier: "UPS", TrackingCode: "1Z-XYZ", At: at.Add(time.Hour)},
		domain.OrderDelivered{OrderID: "ord-snap", At: at.Add(2 * time.Hour)},
		domain.OrderRefunded{OrderID: "ord-snap", Reason: "return", At: at.Add(3 * time.Hour)},
	}

	// Build the snapshot from the first two events, then fold the remaining
	// three on top via RehydrateOrderFromSnapshot.
	base := domain.RehydrateOrder(allEvents[:2])
	snap := domain.SnapshotOrder(base)
	fromSnap := domain.RehydrateOrderFromSnapshot(snap, allEvents[2:])

	// Reference: fold the full history from zero.
	fromFull := domain.RehydrateOrder(allEvents)

	if !reflect.DeepEqual(fromSnap, fromFull) {
		t.Fatalf("snapshot+tail diverged from full replay\nsnap: %+v\nfull: %+v", fromSnap, fromFull)
	}
	// Sanity-check the visible state via getters too — DeepEqual covers it
	// already, but this makes the intent explicit at a glance.
	if fromSnap.Status() != domain.StatusRefunded {
		t.Errorf("status = %q, want refunded", fromSnap.Status())
	}
	if fromSnap.Version() != 5 {
		t.Errorf("version = %d, want 5", fromSnap.Version())
	}
}
