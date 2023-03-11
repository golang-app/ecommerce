package domain_test

import (
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/matryer/is"
)

var pID = domain.NewProduct("productID", "test product", 2.0, "PLN")
var pID2 = domain.NewProduct("productID2", "test product", 10.0, "PLN")

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
}
