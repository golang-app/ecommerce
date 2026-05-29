package adapter

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

// Event versioning + upcasting.
//
// Why this file exists: the checkout aggregate is event-sourced, so the event
// log is the source of truth for every order — and the log lives forever.
// When the domain learns a new fact (here: which sales channel an order came
// in through), the wire shape of the affected event must evolve, but rows
// written under the old shape still need to fold back into a working
// aggregate. We resolve that tension with the textbook upcaster pattern:
//
//  1. Every row in checkout_events carries a payload_version int. Different
//     versions can coexist in the same table.
//  2. On load the codec dispatches on (event_type, version), unmarshals the
//     matching DTO, and — if it isn't the latest version — runs an
//     upcastXxxVN function that translates the old payload into the LATEST
//     in-memory event shape (filling sensible defaults for new fields).
//  3. domain.Order.apply only ever sees the latest event shape. All
//     historical compatibility is contained here; the aggregate stays
//     version-blind.
//
// Contract for adding new versions: bump the version returned by
// marshalEvent for that event type, add a new orderXxxDTOvN, keep the old
// DTO + write an upcastXxxVN that returns the v(N) domain event, and
// extend the version switch in unmarshalEvent. Other event types are free
// to evolve independently — their version space is local.

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

// orderPlacedDTOv1 is the historical wire shape of OrderPlaced — kept around
// purely so the upcaster can read rows that were written before the Channel
// field existed. Do not extend this type: bump to a new vN instead.
type orderPlacedDTOv1 struct {
	OrderID        string            `json:"order_id"`
	UserID         string            `json:"user_id"`
	CustomerID     string            `json:"customer_id"`
	ShipTo         addressDTO        `json:"ship_to"`
	ShipMethod     shippingMethodDTO `json:"ship_method"`
	PayMethod      paymentMethodDTO  `json:"pay_method"`
	Lines          []lineDTO         `json:"lines"`
	Tax            int64             `json:"tax,omitempty"`
	ShippingCost   int64             `json:"shipping_cost,omitempty"`
	DiscountCode   string            `json:"discount_code,omitempty"`
	DiscountAmount int64             `json:"discount_amount,omitempty"`
	At             time.Time         `json:"at"`
}

// orderPlacedDTOv2 is the current wire shape of OrderPlaced and matches the
// in-memory domain.OrderPlaced one-to-one. The Channel field is new in v2;
// rows on the older shape are translated into this one by
// upcastOrderPlacedV1.
type orderPlacedDTOv2 struct {
	OrderID        string            `json:"order_id"`
	UserID         string            `json:"user_id"`
	CustomerID     string            `json:"customer_id"`
	ShipTo         addressDTO        `json:"ship_to"`
	ShipMethod     shippingMethodDTO `json:"ship_method"`
	PayMethod      paymentMethodDTO  `json:"pay_method"`
	Lines          []lineDTO         `json:"lines"`
	Tax            int64             `json:"tax,omitempty"`
	ShippingCost   int64             `json:"shipping_cost,omitempty"`
	DiscountCode   string            `json:"discount_code,omitempty"`
	DiscountAmount int64             `json:"discount_amount,omitempty"`
	Channel        string            `json:"channel,omitempty"`
	At             time.Time         `json:"at"`
}

// orderPlacedLatestVersion is the only version marshalEvent will write going
// forward. Bumping this constant + adding the matching DTO is how a new
// schema rolls out.
const orderPlacedLatestVersion = 2

type paymentSucceededDTO struct {
	OrderID string    `json:"order_id"`
	At      time.Time `json:"at"`
}

type paymentFailedDTO struct {
	OrderID string    `json:"order_id"`
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

type orderCancelledDTO struct {
	OrderID string    `json:"order_id"`
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

type orderShippedDTO struct {
	OrderID      string    `json:"order_id"`
	Carrier      string    `json:"carrier,omitempty"`
	TrackingCode string    `json:"tracking_code,omitempty"`
	At           time.Time `json:"at"`
}

type orderDeliveredDTO struct {
	OrderID string    `json:"order_id"`
	At      time.Time `json:"at"`
}

type orderRefundedDTO struct {
	OrderID string    `json:"order_id"`
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

// marshalEvent returns the stored type name, payload version, and JSON
// payload for a domain event. Every event type owns its own version space:
// OrderPlaced is at v2 (Channel field added); the rest are still on v1
// since they haven't evolved.
func marshalEvent(e domain.Event) (string, int, []byte, error) {
	var payload any
	version := 1
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
		payload = orderPlacedDTOv2{
			OrderID:        ev.OrderID,
			UserID:         ev.UserID,
			CustomerID:     ev.CustomerID,
			ShipTo:         toAddressDTO(ev.ShipTo),
			ShipMethod:     shippingMethodDTO{Code: ev.ShipMethod.Code(), Label: ev.ShipMethod.Label(), Cost: ev.ShipMethod.Cost()},
			PayMethod:      paymentMethodDTO{Code: ev.PayMethod.Code(), Label: ev.PayMethod.Label()},
			Lines:          lines,
			Tax:            ev.Tax,
			ShippingCost:   ev.ShippingCost,
			DiscountCode:   ev.DiscountCode,
			DiscountAmount: ev.DiscountAmount,
			Channel:        ev.Channel,
			At:             ev.At,
		}
		version = orderPlacedLatestVersion
	case domain.PaymentSucceeded:
		payload = paymentSucceededDTO{OrderID: ev.OrderID, At: ev.At}
	case domain.PaymentFailed:
		payload = paymentFailedDTO{OrderID: ev.OrderID, Reason: ev.Reason, At: ev.At}
	case domain.OrderCancelled:
		payload = orderCancelledDTO{OrderID: ev.OrderID, Reason: ev.Reason, At: ev.At}
	case domain.OrderShipped:
		payload = orderShippedDTO{OrderID: ev.OrderID, Carrier: ev.Carrier, TrackingCode: ev.TrackingCode, At: ev.At}
	case domain.OrderDelivered:
		payload = orderDeliveredDTO{OrderID: ev.OrderID, At: ev.At}
	case domain.OrderRefunded:
		payload = orderRefundedDTO{OrderID: ev.OrderID, Reason: ev.Reason, At: ev.At}
	default:
		return "", 0, nil, fmt.Errorf("no codec for event %s", e.EventType())
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", 0, nil, fmt.Errorf("marshal %s: %w", e.EventType(), err)
	}
	return e.EventType(), version, b, nil
}

// unmarshalEvent rebuilds a domain event from its stored type + version +
// payload. The version switch is the single seam where historical payloads
// re-enter the latest in-memory shape; everything past this function sees
// only the latest domain event.
func unmarshalEvent(eventType string, version int, payload []byte) (domain.Event, error) {
	switch eventType {
	case "OrderPlaced":
		switch version {
		case 1:
			var v1 orderPlacedDTOv1
			if err := json.Unmarshal(payload, &v1); err != nil {
				return nil, fmt.Errorf("unmarshal OrderPlaced v1: %w", err)
			}
			return upcastOrderPlacedV1(v1), nil
		case 2:
			var v2 orderPlacedDTOv2
			if err := json.Unmarshal(payload, &v2); err != nil {
				return nil, fmt.Errorf("unmarshal OrderPlaced v2: %w", err)
			}
			return orderPlacedFromDTOv2(v2), nil
		default:
			return nil, fmt.Errorf("unknown OrderPlaced version %d", version)
		}
	case "PaymentSucceeded":
		if version != 1 {
			return nil, fmt.Errorf("unknown PaymentSucceeded version %d", version)
		}
		var dto paymentSucceededDTO
		if err := json.Unmarshal(payload, &dto); err != nil {
			return nil, fmt.Errorf("unmarshal PaymentSucceeded: %w", err)
		}
		return domain.PaymentSucceeded{OrderID: dto.OrderID, At: dto.At}, nil
	case "PaymentFailed":
		if version != 1 {
			return nil, fmt.Errorf("unknown PaymentFailed version %d", version)
		}
		var dto paymentFailedDTO
		if err := json.Unmarshal(payload, &dto); err != nil {
			return nil, fmt.Errorf("unmarshal PaymentFailed: %w", err)
		}
		return domain.PaymentFailed{OrderID: dto.OrderID, Reason: dto.Reason, At: dto.At}, nil
	case "OrderCancelled":
		if version != 1 {
			return nil, fmt.Errorf("unknown OrderCancelled version %d", version)
		}
		var dto orderCancelledDTO
		if err := json.Unmarshal(payload, &dto); err != nil {
			return nil, fmt.Errorf("unmarshal OrderCancelled: %w", err)
		}
		return domain.OrderCancelled{OrderID: dto.OrderID, Reason: dto.Reason, At: dto.At}, nil
	case "OrderShipped":
		if version != 1 {
			return nil, fmt.Errorf("unknown OrderShipped version %d", version)
		}
		var dto orderShippedDTO
		if err := json.Unmarshal(payload, &dto); err != nil {
			return nil, fmt.Errorf("unmarshal OrderShipped: %w", err)
		}
		return domain.OrderShipped{OrderID: dto.OrderID, Carrier: dto.Carrier, TrackingCode: dto.TrackingCode, At: dto.At}, nil
	case "OrderDelivered":
		if version != 1 {
			return nil, fmt.Errorf("unknown OrderDelivered version %d", version)
		}
		var dto orderDeliveredDTO
		if err := json.Unmarshal(payload, &dto); err != nil {
			return nil, fmt.Errorf("unmarshal OrderDelivered: %w", err)
		}
		return domain.OrderDelivered{OrderID: dto.OrderID, At: dto.At}, nil
	case "OrderRefunded":
		if version != 1 {
			return nil, fmt.Errorf("unknown OrderRefunded version %d", version)
		}
		var dto orderRefundedDTO
		if err := json.Unmarshal(payload, &dto); err != nil {
			return nil, fmt.Errorf("unmarshal OrderRefunded: %w", err)
		}
		return domain.OrderRefunded{OrderID: dto.OrderID, Reason: dto.Reason, At: dto.At}, nil
	default:
		return nil, fmt.Errorf("unknown event type %q", eventType)
	}
}

// upcastOrderPlacedV1 is the upcaster for the OrderPlaced v1 → v2 schema
// jump. v1 rows pre-date the Channel field; we materialise them with
// Channel == "unknown" so historical orders read back with an explicit,
// auditable value rather than the zero string — making the upcast visible
// in tests and in the read model alike.
func upcastOrderPlacedV1(v1 orderPlacedDTOv1) domain.OrderPlaced {
	lines := make([]domain.Line, 0, len(v1.Lines))
	for _, l := range v1.Lines {
		lines = append(lines, domain.NewLine(l.ProductID, l.ProductName, l.Qty, l.PriceAmount, l.PriceCurrency))
	}
	return domain.OrderPlaced{
		OrderID:        v1.OrderID,
		UserID:         v1.UserID,
		CustomerID:     v1.CustomerID,
		ShipTo:         domain.RebuildAddress(v1.ShipTo.Name, v1.ShipTo.Street1, v1.ShipTo.Street2, v1.ShipTo.City, v1.ShipTo.Zip, v1.ShipTo.Country),
		ShipMethod:     domain.RebuildShippingMethod(v1.ShipMethod.Code, v1.ShipMethod.Label, v1.ShipMethod.Cost),
		PayMethod:      domain.RebuildPaymentMethod(v1.PayMethod.Code, v1.PayMethod.Label),
		Lines:          lines,
		Tax:            v1.Tax,
		ShippingCost:   v1.ShippingCost,
		DiscountCode:   v1.DiscountCode,
		DiscountAmount: v1.DiscountAmount,
		Channel:        "unknown",
		At:             v1.At,
	}
}

// orderPlacedFromDTOv2 is the no-op-shape mapping from the latest DTO to
// the domain event — it pairs with upcastOrderPlacedV1 so every code path
// in unmarshalEvent ends with the same domain.OrderPlaced shape.
func orderPlacedFromDTOv2(v2 orderPlacedDTOv2) domain.OrderPlaced {
	lines := make([]domain.Line, 0, len(v2.Lines))
	for _, l := range v2.Lines {
		lines = append(lines, domain.NewLine(l.ProductID, l.ProductName, l.Qty, l.PriceAmount, l.PriceCurrency))
	}
	return domain.OrderPlaced{
		OrderID:        v2.OrderID,
		UserID:         v2.UserID,
		CustomerID:     v2.CustomerID,
		ShipTo:         domain.RebuildAddress(v2.ShipTo.Name, v2.ShipTo.Street1, v2.ShipTo.Street2, v2.ShipTo.City, v2.ShipTo.Zip, v2.ShipTo.Country),
		ShipMethod:     domain.RebuildShippingMethod(v2.ShipMethod.Code, v2.ShipMethod.Label, v2.ShipMethod.Cost),
		PayMethod:      domain.RebuildPaymentMethod(v2.PayMethod.Code, v2.PayMethod.Label),
		Lines:          lines,
		Tax:            v2.Tax,
		ShippingCost:   v2.ShippingCost,
		DiscountCode:   v2.DiscountCode,
		DiscountAmount: v2.DiscountAmount,
		Channel:        v2.Channel,
		At:             v2.At,
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
