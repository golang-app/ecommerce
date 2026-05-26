package domain

import (
	"errors"
	"fmt"
)

var ErrCartNotFound = errors.New("cart: not found")

type Cart struct {
	cartItems map[string]CartItem
	user      User
}

type CartItem struct {
	product  Product
	quantity int
}

func (ci CartItem) Product() Product {
	return ci.product
}

func (ci CartItem) Quantity() int {
	return ci.quantity
}

func NewCart(user User) *Cart {
	return &Cart{
		cartItems: map[string]CartItem{},
		user:      user,
	}
}

func (c *Cart) User() User {
	return c.user
}

func (c *Cart) Add(product Product, quantity int) error {
	if _, ok := c.cartItems[product.ID()]; ok {
		if quantity+c.cartItems[product.ID()].Quantity() <= 0 {
			delete(c.cartItems, product.ID())
			return nil
		}

		ci := c.cartItems[product.ID()]
		ci.quantity += quantity
		c.cartItems[product.ID()] = ci

		return nil
	}

	if quantity <= 0 {
		return nil
	}

	if cur := c.currency(); cur != "" && cur != product.Price().Currency() {
		return fmt.Errorf("%w: cart is in %s, product is in %s", ErrCurrencyMismatch, cur, product.Price().Currency())
	}

	c.cartItems[product.ID()] = CartItem{
		product:  product,
		quantity: quantity,
	}

	return nil
}

// currency returns the cart's working currency, derived from its items.
// Returns the empty Currency when the cart is empty.
func (c *Cart) currency() Currency {
	for _, ci := range c.cartItems {
		return ci.product.Price().Currency()
	}
	return ""
}

func (c *Cart) Items() []CartItem {
	items := make([]CartItem, 0, len(c.cartItems))
	for _, ci := range c.cartItems {
		items = append(items, ci)
	}

	return items
}

func (c *Cart) Quantity(productID string) int {
	return c.cartItems[productID].quantity
}

func (c *Cart) TotalQuantity() int {
	total := 0
	for _, quantity := range c.cartItems {
		total += quantity.quantity
	}

	return total
}

// TotalPrice sums every line price in the cart. The cart's single-currency
// invariant (enforced in Add) guarantees this never errors at runtime.
func (c *Cart) TotalPrice() price {
	cur := c.currency()
	if cur == "" {
		return price{}
	}

	total := MustNewPrice(0, cur)
	for _, item := range c.cartItems {
		sum, _ := total.Add(item.product.Price().Multiple(item.Quantity()))
		total = sum
	}

	return total
}
