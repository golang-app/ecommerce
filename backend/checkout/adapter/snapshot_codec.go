package adapter

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

// This file is the only place that knows how an Order aggregate snapshot is
// serialised for the snapshot store. The DTO mirrors domain.OrderSnapshot
// one-to-one and uses the same value-object rebuilders the event codec
// relies on, so an order rebuilt from a snapshot is indistinguishable from
// one rebuilt by replaying its full event history.

type orderSnapshotDTO struct {
	ID           string            `json:"id"`
	UserID       string            `json:"user_id"`
	CustomerID   string            `json:"customer_id"`
	ShipTo       addressDTO        `json:"ship_to"`
	ShipMethod   shippingMethodDTO `json:"ship_method"`
	PayMethod    paymentMethodDTO  `json:"pay_method"`
	Items        []lineDTO         `json:"items"`
	SubtotalAmt  int64             `json:"subtotal_amt"`
	TaxAmt       int64             `json:"tax_amt"`
	ShipCostAmt  int64             `json:"ship_cost_amt"`
	DiscountCode string            `json:"discount_code,omitempty"`
	DiscountAmt  int64             `json:"discount_amt,omitempty"`
	TotalAmt     int64             `json:"total_amt"`
	TotalCcy     string            `json:"total_ccy"`
	Status       string            `json:"status"`
	PlacedAt     time.Time         `json:"placed_at"`
	Carrier      string            `json:"carrier,omitempty"`
	TrackingCode string            `json:"tracking_code,omitempty"`
	Version      int               `json:"version"`
}

// marshalSnapshot returns the JSON payload for a domain snapshot DTO.
func marshalSnapshot(snap domain.OrderSnapshot) ([]byte, error) {
	items := make([]lineDTO, 0, len(snap.Items))
	for _, l := range snap.Items {
		items = append(items, lineDTO{
			ProductID:     l.ProductID(),
			ProductName:   l.ProductName(),
			Qty:           l.Quantity(),
			PriceAmount:   l.PriceAmount(),
			PriceCurrency: l.PriceCurrency(),
		})
	}
	dto := orderSnapshotDTO{
		ID:           snap.ID,
		UserID:       snap.UserID,
		CustomerID:   snap.CustomerID,
		ShipTo:       toAddressDTO(snap.ShipTo),
		ShipMethod:   shippingMethodDTO{Code: snap.ShipMethod.Code(), Label: snap.ShipMethod.Label(), Cost: snap.ShipMethod.Cost()},
		PayMethod:    paymentMethodDTO{Code: snap.PayMethod.Code(), Label: snap.PayMethod.Label()},
		Items:        items,
		SubtotalAmt:  snap.SubtotalAmt,
		TaxAmt:       snap.TaxAmt,
		ShipCostAmt:  snap.ShipCostAmt,
		DiscountCode: snap.DiscountCode,
		DiscountAmt:  snap.DiscountAmt,
		TotalAmt:     snap.TotalAmt,
		TotalCcy:     snap.TotalCcy,
		Status:       string(snap.Status),
		PlacedAt:     snap.PlacedAt,
		Carrier:      snap.Carrier,
		TrackingCode: snap.TrackingCode,
		Version:      snap.Version,
	}
	b, err := json.Marshal(dto)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	return b, nil
}

// unmarshalSnapshot rebuilds a domain snapshot DTO from its stored payload.
func unmarshalSnapshot(payload []byte) (domain.OrderSnapshot, error) {
	var dto orderSnapshotDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		return domain.OrderSnapshot{}, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	items := make([]domain.Line, 0, len(dto.Items))
	for _, l := range dto.Items {
		items = append(items, domain.NewLine(l.ProductID, l.ProductName, l.Qty, l.PriceAmount, l.PriceCurrency))
	}
	return domain.OrderSnapshot{
		ID:           dto.ID,
		UserID:       dto.UserID,
		CustomerID:   dto.CustomerID,
		ShipTo:       domain.RebuildAddress(dto.ShipTo.Name, dto.ShipTo.Street1, dto.ShipTo.Street2, dto.ShipTo.City, dto.ShipTo.Zip, dto.ShipTo.Country),
		ShipMethod:   domain.RebuildShippingMethod(dto.ShipMethod.Code, dto.ShipMethod.Label, dto.ShipMethod.Cost),
		PayMethod:    domain.RebuildPaymentMethod(dto.PayMethod.Code, dto.PayMethod.Label),
		Items:        items,
		SubtotalAmt:  dto.SubtotalAmt,
		TaxAmt:       dto.TaxAmt,
		ShipCostAmt:  dto.ShipCostAmt,
		DiscountCode: dto.DiscountCode,
		DiscountAmt:  dto.DiscountAmt,
		TotalAmt:     dto.TotalAmt,
		TotalCcy:     dto.TotalCcy,
		Status:       domain.Status(dto.Status),
		PlacedAt:     dto.PlacedAt,
		Carrier:      dto.Carrier,
		TrackingCode: dto.TrackingCode,
		Version:      dto.Version,
	}, nil
}
