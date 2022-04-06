package domain

type Cart struct {
	products map[string]cartItem
	ps       priceService
}

type cartItem struct {
	product  Product
	quantity float64
}

func NewCart(ps priceService) *Cart {
	return &Cart{
		products: map[string]cartItem{},
		ps:       ps,
	}
}

type priceService interface {
	PriceFor(productID string) (float64, error)
}

func (c *Cart) Add(product Product, quantity float64) error {
	if _, ok := c.products[product.ID()]; ok {
		ci := c.products[product.ID()]
		ci.quantity += quantity
		c.products[product.ID()] = ci
	} else {
		c.products[product.ID()] = cartItem{
			product:  product,
			quantity: quantity,
		}
	}
	return nil
}

func (c *Cart) Quantity(productID string) float64 {
	return c.products[productID].quantity
}

func (c *Cart) TotalQuantity() float64 {
	total := 0.0
	for _, quantity := range c.products {
		total += quantity.quantity
	}

	return total
}

func (c *Cart) TotalPrice() (float64, error) {
	total := 0.0
	for _, ci := range c.products {
		total += float64(ci.product.price) * ci.quantity
	}

	return total, nil
}
