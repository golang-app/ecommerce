package adapter

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/app"
	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/domain"
	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/integration"
	"github.com/matryer/is"
)

type staticDetailReader struct {
	detail app.OrderDetail
	err    error
}

func (s staticDetailReader) OrderDetail(_ context.Context, _ string) (app.OrderDetail, error) {
	return s.detail, s.err
}

func TestExtractIntegrationEvents_DeliveredProducesOrderDelivered(t *testing.T) {
	is := is.New(t)
	at := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	f := domain.Rebuild("ful-1", "ord-1", domain.StatusShipped, "DHL", "TRACK123", at.Add(-time.Hour), at.Add(-30*time.Minute), time.Time{}, "", 2)
	err := f.Deliver(at)
	is.NoErr(err)

	records, err := extractIntegrationEvents(context.Background(), f, nil)
	is.NoErr(err)
	is.Equal(len(records), 1)
	is.Equal(records[0].Kind, integration.OrderDelivered{}.EventName())

	var decoded integration.OrderDelivered
	is.NoErr(json.Unmarshal(records[0].Payload, &decoded))
	is.Equal(decoded.OrderID, "ord-1")
	is.True(decoded.At.Equal(at))
}

func TestExtractIntegrationEvents_RefundedProducesOrderRefunded(t *testing.T) {
	is := is.New(t)
	at := time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC)
	f := domain.Rebuild("ful-2", "ord-2", domain.StatusDelivered, "DHL", "TRACK123", at.Add(-time.Hour), at.Add(-30*time.Minute), at.Add(-10*time.Minute), "", 3)
	err := f.Refund("item defective", at)
	is.NoErr(err)

	records, err := extractIntegrationEvents(context.Background(), f, nil)
	is.NoErr(err)
	is.Equal(len(records), 1)
	is.Equal(records[0].Kind, integration.OrderRefunded{}.EventName())

	var decoded integration.OrderRefunded
	is.NoErr(json.Unmarshal(records[0].Payload, &decoded))
	is.Equal(decoded.OrderID, "ord-2")
	is.Equal(decoded.Reason, "item defective")
	is.True(decoded.At.Equal(at))
}

func TestExtractIntegrationEvents_ShippedProducesNotificationAndECST(t *testing.T) {
	is := is.New(t)
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	f := domain.Rebuild("ful-3", "ord-3", domain.StatusLabeled, "FedEx", "TRACK999", at.Add(-time.Hour), time.Time{}, time.Time{}, "", 2)
	err := f.Ship(at)
	is.NoErr(err)

	reader := staticDetailReader{
		detail: app.OrderDetail{
			CustomerID: "cust-1",
			Email:      "customer@example.com",
			ShipTo: app.OrderDetailAddress{
				Name:    "Alice",
				Street1: "123 Main St",
				City:    "Springfield",
				Zip:     "12345",
				Country: "US",
			},
			Items: []app.OrderDetailLine{
				{
					ProductID:     "p1",
					ProductName:   "Widget",
					Quantity:      2,
					PriceAmount:   2000,
					PriceCurrency: "USD",
				},
			},
			Subtotal:     4000,
			Tax:          400,
			ShippingCost: 500,
			Total:        4900,
			Currency:     "USD",
		},
	}

	records, err := extractIntegrationEvents(context.Background(), f, reader)
	is.NoErr(err)
	is.Equal(len(records), 2)
	is.Equal(records[0].Kind, integration.OrderShipped{}.EventName())
	is.Equal(records[1].Kind, integration.OrderShippedECST{}.EventName())

	var notif integration.OrderShipped
	is.NoErr(json.Unmarshal(records[0].Payload, &notif))
	is.Equal(notif.OrderID, "ord-3")
	is.Equal(notif.Carrier, "FedEx")
	is.Equal(notif.TrackingCode, "TRACK999")
	is.True(notif.At.Equal(at))

	var ecst integration.OrderShippedECST
	is.NoErr(json.Unmarshal(records[1].Payload, &ecst))
	is.Equal(ecst.OrderID, "ord-3")
	is.Equal(ecst.Email, "customer@example.com")
	is.Equal(ecst.Carrier, "FedEx")
	is.Equal(ecst.TrackingCode, "TRACK999")
	is.Equal(len(ecst.Items), 1)
	is.Equal(ecst.Items[0].ProductName, "Widget")
	is.Equal(ecst.Total, int64(4900))
}
