package adapter

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/integration"
	"github.com/matryer/is"
)

// rehydratePaid builds an Order in StatusPaid by replaying an
// OrderPlaced followed by a PaymentSucceeded. Lifting the rehydration
// helper into the test keeps the fixture obvious — every field on the
// resulting order has an explicit origin.
func rehydratePaid(t *testing.T, orderID, sessionID, customerID string, at time.Time) (*domain.Order, domain.PaymentSucceeded) {
	t.Helper()
	placedAt := at.Add(-time.Minute)
	placed := domain.OrderPlaced{
		OrderID:    orderID,
		UserID:     sessionID,
		CustomerID: customerID,
		ShipTo:     domain.Address{},
		ShipMethod: domain.RebuildShippingMethod("pickup", "Pickup", 0),
		PayMethod:  domain.RebuildPaymentMethod("card", "Card"),
		Lines: []domain.Line{
			domain.NewLine("v1", "Mug", 1, 1500, "USD"),
		},
		At: placedAt,
	}
	paid := domain.PaymentSucceeded{OrderID: orderID, At: at}
	// Rehydrate folds the events so o.Status() ends at StatusPaid, but
	// rehydration clears pendingEvents — extractIntegrationEvents reads
	// pending separately, so we pass the same slice the adapter would
	// see at Save time.
	o := domain.RehydrateOrder([]domain.Event{placed, paid})
	return o, paid
}

func TestExtractIntegrationEvents_PaymentSucceededProducesOrderPaid(t *testing.T) {
	is := is.New(t)
	at := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	o, ps := rehydratePaid(t, "ord-42", "cart-abc", "jane@example.com", at)

	records, err := extractIntegrationEvents(o, []domain.Event{ps})
	is.NoErr(err)
	is.Equal(len(records), 1)
	is.Equal(records[0].Kind, integration.OrderPaid{}.EventName())

	var decoded integration.OrderPaid
	is.NoErr(json.Unmarshal(records[0].Payload, &decoded))
	is.Equal(decoded.OrderID, "ord-42")
	is.Equal(decoded.SessionID, "cart-abc")
	is.Equal(decoded.CustomerID, "jane@example.com")
	is.True(decoded.At.Equal(at))
}

func TestExtractIntegrationEvents_NotPaidEmitsNothing(t *testing.T) {
	is := is.New(t)
	at := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	placed := domain.OrderPlaced{
		OrderID:    "ord-43",
		UserID:     "cart-abc",
		ShipMethod: domain.RebuildShippingMethod("pickup", "Pickup", 0),
		PayMethod:  domain.RebuildPaymentMethod("card", "Card"),
		Lines: []domain.Line{
			domain.NewLine("v1", "Mug", 1, 1500, "USD"),
		},
		At: at,
	}
	o := domain.RehydrateOrder([]domain.Event{placed})

	// A pending order with just an OrderPlaced should NOT produce an
	// OrderPaid outbox record — the integration event tracks the paid
	// transition only.
	records, err := extractIntegrationEvents(o, []domain.Event{placed})
	is.NoErr(err)
	is.Equal(len(records), 0)
}

func TestExtractIntegrationEvents_JSONShapeMatchesIntegrationOrderPaid(t *testing.T) {
	// Sanity check: the JSON the adapter stages MUST round-trip back
	// through json.Unmarshal into the very same integration.OrderPaid
	// type the dispatcher's decode closure unmarshals into in main.go.
	// If anyone renames a field on integration.OrderPaid without
	// updating the producer, this assertion fails loudly.
	is := is.New(t)
	at := time.Date(2026, 5, 28, 11, 0, 0, 0, time.UTC)
	o, ps := rehydratePaid(t, "ord-99", "cart-xyz", "alex@example.com", at)
	records, err := extractIntegrationEvents(o, []domain.Event{ps})
	is.NoErr(err)
	is.Equal(len(records), 1)

	var got integration.OrderPaid
	is.NoErr(json.Unmarshal(records[0].Payload, &got))
	want := integration.OrderPaid{
		OrderID:    "ord-99",
		SessionID:  "cart-xyz",
		CustomerID: "alex@example.com",
		At:         at,
	}
	is.Equal(got.OrderID, want.OrderID)
	is.Equal(got.SessionID, want.SessionID)
	is.Equal(got.CustomerID, want.CustomerID)
	is.True(got.At.Equal(want.At))
}
