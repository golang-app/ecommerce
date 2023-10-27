package domain

import (
	"errors"
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

	c.cartItems[product.ID()] = CartItem{
		product:  product,
		quantity: quantity,
	}

	return nil
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

func (c *Cart) TotalPrice() price {
	// TODO: add support for multiple currencies

	total := NewPrice(0, "USD")
	for _, item := range c.cartItems {
		total = total.Add(item.product.Price().Multiple(item.Quantity()))
	}

	return total
}
