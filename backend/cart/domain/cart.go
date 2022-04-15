package domain

type Cart struct {
	cartItems map[string]cartItem
	ps        priceService
}

type cartItem struct {
	product  Product
	quantity int
}

func (ci cartItem) Product() Product {
	return ci.product
}

func (ci cartItem) Quantity() int {
	return ci.quantity
}

func NewCart(ps priceService) *Cart {
	return &Cart{
		cartItems: map[string]cartItem{},
		ps:        ps,
	}
}

type priceService interface {
	PriceFor(productID string) (float64, error)
}

func (c *Cart) Add(product Product, quantity int) error {
	if _, ok := c.cartItems[product.ID()]; ok {
		ci := c.cartItems[product.ID()]
		ci.quantity += quantity
		c.cartItems[product.ID()] = ci
	} else {
		c.cartItems[product.ID()] = cartItem{
			product:  product,
			quantity: quantity,
		}
	}
	return nil
}

func (c *Cart) Items() map[string]cartItem {
	return c.cartItems
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

func (c *Cart) TotalPrice() (float64, error) {
	total := 0.0
	for _, ci := range c.cartItems {
		total += ci.product.price * float64(ci.quantity)
	}

	return total, nil
}
