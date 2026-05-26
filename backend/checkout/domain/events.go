package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Event is a fact that happened to an order aggregate. Events are the source
// of truth; the read model is projected from them.
type Event interface {
	EventType() string
	OccurredAt() time.Time
}

// OrderPlaced is emitted when a customer places an order. It carries the full
// snapshot of what was ordered so the order is self-contained.
type OrderPlaced struct {
	OrderID    string         `json:"order_id"`
	UserID     string         `json:"user_id"`
	CustomerID string         `json:"customer_id"`
	ShipTo     Address        `json:"ship_to"`
	ShipMethod ShippingMethod `json:"ship_method"`
	PayMethod  PaymentMethod  `json:"pay_method"`
	Lines      []Line         `json:"lines"`
	At         time.Time      `json:"at"`
}

func (e OrderPlaced) EventType() string     { return "OrderPlaced" }
func (e OrderPlaced) OccurredAt() time.Time { return e.At }

// PaymentSucceeded is emitted when the payment for an order is captured.
type PaymentSucceeded struct {
	OrderID string    `json:"order_id"`
	At      time.Time `json:"at"`
}

func (e PaymentSucceeded) EventType() string     { return "PaymentSucceeded" }
func (e PaymentSucceeded) OccurredAt() time.Time { return e.At }

// PaymentFailed is emitted when the payment for an order is declined.
type PaymentFailed struct {
	OrderID string    `json:"order_id"`
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

func (e PaymentFailed) EventType() string     { return "PaymentFailed" }
func (e PaymentFailed) OccurredAt() time.Time { return e.At }

// UnmarshalEvent reconstructs a domain event from its stored type + payload.
func UnmarshalEvent(eventType string, payload []byte) (Event, error) {
	switch eventType {
	case "OrderPlaced":
		var e OrderPlaced
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("unmarshal OrderPlaced: %w", err)
		}
		return e, nil
	case "PaymentSucceeded":
		var e PaymentSucceeded
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("unmarshal PaymentSucceeded: %w", err)
		}
		return e, nil
	case "PaymentFailed":
		var e PaymentFailed
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("unmarshal PaymentFailed: %w", err)
		}
		return e, nil
	default:
		return nil, fmt.Errorf("unknown event type %q", eventType)
	}
}
