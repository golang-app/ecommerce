package domain

type Product struct {
	id    string
	price float64
}

func NewProduct(id string, price float64) Product {
	return Product{id: id, price: price}
}

func (p Product) ID() string {
	return p.id
}

func (p Product) Price() float64 {
	return p.price
}
