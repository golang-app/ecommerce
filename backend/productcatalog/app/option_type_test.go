package app_test

import (
	"context"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/matryer/is"
)

// seedVariantProduct creates a product with the given option types and variants
// directly through the storage, returning its id.
func seedVariantProduct(ctx context.Context, t *testing.T, optionTypes []app.OptionTypeInput, variants []app.VariantInput) string {
	t.Helper()
	srv := app.NewProductService(storage)
	id := randomID()
	err := srv.AddVariantProduct(ctx, id, "Tee", "a tee", "USD", "http://img", optionTypes, variants)
	if err != nil {
		t.Fatalf("seed variant product: %s", err)
	}
	return id
}

func TestAddOptionTypeSeedsExistingVariants(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	srv := app.NewProductService(storage)

	id := seedVariantProduct(ctx, t,
		[]app.OptionTypeInput{{Name: "Color", Values: []string{"Red", "Blue"}}},
		[]app.VariantInput{
			{ID: "v1", SKU: "red", Options: map[string]string{"Color": "Red"}, Price: 100, Stock: 5},
			{ID: "v2", SKU: "blue", Options: map[string]string{"Color": "Blue"}, Price: 100, Stock: 5},
		},
	)

	err := srv.AddOptionType(ctx, id, "Size", []string{"S", "M"}, "S")
	is.NoErr(err)

	p, err := srv.Find(ctx, id)
	is.NoErr(err)
	// Every existing variant must now carry the seeded default, staying resolvable.
	for _, v := range p.Variants() {
		is.Equal(v.Options()["Size"], "S")
	}
	v, ok := p.ResolveVariant(map[string]string{"Color": "Red", "Size": "S"})
	is.True(ok)
	is.Equal(v.ID(), "v1")
}

func TestAddOptionTypeRejectsDuplicateAndBadDefault(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	srv := app.NewProductService(storage)
	id := seedVariantProduct(ctx, t,
		[]app.OptionTypeInput{{Name: "Color", Values: []string{"Red"}}},
		[]app.VariantInput{{ID: "v1", SKU: "red", Options: map[string]string{"Color": "Red"}, Price: 100, Stock: 1}},
	)

	is.True(srv.AddOptionType(ctx, id, "Color", []string{"X"}, "X") != nil)     // duplicate name
	is.True(srv.AddOptionType(ctx, id, "Size", []string{"S", "M"}, "L") != nil) // default not in values
	is.True(srv.AddOptionType(ctx, id, "Size", []string{}, "") != nil)          // no values
	is.True(srv.AddOptionType(ctx, id, "", []string{"S"}, "S") != nil)          // empty name
}

func TestUpdateOptionTypeRenameRekeysVariants(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	srv := app.NewProductService(storage)
	id := seedVariantProduct(ctx, t,
		[]app.OptionTypeInput{{Name: "Color", Values: []string{"Red", "Blue"}}},
		[]app.VariantInput{
			{ID: "v1", SKU: "red", Options: map[string]string{"Color": "Red"}, Price: 100, Stock: 1},
			{ID: "v2", SKU: "blue", Options: map[string]string{"Color": "Blue"}, Price: 100, Stock: 1},
		},
	)

	err := srv.UpdateOptionType(ctx, id, "Color", "Colour", []string{"Red", "Blue"})
	is.NoErr(err)

	p, err := srv.Find(ctx, id)
	is.NoErr(err)
	is.Equal(p.OptionTypes()[0].Name(), "Colour")
	v, ok := p.ResolveVariant(map[string]string{"Colour": "Red"})
	is.True(ok)
	is.Equal(v.ID(), "v1")
}

func TestUpdateOptionTypeGuardsValueInUse(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	srv := app.NewProductService(storage)
	id := seedVariantProduct(ctx, t,
		[]app.OptionTypeInput{{Name: "Color", Values: []string{"Red", "Blue"}}},
		[]app.VariantInput{
			{ID: "v1", SKU: "red", Options: map[string]string{"Color": "Red"}, Price: 100, Stock: 1},
			{ID: "v2", SKU: "blue", Options: map[string]string{"Color": "Blue"}, Price: 100, Stock: 1},
		},
	)

	// Dropping "Blue" while a variant uses it must be rejected.
	err := srv.UpdateOptionType(ctx, id, "Color", "Color", []string{"Red"})
	is.True(err != nil)
}

func TestDeleteOptionTypeStripsAndGuardsAmbiguity(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	srv := app.NewProductService(storage)
	id := seedVariantProduct(ctx, t,
		[]app.OptionTypeInput{
			{Name: "Color", Values: []string{"Red", "Blue"}},
			{Name: "Size", Values: []string{"S"}},
		},
		[]app.VariantInput{
			{ID: "v1", SKU: "red-s", Options: map[string]string{"Color": "Red", "Size": "S"}, Price: 100, Stock: 1},
			{ID: "v2", SKU: "blue-s", Options: map[string]string{"Color": "Blue", "Size": "S"}, Price: 100, Stock: 1},
		},
	)

	// Deleting "Size" is safe: variants still differ by Color.
	err := srv.DeleteOptionType(ctx, id, "Size")
	is.NoErr(err)
	p, err := srv.Find(ctx, id)
	is.NoErr(err)
	for _, v := range p.Variants() {
		_, ok := v.Options()["Size"]
		is.True(!ok)
	}

	// Deleting "Color" now would collapse both variants to {} -> ambiguous.
	err = srv.DeleteOptionType(ctx, id, "Color")
	is.True(err != nil)
}

func TestDeleteOptionTypeRejectsUnknown(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	srv := app.NewProductService(storage)
	id := seedVariantProduct(ctx, t,
		[]app.OptionTypeInput{{Name: "Color", Values: []string{"Red"}}},
		[]app.VariantInput{{ID: "v1", SKU: "red", Options: map[string]string{"Color": "Red"}, Price: 100, Stock: 1}},
	)
	is.True(srv.DeleteOptionType(ctx, id, "Nope") != nil)
}
