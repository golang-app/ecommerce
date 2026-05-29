package domain

import "time"

// OrderSnapshot mirrors the domain.Order aggregate one-to-one as a plain,
// public-shape DTO. It captures every field the apply() folder writes to,
// so an aggregate reconstructed from a snapshot is byte-equivalent to one
// rehydrated by replaying its full event history up to the same version.
//
// The DTO is intentionally exported and free of behaviour: the adapter
// (checkout/adapter) marshals and unmarshals it as JSON. Keeping the type
// in the domain package is what lets RehydrateOrderFromSnapshot poke the
// aggregate's unexported fields without breaking encapsulation for the
// rest of the codebase.
type OrderSnapshot struct {
	ID           string
	UserID       string
	CustomerID   string
	ShipTo       Address
	ShipMethod   ShippingMethod
	PayMethod    PaymentMethod
	Items        []Line
	SubtotalAmt  int64
	TaxAmt       int64
	ShipCostAmt  int64
	DiscountCode string
	DiscountAmt  int64
	TotalAmt     int64
	TotalCcy     string
	Status       Status
	PlacedAt     time.Time
	Carrier      string
	TrackingCode string
	Channel      string
	Version      int
}

// SnapshotOrder builds an OrderSnapshot from the aggregate's current state
// using the public getters. It is defined as a package-level function so
// the adapter can encode a snapshot without reaching into unexported
// fields — the asymmetry with the rehydration helper (which does need the
// unexported access) is intentional.
func SnapshotOrder(o *Order) OrderSnapshot {
	if o == nil {
		return OrderSnapshot{}
	}
	items := make([]Line, len(o.items))
	copy(items, o.items)
	return OrderSnapshot{
		ID:           o.ID(),
		UserID:       o.UserID(),
		CustomerID:   o.CustomerID(),
		ShipTo:       o.ShipTo(),
		ShipMethod:   o.ShippingMethod(),
		PayMethod:    o.PaymentMethod(),
		Items:        items,
		SubtotalAmt:  o.Subtotal(),
		TaxAmt:       o.TaxAmount(),
		ShipCostAmt:  o.ShippingCost(),
		DiscountCode: o.DiscountCode(),
		DiscountAmt:  o.DiscountAmount(),
		TotalAmt:     o.TotalAmount(),
		TotalCcy:     o.TotalCurrency(),
		Status:       o.Status(),
		PlacedAt:     o.PlacedAt(),
		Carrier:      o.Carrier(),
		TrackingCode: o.TrackingCode(),
		Channel:      o.Channel(),
		Version:      o.version,
	}
}

// RehydrateOrderFromSnapshot initialises an aggregate from a snapshot at
// version V, then folds the supplied tail events on top. The result is the
// same as RehydrateOrder(allEvents) — snapshots are an optimisation, not a
// new truth. The function lives in the domain package because seeding the
// aggregate fields without re-running apply() requires unexported access.
func RehydrateOrderFromSnapshot(snap OrderSnapshot, tail []Event) *Order {
	items := make([]Line, len(snap.Items))
	copy(items, snap.Items)
	o := &Order{
		id:           snap.ID,
		userID:       snap.UserID,
		customerID:   snap.CustomerID,
		shipTo:       snap.ShipTo,
		shipMethod:   snap.ShipMethod,
		payMethod:    snap.PayMethod,
		items:        items,
		subtotalAmt:  snap.SubtotalAmt,
		taxAmt:       snap.TaxAmt,
		shipCostAmt:  snap.ShipCostAmt,
		discountCode: snap.DiscountCode,
		discountAmt:  snap.DiscountAmt,
		totalAmt:     snap.TotalAmt,
		totalCcy:     snap.TotalCcy,
		status:       snap.Status,
		placedAt:     snap.PlacedAt,
		carrier:      snap.Carrier,
		trackingCode: snap.TrackingCode,
		channel:      snap.Channel,
		version:      snap.Version,
	}
	ApplyTail(o, tail)
	return o
}

// ApplyTail folds a sequence of events into the aggregate as a tail update,
// incrementing the version per event via apply. It is the adapter-facing
// hook for snapshot-based loads: Load resolves the snapshot, fetches events
// with sequence > snapshot.version, and lets the domain replay them.
func ApplyTail(o *Order, events []Event) {
	if o == nil {
		return
	}
	for _, e := range events {
		o.apply(e)
	}
	o.pendingEvents = nil
}
