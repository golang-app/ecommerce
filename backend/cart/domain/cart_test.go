package domain_test

import (
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/matryer/is"
)

var pID = domain.NewProduct("productID", "test product", 200, domain.MustNewCurrency("PLN"))
var pID2 = domain.NewProduct("productID2", "test product", 1000, domain.MustNewCurrency("PLN"))
var pUSD = domain.NewProduct("productUSD", "usd product", 500, domain.MustNewCurrency("USD"))

func TestCart_Adding_Products_To_Cart_Should_Change_Quantity(t *testing.T) {
	is := is.New(t)
	// given
	c := domain.NewCart(domain.NewUser(""))

	// when
	_ = c.Add(pID, 3)
	_ = c.Add(pID, 1)

	// then
	is.Equal(4, c.Quantity(pID.ID()))
}

func TestCart_Adding_Products_To_Cart_Should_Change_Quantity_Of_Single_Product(t *testing.T) {
	is := is.New(t)
	// given
	c := domain.NewCart(domain.NewUser(""))
	_ = c.Add(pID, 1)

	// when
	_ = c.Add(pID2, 3)

	// then
	is.Equal(1, c.Quantity(pID.ID()))
	is.Equal(3, c.Quantity(pID2.ID()))
}

func TestCart_CalculateTotalPrice(t *testing.T) {
	is := is.New(t)
	// given
	c := domain.NewCart(domain.NewUser(""))

	// when
	_ = c.Add(pID, 1)
	_ = c.Add(pID2, 3)

	// then
	is.Equal(4, c.TotalQuantity())

	// total = 1 * 200 + 3 * 1000 = 3200 PLN-minor
	total := c.TotalPrice()
	is.Equal(int64(3200), total.Amount())
	is.Equal(domain.MustNewCurrency("PLN"), total.Currency())
}

func TestCart_RejectsCrossCurrencyItem(t *testing.T) {
	is := is.New(t)
	// given a cart with a PLN item
	c := domain.NewCart(domain.NewUser(""))
	is.NoErr(c.Add(pID, 1))

	// when adding a USD item
	err := c.Add(pUSD, 1)

	// then it must fail with ErrCurrencyMismatch and not mutate the cart
	is.True(errors.Is(err, domain.ErrCurrencyMismatch))
	is.Equal(1, c.TotalQuantity())
}

func TestCart_EmptyTotalPrice(t *testing.T) {
	is := is.New(t)
	c := domain.NewCart(domain.NewUser(""))

	total := c.TotalPrice()
	is.Equal(int64(0), total.Amount())
	is.Equal(domain.Currency(""), total.Currency())
}
