package domain

type Product struct {
	id    string
	price int
}

func (p Product) ID() string {
	return p.id
}

func NewProduct(id string, price int) Product {
	return Product{id: id, price: price}
}
