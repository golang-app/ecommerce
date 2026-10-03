package integration_test

import (
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/integration"
	"github.com/matryer/is"
)

func TestEventNames(t *testing.T) {
	is := is.New(t)

	is.Equal(integration.OrderPaid{}.EventName(), "checkout.OrderPaid")
	is.Equal(integration.OrderPlaced{}.EventName(), "checkout.OrderPlaced")
	is.Equal(integration.OrderCancelled{}.EventName(), "checkout.OrderCancelled")
	is.Equal(integration.OrderPaymentFailed{}.EventName(), "checkout.OrderPaymentFailed")
}

func TestEventFields(t *testing.T) {
	is := is.New(t)
	now := time.Now()

	placed := integration.OrderPlaced{
		OrderID:    "ord-1",
		CustomerID: "cust-1",
		At:         now,
	}
	is.Equal(placed.OrderID, "ord-1")
	is.Equal(placed.CustomerID, "cust-1")
	is.Equal(placed.At, now)

	cancelled := integration.OrderCancelled{
		OrderID:    "ord-2",
		CustomerID: "cust-2",
		Reason:     "out of stock",
		At:         now,
	}
	is.Equal(cancelled.OrderID, "ord-2")
	is.Equal(cancelled.CustomerID, "cust-2")
	is.Equal(cancelled.Reason, "out of stock")
	is.Equal(cancelled.At, now)

	failed := integration.OrderPaymentFailed{
		OrderID:    "ord-3",
		CustomerID: "cust-3",
		Reason:     "card declined",
		At:         now,
	}
	is.Equal(failed.OrderID, "ord-3")
	is.Equal(failed.CustomerID, "cust-3")
	is.Equal(failed.Reason, "card declined")
	is.Equal(failed.At, now)
}
