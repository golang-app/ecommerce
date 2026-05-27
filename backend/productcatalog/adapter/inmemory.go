package adapter

import (
	"context"
	"sort"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

type inMemory struct {
	products    []domain.Product
	optionTypes map[string][]domain.OptionType
	variants    map[string][]domain.Variant
	// classification state
	attrTypes  map[string]domain.AttributeType
	categories map[string]domain.Category
	prodAttrs  map[string][]domain.AttributeValue
	prodCats   map[string][]domain.Category
}

func NewInMemory() *inMemory {
	return &inMemory{
		optionTypes: map[string][]domain.OptionType{},
		variants:    map[string][]domain.Variant{},
		attrTypes:   map[string]domain.AttributeType{},
		categories:  map[string]domain.Category{},
		prodAttrs:   map[string][]domain.AttributeValue{},
		prodCats:    map[string][]domain.Category{},
	}
}

func (im *inMemory) Add(ctx context.Context, p domain.Product) error {
	im.products = append(im.products, p)
	return nil
}

func (im *inMemory) AddOptionType(ctx context.Context, productID string, position int, ot domain.OptionType) error {
	im.optionTypes[productID] = append(im.optionTypes[productID], ot)
	return nil
}

func (im *inMemory) AddVariant(ctx context.Context, productID string, position int, v domain.Variant) error {
	im.variants[productID] = append(im.variants[productID], v)
	return nil
}

// UpdateVariant updates a single variant's sku, image, price and stock by id.
func (im *inMemory) UpdateVariant(ctx context.Context, variantID, sku, image string, priceAmount int64, currency string, stock int) error {
	pid, i, ok := im.find(variantID)
	if !ok {
		return domain.ErrProductNotFound
	}
	cur, err := domain.NewCurrency(currency)
	if err != nil {
		return err
	}
	price, err := domain.NewPrice(priceAmount, cur)
	if err != nil {
		return err
	}
	v := im.variants[pid][i]
	im.variants[pid][i] = domain.NewVariant(v.ID(), sku, image, v.Options(), price, stock)
	return nil
}

// DeleteVariant removes a single variant by id.
func (im *inMemory) DeleteVariant(ctx context.Context, variantID string) error {
	pid, i, ok := im.find(variantID)
	if !ok {
		return domain.ErrProductNotFound
	}
	im.variants[pid] = append(im.variants[pid][:i], im.variants[pid][i+1:]...)
	return nil
}

func (im *inMemory) hydrate(p domain.Product) domain.Product {
	return p.WithCatalog(im.optionTypes[string(p.ID())], im.variants[string(p.ID())]).
		WithClassification(im.prodCats[string(p.ID())], im.prodAttrs[string(p.ID())])
}

func (im *inMemory) All(ctx context.Context) ([]domain.Product, error) {
	out := make([]domain.Product, 0, len(im.products))
	for _, p := range im.products {
		out = append(out, im.hydrate(p))
	}
	return out, nil
}

// Newest returns up to limit products in insertion order (there is no
// created_at in the in-memory store), hydrated like All.
func (im *inMemory) Newest(ctx context.Context, limit int) ([]domain.Product, error) {
	if limit > len(im.products) {
		limit = len(im.products)
	}
	out := make([]domain.Product, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, im.hydrate(im.products[i]))
	}
	return out, nil
}

func (im *inMemory) Find(ctx context.Context, id string) (domain.Product, error) {
	for _, p := range im.products {
		if string(p.ID()) == id {
			return im.hydrate(p), nil
		}
	}
	return domain.Product{}, domain.ErrProductNotFound
}

// UpdateProduct replaces the core product fields (name/description/price/
// thumbnail) for the matching id, preserving its option types/variants and
// classification (held in separate maps).
func (im *inMemory) UpdateProduct(ctx context.Context, p domain.Product) error {
	for i, existing := range im.products {
		if existing.ID() == p.ID() {
			im.products[i] = p
			return nil
		}
	}
	return domain.ErrProductNotFound
}

// DeleteProduct removes a product and its associated option types, variants and
// classification (mirroring the postgres ON DELETE CASCADE behaviour).
func (im *inMemory) DeleteProduct(ctx context.Context, id string) error {
	for i, existing := range im.products {
		if string(existing.ID()) == id {
			im.products = append(im.products[:i], im.products[i+1:]...)
			break
		}
	}
	delete(im.optionTypes, id)
	delete(im.variants, id)
	delete(im.prodCats, id)
	delete(im.prodAttrs, id)
	return nil
}

// SetVariantStock sets a single variant's stock, wherever it lives.
func (im *inMemory) SetVariantStock(ctx context.Context, variantID string, stock int) error {
	pid, i, ok := im.find(variantID)
	if !ok {
		return domain.ErrProductNotFound
	}
	v := im.variants[pid][i]
	im.variants[pid][i] = domain.NewVariant(v.ID(), v.SKU(), v.Image(), v.Options(), v.Price(), stock)
	return nil
}

// SetProductCategories replaces the product's category links with the given set.
func (im *inMemory) SetProductCategories(ctx context.Context, productID string, categoryIDs []string) error {
	cats := make([]domain.Category, 0, len(categoryIDs))
	for _, cid := range categoryIDs {
		if c, ok := im.categories[cid]; ok {
			cats = append(cats, c)
		}
	}
	im.prodCats[productID] = cats
	return nil
}

// SetProductAttributes replaces the product's attribute values with the given set.
func (im *inMemory) SetProductAttributes(ctx context.Context, productID string, values []app.AttributeAssignment) error {
	out := make([]domain.AttributeValue, 0, len(values))
	for _, v := range values {
		t, ok := im.attrTypes[v.TypeID]
		if !ok {
			continue
		}
		if v.Num != nil {
			out = append(out, domain.NewNumericValue(t, *v.Num))
		} else {
			out = append(out, domain.NewEnumValue(t, v.Text))
		}
	}
	im.prodAttrs[productID] = out
	return nil
}

func (im *inMemory) find(variantID string) (string, int, bool) {
	for pid, vs := range im.variants {
		for i, v := range vs {
			if v.ID() == variantID {
				return pid, i, true
			}
		}
	}
	return "", 0, false
}

// Reserve checks availability for all variants first, then decrements — so an
// insufficient item leaves everything untouched.
func (im *inMemory) Reserve(ctx context.Context, quantities map[string]int) error {
	for id, qty := range quantities {
		pid, i, ok := im.find(id)
		if !ok || im.variants[pid][i].Stock() < qty {
			return domain.ErrInsufficientStock
		}
	}
	for id, qty := range quantities {
		pid, i, _ := im.find(id)
		v := im.variants[pid][i]
		im.variants[pid][i] = domain.NewVariant(v.ID(), v.SKU(), v.Image(), v.Options(), v.Price(), v.Stock()-qty)
	}
	return nil
}

func (im *inMemory) Release(ctx context.Context, quantities map[string]int) error {
	for id, qty := range quantities {
		if pid, i, ok := im.find(id); ok {
			v := im.variants[pid][i]
			im.variants[pid][i] = domain.NewVariant(v.ID(), v.SKU(), v.Image(), v.Options(), v.Price(), v.Stock()+qty)
		}
	}
	return nil
}

func (im *inMemory) FindVariant(ctx context.Context, variantID string) (domain.Product, domain.Variant, error) {
	for _, p := range im.products {
		full := im.hydrate(p)
		if v, ok := full.Variant(variantID); ok {
			return full, v, nil
		}
	}
	return domain.Product{}, domain.Variant{}, domain.ErrProductNotFound
}

// AddAttributeType registers a predefined attribute type. Test/seed helper for
// the in-memory store (the postgres store loads these from the DB).
func (im *inMemory) AddAttributeType(t domain.AttributeType) {
	im.attrTypes[t.ID()] = t
}

// AddCategory registers a category. Test/seed helper for the in-memory store.
func (im *inMemory) AddCategory(c domain.Category) {
	im.categories[c.ID()] = c
}

// SeedProductAttributes attaches attribute values to a product. Test/seed helper.
func (im *inMemory) SeedProductAttributes(productID string, values ...domain.AttributeValue) {
	im.prodAttrs[productID] = append(im.prodAttrs[productID], values...)
}

// SeedProductCategories attaches categories to a product. Test/seed helper.
func (im *inMemory) SeedProductCategories(productID string, cats ...domain.Category) {
	im.prodCats[productID] = append(im.prodCats[productID], cats...)
}

func (im *inMemory) Categories(ctx context.Context) ([]domain.Category, error) {
	out := make([]domain.Category, 0, len(im.categories))
	for _, c := range im.categories {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position() != out[j].Position() {
			return out[i].Position() < out[j].Position()
		}
		return out[i].Name() < out[j].Name()
	})
	return out, nil
}

func (im *inMemory) CreateCategory(ctx context.Context, c domain.Category) error {
	im.categories[c.ID()] = c
	return nil
}

func (im *inMemory) UpdateCategory(ctx context.Context, c domain.Category) error {
	im.categories[c.ID()] = c
	return nil
}

func (im *inMemory) DeleteCategory(ctx context.Context, id string) error {
	delete(im.categories, id)
	return nil
}

func (im *inMemory) AllAttributeTypes(ctx context.Context) ([]domain.AttributeType, error) {
	out := make([]domain.AttributeType, 0, len(im.attrTypes))
	for _, t := range im.attrTypes {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position() != out[j].Position() {
			return out[i].Position() < out[j].Position()
		}
		return out[i].Name() < out[j].Name()
	})
	return out, nil
}

func (im *inMemory) CreateAttributeType(ctx context.Context, t domain.AttributeType) error {
	im.attrTypes[t.ID()] = t
	return nil
}

func (im *inMemory) UpdateAttributeType(ctx context.Context, t domain.AttributeType) error {
	im.attrTypes[t.ID()] = t
	return nil
}

func (im *inMemory) DeleteAttributeType(ctx context.Context, id string) error {
	delete(im.attrTypes, id)
	return nil
}

func (im *inMemory) inCategory(productID, slug string) bool {
	for _, c := range im.prodCats[productID] {
		if c.Slug() == slug {
			return true
		}
	}
	return false
}

func (im *inMemory) ListProducts(ctx context.Context, q app.ProductQuery) ([]domain.Product, error) {
	var out []domain.Product
	for _, p := range im.products {
		pid := string(p.ID())
		if q.CategorySlug != "" && !im.inCategory(pid, q.CategorySlug) {
			continue
		}
		if !im.matchesNumeric(pid, q.NumericRanges) {
			continue
		}
		if !im.matchesEnum(pid, q.EnumSelections) {
			continue
		}
		out = append(out, im.hydrate(p))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

func (im *inMemory) matchesNumeric(productID string, ranges map[string]app.Range) bool {
	for typeID, r := range ranges {
		ok := false
		for _, av := range im.prodAttrs[productID] {
			if av.Type().ID() != typeID || !av.Type().IsNumeric() {
				continue
			}
			v := av.NumValue()
			if r.Min != nil && v < *r.Min {
				continue
			}
			if r.Max != nil && v > *r.Max {
				continue
			}
			ok = true
			break
		}
		if !ok {
			return false
		}
	}
	return true
}

func (im *inMemory) matchesEnum(productID string, selections map[string][]string) bool {
	for typeID, values := range selections {
		if len(values) == 0 {
			continue
		}
		ok := false
		for _, av := range im.prodAttrs[productID] {
			if av.Type().ID() != typeID || !av.Type().IsEnum() {
				continue
			}
			for _, want := range values {
				if av.TextValue() == want {
					ok = true
					break
				}
			}
			if ok {
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func (im *inMemory) Facets(ctx context.Context, categorySlug string) ([]app.Facet, error) {
	types := make([]domain.AttributeType, 0, len(im.attrTypes))
	for _, t := range im.attrTypes {
		if t.Filterable() {
			types = append(types, t)
		}
	}
	sort.Slice(types, func(i, j int) bool {
		if types[i].Position() != types[j].Position() {
			return types[i].Position() < types[j].Position()
		}
		return types[i].Name() < types[j].Name()
	})

	var facets []app.Facet
	for _, t := range types {
		if t.IsNumeric() {
			var min, max float64
			found := false
			for pid, attrs := range im.prodAttrs {
				if categorySlug != "" && !im.inCategory(pid, categorySlug) {
					continue
				}
				for _, av := range attrs {
					if av.Type().ID() != t.ID() || !av.Type().IsNumeric() {
						continue
					}
					v := av.NumValue()
					if !found || v < min {
						min = v
					}
					if !found || v > max {
						max = v
					}
					found = true
				}
			}
			if !found {
				continue
			}
			lo, hi := min, max
			facets = append(facets, app.Facet{Type: t, Min: &lo, Max: &hi})
			continue
		}

		set := map[string]struct{}{}
		for pid, attrs := range im.prodAttrs {
			if categorySlug != "" && !im.inCategory(pid, categorySlug) {
				continue
			}
			for _, av := range attrs {
				if av.Type().ID() != t.ID() || !av.Type().IsEnum() {
					continue
				}
				if av.TextValue() != "" {
					set[av.TextValue()] = struct{}{}
				}
			}
		}
		if len(set) == 0 {
			continue
		}
		values := make([]string, 0, len(set))
		for v := range set {
			values = append(values, v)
		}
		sort.Strings(values)
		facets = append(facets, app.Facet{Type: t, Values: values})
	}
	return facets, nil
}
