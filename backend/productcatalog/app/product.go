package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

type ProductService struct {
	storage ProductStorage
}

type ProductStorage interface {
	All(ctx context.Context) ([]domain.Product, error)
	Newest(ctx context.Context, limit int) ([]domain.Product, error)
	Add(ctx context.Context, p domain.Product) error
	UpdateProduct(ctx context.Context, p domain.Product) error
	DeleteProduct(ctx context.Context, id string) error
	SetVariantStock(ctx context.Context, variantID string, stock int) error
	SetProductCategories(ctx context.Context, productID string, categoryIDs []string) error
	SetProductAttributes(ctx context.Context, productID string, values []AttributeAssignment) error
	SetProductAttributeSet(ctx context.Context, productID, setID string) error
	Find(ctx context.Context, id string) (domain.Product, error)
	FindVariant(ctx context.Context, variantID string) (domain.Product, domain.Variant, error)
	AddOptionType(ctx context.Context, productID string, position int, ot domain.OptionType) error
	AddProductOptionType(ctx context.Context, productID, optionTypeID, name string, position int, values []string, variantDefault string) error
	UpdateProductOptionType(ctx context.Context, productID, currentName, newName string, values []string) error
	DeleteProductOptionType(ctx context.Context, productID, name string) error
	AddVariant(ctx context.Context, productID string, position int, v domain.Variant) error
	UpdateVariant(ctx context.Context, variantID, sku, image string, priceAmount int64, currency string, stock int) error
	DeleteVariant(ctx context.Context, variantID string) error
	Reserve(ctx context.Context, quantities map[string]int) error
	Release(ctx context.Context, quantities map[string]int) error
	ListProducts(ctx context.Context, q ProductQuery) ([]domain.Product, error)
	Categories(ctx context.Context) ([]domain.Category, error)
	Facets(ctx context.Context, categorySlug string) ([]Facet, error)

	CreateCategory(ctx context.Context, c domain.Category) error
	UpdateCategory(ctx context.Context, c domain.Category) error
	DeleteCategory(ctx context.Context, id string) error

	AllAttributeTypes(ctx context.Context) ([]domain.AttributeType, error)
	CreateAttributeType(ctx context.Context, t domain.AttributeType) error
	UpdateAttributeType(ctx context.Context, t domain.AttributeType) error
	DeleteAttributeType(ctx context.Context, id string) error

	AllAttributeSets(ctx context.Context) ([]domain.AttributeSet, error)
	FindAttributeSet(ctx context.Context, id string) (domain.AttributeSet, error)
	CreateAttributeSet(ctx context.Context, s domain.AttributeSet) error
	UpdateAttributeSet(ctx context.Context, s domain.AttributeSet) error
	DeleteAttributeSet(ctx context.Context, id string) error
	SetAttributeSetItems(ctx context.Context, setID string, attributeTypeIDs []string) error
}

func NewProductService(s ProductStorage) ProductService {
	return ProductService{storage: s}
}

func (ps ProductService) AllProducts(ctx context.Context) ([]domain.Product, error) {
	return ps.storage.All(ctx)
}

// defaultNewestLimit is used when Newest is called with a non-positive limit.
const defaultNewestLimit = 8

// Newest returns up to limit most-recently-created products ("new arrivals").
// A non-positive limit falls back to defaultNewestLimit.
func (ps ProductService) Newest(ctx context.Context, limit int) ([]domain.Product, error) {
	if limit <= 0 {
		limit = defaultNewestLimit
	}
	return ps.storage.Newest(ctx, limit)
}

func (ps ProductService) Find(ctx context.Context, id string) (domain.Product, error) {
	return ps.storage.Find(ctx, id)
}

// FindVariant resolves a variant id to its variant and owning product.
func (ps ProductService) FindVariant(ctx context.Context, variantID string) (domain.Product, domain.Variant, error) {
	return ps.storage.FindVariant(ctx, variantID)
}

// Reserve atomically decrements stock for the given variants (variant id ->
// quantity). All-or-nothing: returns ErrInsufficientStock if any variant is
// short, leaving stock untouched.
func (ps ProductService) Reserve(ctx context.Context, quantities map[string]int) error {
	return ps.storage.Reserve(ctx, quantities)
}

// Release returns previously-reserved stock (e.g. after a failed payment).
func (ps ProductService) Release(ctx context.Context, quantities map[string]int) error {
	return ps.storage.Release(ctx, quantities)
}

// List returns the products matching the given listing-page query.
func (ps ProductService) List(ctx context.Context, q ProductQuery) ([]domain.Product, error) {
	return ps.storage.ListProducts(ctx, q)
}

// Categories returns all catalog categories in display order.
func (ps ProductService) Categories(ctx context.Context) ([]domain.Category, error) {
	return ps.storage.Categories(ctx)
}

// Facets returns the available filter facets, optionally scoped to a category.
func (ps ProductService) Facets(ctx context.Context, categorySlug string) ([]Facet, error) {
	return ps.storage.Facets(ctx, categorySlug)
}

// CreateCategory validates and persists a new category. The id equals the slug
// and the position is appended after the current categories.
func (ps ProductService) CreateCategory(ctx context.Context, name, slug string) error {
	existing, err := ps.storage.Categories(ctx)
	if err != nil {
		return err
	}
	c, err := domain.NewCategory(slug, name, slug, len(existing)+1)
	if err != nil {
		return err
	}
	return ps.storage.CreateCategory(ctx, c)
}

// UpdateCategory validates and persists changes to an existing category.
func (ps ProductService) UpdateCategory(ctx context.Context, id, name, slug string, position int) error {
	c, err := domain.NewCategory(id, name, slug, position)
	if err != nil {
		return err
	}
	return ps.storage.UpdateCategory(ctx, c)
}

// DeleteCategory removes a category (its product links cascade in storage).
func (ps ProductService) DeleteCategory(ctx context.Context, id string) error {
	return ps.storage.DeleteCategory(ctx, id)
}

// AttributeTypes returns every attribute type in display order (admin view).
func (ps ProductService) AttributeTypes(ctx context.Context) ([]domain.AttributeType, error) {
	return ps.storage.AllAttributeTypes(ctx)
}

// AllAttributeTypes returns every attribute type in display order. It is an
// alias of AttributeTypes exposed for callers (e.g. the attribute-set editor)
// that need the full list of types to choose from.
func (ps ProductService) AllAttributeTypes(ctx context.Context) ([]domain.AttributeType, error) {
	return ps.storage.AllAttributeTypes(ctx)
}

// CreateAttributeType validates and persists a new attribute type. The id is a
// slug derived from the name and the position is appended after existing types.
func (ps ProductService) CreateAttributeType(ctx context.Context, name, unit string, kind domain.AttributeKind, filterable bool) error {
	existing, err := ps.storage.AllAttributeTypes(ctx)
	if err != nil {
		return err
	}
	id := slugify(name)
	t, err := domain.NewAttributeType(id, name, unit, kind, filterable, len(existing)+1)
	if err != nil {
		return err
	}
	return ps.storage.CreateAttributeType(ctx, t)
}

// UpdateAttributeType validates and persists changes to an attribute type.
func (ps ProductService) UpdateAttributeType(ctx context.Context, id, name, unit string, kind domain.AttributeKind, filterable bool, position int) error {
	t, err := domain.NewAttributeType(id, name, unit, kind, filterable, position)
	if err != nil {
		return err
	}
	return ps.storage.UpdateAttributeType(ctx, t)
}

// DeleteAttributeType removes an attribute type (its product links cascade).
func (ps ProductService) DeleteAttributeType(ctx context.Context, id string) error {
	return ps.storage.DeleteAttributeType(ctx, id)
}

// AttributeSets returns every attribute set in display order, each hydrated
// with its ordered members.
func (ps ProductService) AttributeSets(ctx context.Context) ([]domain.AttributeSet, error) {
	return ps.storage.AllAttributeSets(ctx)
}

// FindAttributeSet returns a single attribute set (with its members) by id.
func (ps ProductService) FindAttributeSet(ctx context.Context, id string) (domain.AttributeSet, error) {
	return ps.storage.FindAttributeSet(ctx, id)
}

// CreateAttributeSet validates and persists a new attribute set. The id is a
// slug derived from the name, the position is appended after existing sets, and
// the given attribute type ids become its ordered members (order = slice
// order). Unknown attribute type ids are rejected.
func (ps ProductService) CreateAttributeSet(ctx context.Context, name string, attributeTypeIDs []string) error {
	existing, err := ps.storage.AllAttributeSets(ctx)
	if err != nil {
		return err
	}
	id := slugify(name)
	if id == "" {
		return fmt.Errorf("%w: name must contain letters or digits", domain.ErrInvalidAttributeSet)
	}
	ids, err := ps.validateAttributeTypeIDs(ctx, attributeTypeIDs)
	if err != nil {
		return err
	}
	set, err := domain.NewAttributeSet(id, name, len(existing)+1)
	if err != nil {
		return err
	}
	if err = ps.storage.CreateAttributeSet(ctx, set); err != nil {
		return err
	}
	return ps.storage.SetAttributeSetItems(ctx, id, ids)
}

// UpdateAttributeSet renames an existing set and replaces its members with the
// given ordered attribute type ids (order = slice order). Unknown attribute
// type ids are rejected.
func (ps ProductService) UpdateAttributeSet(ctx context.Context, id, name string, attributeTypeIDs []string) error {
	current, err := ps.storage.FindAttributeSet(ctx, id)
	if err != nil {
		return err
	}
	ids, err := ps.validateAttributeTypeIDs(ctx, attributeTypeIDs)
	if err != nil {
		return err
	}
	set, err := domain.NewAttributeSet(id, name, current.Position())
	if err != nil {
		return err
	}
	if err = ps.storage.UpdateAttributeSet(ctx, set); err != nil {
		return err
	}
	return ps.storage.SetAttributeSetItems(ctx, id, ids)
}

// DeleteAttributeSet removes an attribute set (its member items cascade).
func (ps ProductService) DeleteAttributeSet(ctx context.Context, id string) error {
	return ps.storage.DeleteAttributeSet(ctx, id)
}

// validateAttributeTypeIDs checks every given id exists among the known
// attribute types (rejecting unknown ids) while preserving the input order.
func (ps ProductService) validateAttributeTypeIDs(ctx context.Context, attributeTypeIDs []string) ([]string, error) {
	types, err := ps.storage.AllAttributeTypes(ctx)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(types))
	for _, t := range types {
		known[t.ID()] = true
	}
	out := make([]string, 0, len(attributeTypeIDs))
	for _, id := range attributeTypeIDs {
		if !known[id] {
			return nil, fmt.Errorf("unknown attribute type %q", id)
		}
		out = append(out, id)
	}
	return out, nil
}

// slugify turns a display name into a url-safe id: lowercase, spaces to
// hyphens, dropping any character that is not a lowercase letter, digit or
// hyphen, and collapsing repeated/edge hyphens.
func slugify(name string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case r == ' ' || r == '-' || r == '_':
			if !lastHyphen && b.Len() > 0 {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// defaultStock is given to a simple product's auto-created default variant.
const defaultStock = 100

func (ps ProductService) Add(ctx context.Context, id, name, desc string, priceMinorUnits int64, currency, thumbnail string) error {
	pId, err := domain.NewProductId(id)
	if err != nil {
		return err
	}

	cur, err := domain.NewCurrency(currency)
	if err != nil {
		return fmt.Errorf("invalid currency: %w", err)
	}

	priceVO, err := domain.NewPrice(priceMinorUnits, cur)
	if err != nil {
		return fmt.Errorf("invalid price: %w", err)
	}

	p, err := domain.NewProduct(pId, name, desc, priceVO, thumbnail)
	if err != nil {
		return err
	}

	if err = ps.storage.Add(ctx, p); err != nil {
		return err
	}

	// A simple product is purchasable through a single default variant
	// carrying its price (no options) and the product image.
	defaultVariant := domain.NewVariant("var-"+id, id, thumbnail, nil, priceVO, defaultStock)
	return ps.storage.AddVariant(ctx, id, 0, defaultVariant)
}

// UpdateProduct validates the core product fields and persists them. It only
// touches the product row (name/description/price/thumbnail); variants,
// categories and attributes are managed by their own methods.
func (ps ProductService) UpdateProduct(ctx context.Context, id, name, desc string, priceMinorUnits int64, currency, thumbnail string) error {
	pID, err := domain.NewProductId(id)
	if err != nil {
		return err
	}
	cur, err := domain.NewCurrency(currency)
	if err != nil {
		return fmt.Errorf("invalid currency: %w", err)
	}
	priceVO, err := domain.NewPrice(priceMinorUnits, cur)
	if err != nil {
		return fmt.Errorf("invalid price: %w", err)
	}
	p, err := domain.NewProduct(pID, name, desc, priceVO, thumbnail)
	if err != nil {
		return err
	}
	return ps.storage.UpdateProduct(ctx, p)
}

// DeleteProduct removes a product; its variants, category and attribute links
// cascade in storage.
func (ps ProductService) DeleteProduct(ctx context.Context, id string) error {
	return ps.storage.DeleteProduct(ctx, id)
}

// SetVariantStock sets a single variant's stock level (rejecting negatives).
func (ps ProductService) SetVariantStock(ctx context.Context, variantID string, stock int) error {
	if stock < 0 {
		return fmt.Errorf("stock cannot be negative: %d", stock)
	}
	return ps.storage.SetVariantStock(ctx, variantID, stock)
}

// SetProductCategories replaces the product's category links with the given set.
func (ps ProductService) SetProductCategories(ctx context.Context, productID string, categoryIDs []string) error {
	return ps.storage.SetProductCategories(ctx, productID, categoryIDs)
}

// SetProductAttributes replaces the product's attribute values with the given set.
func (ps ProductService) SetProductAttributes(ctx context.Context, productID string, values []AttributeAssignment) error {
	return ps.storage.SetProductAttributes(ctx, productID, values)
}

// SetProductAttributeSet assigns (or clears, when setID is empty) the product's
// attribute set. A non-empty setID is validated to exist before assignment.
func (ps ProductService) SetProductAttributeSet(ctx context.Context, productID, setID string) error {
	if setID != "" {
		if _, err := ps.storage.FindAttributeSet(ctx, setID); err != nil {
			return err
		}
	}
	return ps.storage.SetProductAttributeSet(ctx, productID, setID)
}

// ProductAttributeTypes returns the ordered attribute types a product's edit
// form should show: when the product has an attribute set, its members (already
// ordered); otherwise every attribute type as a fallback so products without a
// set still work.
func (ps ProductService) ProductAttributeTypes(ctx context.Context, productID string) ([]domain.AttributeType, error) {
	product, err := ps.storage.Find(ctx, productID)
	if err != nil {
		return nil, err
	}
	if setID := product.AttributeSetID(); setID != "" {
		set, err := ps.storage.FindAttributeSet(ctx, setID)
		if err != nil {
			return nil, err
		}
		return set.Members(), nil
	}
	return ps.storage.AllAttributeTypes(ctx)
}

// OptionTypeInput / VariantInput describe a product's options and variants for
// AddVariantProduct (used by seeding).
type OptionTypeInput struct {
	Name   string
	Values []string
}

type VariantInput struct {
	ID      string
	SKU     string
	Image   string
	Options map[string]string
	Price   int64
	Stock   int
}

// AddVariantProduct creates a product with explicit option types and variants
// (each independently priced). The product's base price is the lowest variant
// price, used for "from $X" display.
func (ps ProductService) AddVariantProduct(ctx context.Context, id, name, desc, currency, thumbnail string, optionTypes []OptionTypeInput, variants []VariantInput) error {
	if len(variants) == 0 {
		return fmt.Errorf("a variant product needs at least one variant")
	}
	pID, err := domain.NewProductId(id)
	if err != nil {
		return err
	}
	cur, err := domain.NewCurrency(currency)
	if err != nil {
		return fmt.Errorf("invalid currency: %w", err)
	}

	base := variants[0].Price
	for _, v := range variants[1:] {
		if v.Price < base {
			base = v.Price
		}
	}
	basePrice, err := domain.NewPrice(base, cur)
	if err != nil {
		return fmt.Errorf("invalid base price: %w", err)
	}

	product, err := domain.NewProduct(pID, name, desc, basePrice, thumbnail)
	if err != nil {
		return err
	}
	if err = ps.storage.Add(ctx, product); err != nil {
		return err
	}

	for i, ot := range optionTypes {
		if err = ps.storage.AddOptionType(ctx, id, i, domain.NewOptionType(ot.Name, ot.Values)); err != nil {
			return err
		}
	}
	for i, v := range variants {
		price, err := domain.NewPrice(v.Price, cur)
		if err != nil {
			return fmt.Errorf("invalid variant price: %w", err)
		}
		if err = ps.storage.AddVariant(ctx, id, i, domain.NewVariant(v.ID, v.SKU, v.Image, v.Options, price, v.Stock)); err != nil {
			return err
		}
	}
	return nil
}

// AddVariantToProduct adds a single variant to an existing product that has
// option types. It validates the product exists and that options cover each
// option type with an allowed value, generates a collision-free variant id and
// appends the variant at the next position.
func (ps ProductService) AddVariantToProduct(ctx context.Context, productID, sku, image string, priceMinor int64, currency string, stock int, options map[string]string) error {
	if priceMinor < 0 {
		return fmt.Errorf("price cannot be negative")
	}
	if stock < 0 {
		return fmt.Errorf("stock cannot be negative")
	}
	product, err := ps.storage.Find(ctx, productID)
	if err != nil {
		return err
	}
	optionTypes := product.OptionTypes()
	if len(optionTypes) == 0 {
		return fmt.Errorf("product %q has no option types; cannot add a variant", productID)
	}

	// Validate that options cover each option type with an allowed value.
	for _, ot := range optionTypes {
		val, ok := options[ot.Name()]
		if !ok || val == "" {
			return fmt.Errorf("missing value for option %q", ot.Name())
		}
		allowed := false
		for _, v := range ot.Values() {
			if v == val {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("value %q is not allowed for option %q", val, ot.Name())
		}
	}

	cur, err := domain.NewCurrency(currency)
	if err != nil {
		return fmt.Errorf("invalid currency: %w", err)
	}
	price, err := domain.NewPrice(priceMinor, cur)
	if err != nil {
		return fmt.Errorf("invalid price: %w", err)
	}

	existing := product.Variants()
	variantID := ps.uniqueVariantID(productID, sku, existing)
	position := len(existing)
	return ps.storage.AddVariant(ctx, productID, position, domain.NewVariant(variantID, sku, image, options, price, stock))
}

// uniqueVariantID derives a variant id (slug of sku, else productID-<n>) that
// does not collide with any existing variant id, appending a counter if needed.
func (ps ProductService) uniqueVariantID(productID, sku string, existing []domain.Variant) string {
	taken := make(map[string]bool, len(existing))
	for _, v := range existing {
		taken[v.ID()] = true
	}
	base := slugify(sku)
	if base == "" {
		base = fmt.Sprintf("%s-%d", productID, len(existing))
	}
	candidate := base
	for i := 1; taken[candidate]; i++ {
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
	return candidate
}

// UpdateVariant updates a single variant's sku, image, price and stock.
func (ps ProductService) UpdateVariant(ctx context.Context, variantID, sku, image string, priceMinor int64, currency string, stock int) error {
	if priceMinor < 0 {
		return fmt.Errorf("price cannot be negative: %d", priceMinor)
	}
	if stock < 0 {
		return fmt.Errorf("stock cannot be negative: %d", stock)
	}
	if _, err := domain.NewCurrency(currency); err != nil {
		return fmt.Errorf("invalid currency: %w", err)
	}
	return ps.storage.UpdateVariant(ctx, variantID, sku, image, priceMinor, currency, stock)
}

// DeleteVariant removes a variant from a product, refusing to delete the last
// remaining variant (a product needs at least one to stay purchasable).
func (ps ProductService) DeleteVariant(ctx context.Context, productID, variantID string) error {
	product, err := ps.storage.Find(ctx, productID)
	if err != nil {
		return err
	}
	if len(product.Variants()) <= 1 {
		return fmt.Errorf("cannot delete the last variant: a product needs at least one variant")
	}
	return ps.storage.DeleteVariant(ctx, variantID)
}

// AddOptionType adds a new option type to an existing product. Because a
// variant must carry a value for every option type to stay resolvable, the
// chosen variantDefault is seeded onto every existing variant.
func (ps ProductService) AddOptionType(ctx context.Context, productID, name string, values []string, variantDefault string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("option type name is required")
	}
	product, err := ps.storage.Find(ctx, productID)
	if err != nil {
		return err
	}
	for _, ot := range product.OptionTypes() {
		if ot.Name() == name {
			return fmt.Errorf("option type %q already exists", name)
		}
	}

	cleaned := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		cleaned = append(cleaned, v)
	}
	if len(cleaned) == 0 {
		return fmt.Errorf("option type %q needs at least one value", name)
	}

	variantDefault = strings.TrimSpace(variantDefault)
	allowed := false
	for _, v := range cleaned {
		if v == variantDefault {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("default value %q must be one of the option values", variantDefault)
	}

	id := fmt.Sprintf("opt-%s-%s", productID, slugify(name))
	position := len(product.OptionTypes())
	return ps.storage.AddProductOptionType(ctx, productID, id, name, position, cleaned, variantDefault)
}

// UpdateOptionType renames an option type and/or replaces its values. It
// guards that every value currently used by a variant for this option remains
// in the new values list, so no variant becomes unreachable.
func (ps ProductService) UpdateOptionType(ctx context.Context, productID, currentName, newName string, values []string) error {
	currentName = strings.TrimSpace(currentName)
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("option type name is required")
	}
	product, err := ps.storage.Find(ctx, productID)
	if err != nil {
		return err
	}

	exists := false
	for _, ot := range product.OptionTypes() {
		if ot.Name() == currentName {
			exists = true
		} else if ot.Name() == newName {
			return fmt.Errorf("option type %q already exists", newName)
		}
	}
	if !exists {
		return fmt.Errorf("option type %q does not exist", currentName)
	}

	cleaned := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		cleaned = append(cleaned, v)
	}
	if len(cleaned) == 0 {
		return fmt.Errorf("option type %q needs at least one value", newName)
	}
	allowed := map[string]bool{}
	for _, v := range cleaned {
		allowed[v] = true
	}

	// Guard: every value a variant currently uses for this option must survive.
	for _, v := range product.Variants() {
		if used, ok := v.Options()[currentName]; ok && used != "" && !allowed[used] {
			return fmt.Errorf("cannot remove value %q: it is in use by a variant", used)
		}
	}

	return ps.storage.UpdateProductOptionType(ctx, productID, currentName, newName, cleaned)
}

// DeleteOptionType removes an option type from a product, stripping its key
// from every variant. It rejects the deletion when two variants would collapse
// to the same option combination (becoming ambiguous for ResolveVariant); this
// also prevents removing the last distinguishing option type.
func (ps ProductService) DeleteOptionType(ctx context.Context, productID, name string) error {
	name = strings.TrimSpace(name)
	product, err := ps.storage.Find(ctx, productID)
	if err != nil {
		return err
	}

	exists := false
	for _, ot := range product.OptionTypes() {
		if ot.Name() == name {
			exists = true
			break
		}
	}
	if !exists {
		return fmt.Errorf("option type %q does not exist", name)
	}

	// Guard: simulate removing the key from every variant and detect collisions.
	seen := map[string]bool{}
	for _, v := range product.Variants() {
		keys := make([]string, 0, len(v.Options()))
		for k := range v.Options() {
			if k == name {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for _, k := range keys {
			b.WriteString(k)
			b.WriteString("=")
			b.WriteString(v.Options()[k])
			b.WriteString(";")
		}
		sig := b.String()
		if seen[sig] {
			return fmt.Errorf("cannot delete option type %q: variants would become ambiguous", name)
		}
		seen[sig] = true
	}

	return ps.storage.DeleteProductOptionType(ctx, productID, name)
}
