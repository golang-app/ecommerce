package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

// factoryFixture builds the inputs the happy-path tests reuse. Each
// rejection test mutates exactly one field to exercise a single guard.
type factoryFixture struct {
	id           string
	userID       string
	customerID   string
	shipTo       domain.Address
	shipMethod   domain.ShippingMethod
	payMethod    domain.PaymentMethod
	lines        []domain.Line
	quote        domain.Quote
	discountCode string
	channel      string
	at           time.Time
}

func newFactoryFixture() factoryFixture {
	at := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	lines := []domain.Line{
		domain.NewLine("v1", "Mug", 2, 1500, "USD"),
		domain.NewLine("v2", "Plate", 1, 2000, "USD"),
	}
	return factoryFixture{
		id:         "ord-1",
		userID:     "cart-1",
		customerID: "jane@example.com",
		shipTo: domain.RebuildAddress(
			"Jane", "1 Main St", "", "PDX", "97201", "USA",
		),
		shipMethod: domain.RebuildShippingMethod("courier", "Courier", 1500),
		payMethod:  domain.RebuildPaymentMethod("card", "Credit / debit card"),
		lines:      lines,
		// subtotal 5000, no discount, no tax, courier shipping 1500.
		quote: domain.Quote{
			Subtotal:     5000,
			Tax:          0,
			ShippingCost: 1500,
			Total:        6500,
		},
		channel: "web",
		at:      at,
	}
}

func (f factoryFixture) call() (*domain.Order, error) {
	return domain.NewOrderFactory().FromCart(
		f.id, f.userID, f.customerID,
		f.shipTo, f.shipMethod, f.payMethod,
		f.lines, f.quote, f.discountCode, f.channel, f.at,
	)
}

func TestOrderFactory_FromCart_RejectsEmptyCart(t *testing.T) {
	f := newFactoryFixture()
	f.lines = nil

	_, err := f.call()
	if !errors.Is(err, domain.ErrCartEmpty) {
		t.Fatalf("err = %v, want ErrCartEmpty", err)
	}
}

func TestOrderFactory_FromCart_RejectsMissingAddressForDeliveryMethod(t *testing.T) {
	f := newFactoryFixture()
	// courier requires an address; zero it out.
	f.shipTo = domain.Address{}

	_, err := f.call()
	if !errors.Is(err, domain.ErrAddressRequired) {
		t.Fatalf("err = %v, want ErrAddressRequired", err)
	}
}

// Pickup-style methods do NOT require an address; the same call with a
// zero Address must succeed when the method is "pickup".
func TestOrderFactory_FromCart_PickupAllowsZeroAddress(t *testing.T) {
	f := newFactoryFixture()
	f.shipMethod = domain.RebuildShippingMethod("pickup", "Personal pickup", 0)
	f.shipTo = domain.Address{}
	// Pickup is free — the quote's shipping is 0 to match.
	f.quote = domain.Quote{Subtotal: 5000, ShippingCost: 0, Total: 5000}

	if _, err := f.call(); err != nil {
		t.Fatalf("FromCart with pickup + zero address: err = %v", err)
	}
}

func TestOrderFactory_FromCart_RejectsMissingChannel(t *testing.T) {
	f := newFactoryFixture()
	f.channel = ""

	_, err := f.call()
	if !errors.Is(err, domain.ErrChannelRequired) {
		t.Fatalf("err = %v, want ErrChannelRequired", err)
	}
}

func TestOrderFactory_FromCart_RejectsMissingPaymentMethod(t *testing.T) {
	f := newFactoryFixture()
	f.payMethod = domain.PaymentMethod{}

	_, err := f.call()
	if !errors.Is(err, domain.ErrPaymentMethodRequired) {
		t.Fatalf("err = %v, want ErrPaymentMethodRequired", err)
	}
}

// Happy path: the resulting order is in StatusPending with exactly one
// pending event (OrderPlaced) and the quote's tax/shipping/discount are
// stamped onto the aggregate.
func TestOrderFactory_FromCart_HappyPath_PlacesPendingOrder(t *testing.T) {
	f := newFactoryFixture()
	// Quote with a real discount + tax so we can read each number back.
	// subtotal 5000, 10% promo discount = 500 → discounted subtotal 4500,
	// 8.875% tax on 4500 ≈ 399, courier shipping 1500, total 6399.
	f.discountCode = "SAVE10"
	f.quote = domain.Quote{
		Subtotal:       5000,
		DiscountAmount: 500,
		Tax:            399,
		ShippingCost:   1500,
		Total:          6399,
	}

	o, err := f.call()
	if err != nil {
		t.Fatalf("FromCart: %v", err)
	}
	if o.Status() != domain.StatusPending {
		t.Errorf("status = %q, want pending", o.Status())
	}
	pending := o.PendingEvents()
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
	if _, ok := pending[0].(domain.OrderPlaced); !ok {
		t.Errorf("pending[0] = %T, want OrderPlaced", pending[0])
	}
	if o.DiscountCode() != "SAVE10" || o.DiscountAmount() != 500 {
		t.Errorf("discount mismatch: code=%q amount=%d, want SAVE10/500",
			o.DiscountCode(), o.DiscountAmount())
	}
	if o.TaxAmount() != 399 {
		t.Errorf("tax = %d, want 399", o.TaxAmount())
	}
	if o.ShippingCost() != 1500 {
		t.Errorf("shipping = %d, want 1500", o.ShippingCost())
	}
	// total = subtotal - discount + tax + effectiveShipping = 5000 - 500 + 399 + 1500 = 6399.
	if o.TotalAmount() != 6399 {
		t.Errorf("total = %d, want 6399", o.TotalAmount())
	}
	if o.Channel() != "web" {
		t.Errorf("channel = %q, want web", o.Channel())
	}
}

// A Quote that already encodes free shipping (Tax=0, ShippingCost=0)
// must flow through to the aggregate unchanged — the factory does not
// second-guess the pricing service.
func TestOrderFactory_FromCart_FreeShippingQuote_StampsZeros(t *testing.T) {
	f := newFactoryFixture()
	// Pickup-style happy path: zero shipping, zero tax.
	f.shipMethod = domain.RebuildShippingMethod("pickup", "Personal pickup", 0)
	f.shipTo = domain.Address{}
	f.quote = domain.Quote{
		Subtotal:     5000,
		Tax:          0,
		ShippingCost: 0,
		Total:        5000,
		FreeShipping: true,
	}

	o, err := f.call()
	if err != nil {
		t.Fatalf("FromCart: %v", err)
	}
	if o.TaxAmount() != 0 {
		t.Errorf("tax = %d, want 0", o.TaxAmount())
	}
	if o.ShippingCost() != 0 {
		t.Errorf("shipping = %d, want 0 (free)", o.ShippingCost())
	}
	// total = subtotal - discount(0) + tax(0) + shipping(0) = 5000.
	if o.TotalAmount() != 5000 {
		t.Errorf("total = %d, want 5000", o.TotalAmount())
	}
}
