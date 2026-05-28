package adapter

import (
	"context"
	"sort"
	"strings"
	"time"

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
	// prodSets maps a product id to its assigned attribute set id (empty/absent
	// means no set).
	prodSets map[string]string
	// attribute sets: the set row plus its ordered member attribute type ids.
	attrSets     map[string]attrSetRow
	attrSetItems map[string][]string
	// movements is the in-memory inventory audit log, kept oldest-first; the
	// adapter slices/sorts it newest-first when returning.
	movements []domain.StockMovement
	// nextMovementID is the auto-increment cursor mirroring the postgres
	// bigserial column.
	nextMovementID int64
}

// attrSetRow holds an attribute set's own fields (members are tracked
// separately in attrSetItems, mirroring the postgres join table).
type attrSetRow struct {
	name     string
	position int
}

func NewInMemory() *inMemory {
	return &inMemory{
		optionTypes:  map[string][]domain.OptionType{},
		variants:     map[string][]domain.Variant{},
		attrTypes:    map[string]domain.AttributeType{},
		categories:   map[string]domain.Category{},
		prodAttrs:    map[string][]domain.AttributeValue{},
		prodCats:     map[string][]domain.Category{},
		prodSets:     map[string]string{},
		attrSets:     map[string]attrSetRow{},
		attrSetItems: map[string][]string{},
	}
}

func (im *inMemory) Add(ctx context.Context, p domain.Product) error {
	im.products = append(im.products, p)
	if setID := p.AttributeSetID(); setID != "" {
		im.prodSets[string(p.ID())] = setID
	}
	return nil
}

func (im *inMemory) AddOptionType(ctx context.Context, productID string, position int, ot domain.OptionType) error {
	im.optionTypes[productID] = append(im.optionTypes[productID], ot)
	return nil
}

// AddProductOptionType appends a new option type and seeds the chosen default
// value onto every existing variant's options map (keyed by name), mirroring
// the postgres transactional cascade.
func (im *inMemory) AddProductOptionType(ctx context.Context, productID, optionTypeID, name string, position int, values []string, variantDefault string) error {
	im.optionTypes[productID] = append(im.optionTypes[productID], domain.NewOptionType(name, values))
	for i, v := range im.variants[productID] {
		opts := cloneOptions(v.Options())
		opts[name] = variantDefault
		im.variants[productID][i] = domain.NewVariant(v.ID(), v.SKU(), v.Image(), opts, v.Price(), v.Stock())
	}
	return nil
}

// UpdateProductOptionType renames/re-values an option type and rekeys the
// option in every variant's options map when the name changes.
func (im *inMemory) UpdateProductOptionType(ctx context.Context, productID, currentName, newName string, values []string) error {
	ots := im.optionTypes[productID]
	for i, ot := range ots {
		if ot.Name() == currentName {
			ots[i] = domain.NewOptionType(newName, values)
			break
		}
	}
	if newName != currentName {
		for i, v := range im.variants[productID] {
			opts := cloneOptions(v.Options())
			if val, ok := opts[currentName]; ok {
				delete(opts, currentName)
				opts[newName] = val
				im.variants[productID][i] = domain.NewVariant(v.ID(), v.SKU(), v.Image(), opts, v.Price(), v.Stock())
			}
		}
	}
	return nil
}

// DeleteProductOptionType removes an option type and strips its key from every
// variant's options map.
func (im *inMemory) DeleteProductOptionType(ctx context.Context, productID, name string) error {
	ots := im.optionTypes[productID]
	for i, ot := range ots {
		if ot.Name() == name {
			im.optionTypes[productID] = append(ots[:i], ots[i+1:]...)
			break
		}
	}
	for i, v := range im.variants[productID] {
		if _, ok := v.Options()[name]; ok {
			opts := cloneOptions(v.Options())
			delete(opts, name)
			im.variants[productID][i] = domain.NewVariant(v.ID(), v.SKU(), v.Image(), opts, v.Price(), v.Stock())
		}
	}
	return nil
}

// cloneOptions returns a shallow copy of a variant's options map so mutations
// don't alias the original.
func cloneOptions(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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
		WithClassification(im.prodCats[string(p.ID())], im.prodAttrs[string(p.ID())]).
		WithAttributeSet(im.prodSets[string(p.ID())])
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
	delete(im.prodSets, id)
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

// SetProductAttributeSet sets (or clears, when setID is empty) the product's
// attribute set id.
func (im *inMemory) SetProductAttributeSet(ctx context.Context, productID, setID string) error {
	if setID == "" {
		delete(im.prodSets, productID)
		return nil
	}
	im.prodSets[productID] = setID
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

// attributeSetMembers resolves a set's member ids to their attribute types in
// the stored order, skipping any that no longer exist.
func (im *inMemory) attributeSetMembers(setID string) []domain.AttributeType {
	ids := im.attrSetItems[setID]
	out := make([]domain.AttributeType, 0, len(ids))
	for _, id := range ids {
		if t, ok := im.attrTypes[id]; ok {
			out = append(out, t)
		}
	}
	return out
}

func (im *inMemory) AllAttributeSets(ctx context.Context) ([]domain.AttributeSet, error) {
	out := make([]domain.AttributeSet, 0, len(im.attrSets))
	for id, row := range im.attrSets {
		out = append(out, domain.RebuildAttributeSet(id, row.name, row.position, im.attributeSetMembers(id)))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position() != out[j].Position() {
			return out[i].Position() < out[j].Position()
		}
		return out[i].Name() < out[j].Name()
	})
	return out, nil
}

func (im *inMemory) FindAttributeSet(ctx context.Context, id string) (domain.AttributeSet, error) {
	row, ok := im.attrSets[id]
	if !ok {
		return domain.AttributeSet{}, domain.ErrAttributeSetNotFound
	}
	return domain.RebuildAttributeSet(id, row.name, row.position, im.attributeSetMembers(id)), nil
}

func (im *inMemory) CreateAttributeSet(ctx context.Context, s domain.AttributeSet) error {
	im.attrSets[s.ID()] = attrSetRow{name: s.Name(), position: s.Position()}
	return nil
}

func (im *inMemory) UpdateAttributeSet(ctx context.Context, s domain.AttributeSet) error {
	im.attrSets[s.ID()] = attrSetRow{name: s.Name(), position: s.Position()}
	return nil
}

func (im *inMemory) DeleteAttributeSet(ctx context.Context, id string) error {
	delete(im.attrSets, id)
	delete(im.attrSetItems, id)
	return nil
}

// SetAttributeSetItems replaces a set's member ids, preserving the given order.
func (im *inMemory) SetAttributeSetItems(ctx context.Context, setID string, attributeTypeIDs []string) error {
	items := make([]string, len(attributeTypeIDs))
	copy(items, attributeTypeIDs)
	im.attrSetItems[setID] = items
	return nil
}

// InsertStockMovement appends an audit-log entry. IDs are assigned in
// insertion order, mirroring the postgres bigserial column.
func (im *inMemory) InsertStockMovement(ctx context.Context, variantID string, delta int, reason, refOrderID string) error {
	im.nextMovementID++
	im.movements = append(im.movements, domain.NewStockMovement(im.nextMovementID, variantID, delta, reason, refOrderID, time.Now().UTC()))
	return nil
}

// ListStockMovements returns up to limit movements newest-first; when
// variantID is empty the full log is returned.
func (im *inMemory) ListStockMovements(ctx context.Context, variantID string, limit int) ([]domain.StockMovement, error) {
	filtered := make([]domain.StockMovement, 0, len(im.movements))
	for i := len(im.movements) - 1; i >= 0; i-- {
		m := im.movements[i]
		if variantID != "" && m.VariantID != variantID {
			continue
		}
		filtered = append(filtered, m)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered, nil
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
	search := strings.ToLower(strings.TrimSpace(q.Search))
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
		if search != "" &&
			!strings.Contains(strings.ToLower(p.Name()), search) &&
			!strings.Contains(strings.ToLower(p.Description()), search) {
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
