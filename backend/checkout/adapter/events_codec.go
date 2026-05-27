package adapter

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

// This file is the only place that knows how checkout events are serialised
// for the event store. Domain events stay pure; here we map them to/from
// exported JSON DTOs and rebuild the value objects through their domain
// constructors.

type addressDTO struct {
	Name    string `json:"name"`
	Street1 string `json:"street1"`
	Street2 string `json:"street2"`
	City    string `json:"city"`
	Zip     string `json:"zip"`
	Country string `json:"country"`
}

type shippingMethodDTO struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Cost  int64  `json:"cost"`
}

type paymentMethodDTO struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type lineDTO struct {
	ProductID     string `json:"product_id"`
	ProductName   string `json:"product_name"`
	Qty           int    `json:"qty"`
	PriceAmount   int64  `json:"price_amount"`
	PriceCurrency string `json:"price_currency"`
}

type orderPlacedDTO struct {
	OrderID    string            `json:"order_id"`
	UserID     string            `json:"user_id"`
	CustomerID string            `json:"customer_id"`
	ShipTo     addressDTO        `json:"ship_to"`
	ShipMethod shippingMethodDTO `json:"ship_method"`
	PayMethod  paymentMethodDTO  `json:"pay_method"`
	Lines      []lineDTO         `json:"lines"`
	At         time.Time         `json:"at"`
}

type paymentSucceededDTO struct {
	OrderID string    `json:"order_id"`
	At      time.Time `json:"at"`
}

type paymentFailedDTO struct {
	OrderID string    `json:"order_id"`
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

// marshalEvent returns the stored type name and JSON payload for a domain event.
func marshalEvent(e domain.Event) (string, []byte, error) {
	var payload any
	switch ev := e.(type) {
	case domain.OrderPlaced:
		lines := make([]lineDTO, 0, len(ev.Lines))
		for _, l := range ev.Lines {
			lines = append(lines, lineDTO{
				ProductID:     l.ProductID(),
				ProductName:   l.ProductName(),
				Qty:           l.Quantity(),
				PriceAmount:   l.PriceAmount(),
				PriceCurrency: l.PriceCurrency(),
			})
		}
		payload = orderPlacedDTO{
			OrderID:    ev.OrderID,
			UserID:     ev.UserID,
			CustomerID: ev.CustomerID,
			ShipTo:     toAddressDTO(ev.ShipTo),
			ShipMethod: shippingMethodDTO{Code: ev.ShipMethod.Code(), Label: ev.ShipMethod.Label(), Cost: ev.ShipMethod.Cost()},
			PayMethod:  paymentMethodDTO{Code: ev.PayMethod.Code(), Label: ev.PayMethod.Label()},
			Lines:      lines,
			At:         ev.At,
		}
	case domain.PaymentSucceeded:
		payload = paymentSucceededDTO{OrderID: ev.OrderID, At: ev.At}
	case domain.PaymentFailed:
		payload = paymentFailedDTO{OrderID: ev.OrderID, Reason: ev.Reason, At: ev.At}
	default:
		return "", nil, fmt.Errorf("no codec for event %s", e.EventType())
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", nil, fmt.Errorf("marshal %s: %w", e.EventType(), err)
	}
	return e.EventType(), b, nil
}

// unmarshalEvent rebuilds a domain event from its stored type + payload.
func unmarshalEvent(eventType string, payload []byte) (domain.Event, error) {
	switch eventType {
	case "OrderPlaced":
		var dto orderPlacedDTO
		if err := json.Unmarshal(payload, &dto); err != nil {
			return nil, fmt.Errorf("unmarshal OrderPlaced: %w", err)
		}
		lines := make([]domain.Line, 0, len(dto.Lines))
		for _, l := range dto.Lines {
			lines = append(lines, domain.NewLine(l.ProductID, l.ProductName, l.Qty, l.PriceAmount, l.PriceCurrency))
		}
		return domain.OrderPlaced{
			OrderID:    dto.OrderID,
			UserID:     dto.UserID,
			CustomerID: dto.CustomerID,
			ShipTo:     domain.RebuildAddress(dto.ShipTo.Name, dto.ShipTo.Street1, dto.ShipTo.Street2, dto.ShipTo.City, dto.ShipTo.Zip, dto.ShipTo.Country),
			ShipMethod: domain.RebuildShippingMethod(dto.ShipMethod.Code, dto.ShipMethod.Label, dto.ShipMethod.Cost),
			PayMethod:  domain.RebuildPaymentMethod(dto.PayMethod.Code, dto.PayMethod.Label),
			Lines:      lines,
			At:         dto.At,
		}, nil
	case "PaymentSucceeded":
		var dto paymentSucceededDTO
		if err := json.Unmarshal(payload, &dto); err != nil {
			return nil, fmt.Errorf("unmarshal PaymentSucceeded: %w", err)
		}
		return domain.PaymentSucceeded{OrderID: dto.OrderID, At: dto.At}, nil
	case "PaymentFailed":
		var dto paymentFailedDTO
		if err := json.Unmarshal(payload, &dto); err != nil {
			return nil, fmt.Errorf("unmarshal PaymentFailed: %w", err)
		}
		return domain.PaymentFailed{OrderID: dto.OrderID, Reason: dto.Reason, At: dto.At}, nil
	default:
		return nil, fmt.Errorf("unknown event type %q", eventType)
	}
}

func toAddressDTO(a domain.Address) addressDTO {
	return addressDTO{
		Name:    a.Name(),
		Street1: a.Street1(),
		Street2: a.Street2(),
		City:    a.City(),
		Zip:     a.Zip(),
		Country: a.Country(),
	}
}
