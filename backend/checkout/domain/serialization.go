package domain

import "encoding/json"

// Value objects keep unexported fields, so they need explicit JSON
// marshaling to live inside persisted events. Each marshals to a small
// exported DTO and rebuilds through its (non-validating) Rebuild* constructor
// — the data was already valid when the event was first written.

type addressDTO struct {
	Name    string `json:"name"`
	Street1 string `json:"street1"`
	Street2 string `json:"street2"`
	City    string `json:"city"`
	Zip     string `json:"zip"`
	Country string `json:"country"`
}

func (a Address) MarshalJSON() ([]byte, error) {
	return json.Marshal(addressDTO{a.name, a.street1, a.street2, a.city, a.zip, a.country})
}

func (a *Address) UnmarshalJSON(b []byte) error {
	var d addressDTO
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	*a = RebuildAddress(d.Name, d.Street1, d.Street2, d.City, d.Zip, d.Country)
	return nil
}

type shippingMethodDTO struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Cost  int64  `json:"cost"`
}

func (m ShippingMethod) MarshalJSON() ([]byte, error) {
	return json.Marshal(shippingMethodDTO{m.code, m.label, m.cost})
}

func (m *ShippingMethod) UnmarshalJSON(b []byte) error {
	var d shippingMethodDTO
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	*m = RebuildShippingMethod(d.Code, d.Label, d.Cost)
	return nil
}

type paymentMethodDTO struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

func (m PaymentMethod) MarshalJSON() ([]byte, error) {
	return json.Marshal(paymentMethodDTO{m.code, m.label})
}

func (m *PaymentMethod) UnmarshalJSON(b []byte) error {
	var d paymentMethodDTO
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	*m = RebuildPaymentMethod(d.Code, d.Label)
	return nil
}

type lineDTO struct {
	ProductID     string `json:"product_id"`
	ProductName   string `json:"product_name"`
	Qty           int    `json:"qty"`
	PriceAmount   int64  `json:"price_amount"`
	PriceCurrency string `json:"price_currency"`
}

func (l Line) MarshalJSON() ([]byte, error) {
	return json.Marshal(lineDTO{l.productID, l.productName, l.qty, l.priceAmount, l.priceCurrency})
}

func (l *Line) UnmarshalJSON(b []byte) error {
	var d lineDTO
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	*l = NewLine(d.ProductID, d.ProductName, d.Qty, d.PriceAmount, d.PriceCurrency)
	return nil
}
