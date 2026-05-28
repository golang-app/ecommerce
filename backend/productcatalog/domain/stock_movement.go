package domain

import "time"

// StockMovement is a single audit-log entry for a variant's stock level. A
// positive delta increases stock (release, refund, admin top-up); a negative
// delta decreases stock (reservation, manual adjustment down). RefOrderID is
// non-empty when the movement was driven by a checkout flow.
type StockMovement struct {
	ID         int64
	VariantID  string
	Delta      int
	Reason     string
	RefOrderID string
	At         time.Time
}

// NewStockMovement is the convenience constructor used by storage adapters
// when reading rows out of the audit log.
func NewStockMovement(id int64, variantID string, delta int, reason, refOrderID string, at time.Time) StockMovement {
	return StockMovement{
		ID:         id,
		VariantID:  variantID,
		Delta:      delta,
		Reason:     reason,
		RefOrderID: refOrderID,
		At:         at,
	}
}
