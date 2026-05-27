//go:build !integration

package app_test

import (
	"context"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/matryer/is"
)

// inMemoryClassifier is the subset of the in-memory adapter's test helpers we
// need to populate attributes and categories for List filtering tests.
type inMemoryClassifier interface {
	app.ProductStorage
	AddAttributeType(t domain.AttributeType)
	AddCategory(c domain.Category)
	SetProductAttributes(productID string, values ...domain.AttributeValue)
	SetProductCategories(productID string, cats ...domain.Category)
}

func fptr(v float64) *float64 { return &v }

func seedListFixtures(ctx context.Context, store inMemoryClassifier) error {
	weight := domain.RebuildAttributeType("weight", "Weight", "kg", domain.AttributeNumeric, true, 0)
	material := domain.RebuildAttributeType("material", "Material", "", domain.AttributeEnum, true, 1)
	store.AddAttributeType(weight)
	store.AddAttributeType(material)

	tools := domain.RebuildCategory("cat-tools", "Tools", "tools", 0)
	store.AddCategory(tools)

	type fixture struct {
		id       string
		weight   float64
		material string
		inTools  bool
	}
	fixtures := []fixture{
		{"prod-a", 2, "Steel", true},
		{"prod-b", 5, "Cotton", true},
		{"prod-c", 10, "Steel", false},
	}
	for _, f := range fixtures {
		p, err := domain.NewProduct(domain.ProductID(f.id), "P "+f.id, "desc", domain.MustNewPrice(100, domain.MustNewCurrency("USD")), "thumb")
		if err != nil {
			return err
		}
		if err := store.Add(ctx, p); err != nil {
			return err
		}
		store.SetProductAttributes(f.id, domain.NewNumericValue(weight, f.weight), domain.NewEnumValue(material, f.material))
		if f.inTools {
			store.SetProductCategories(f.id, tools)
		}
	}
	return nil
}

func ids(products []domain.Product) []string {
	out := make([]string, 0, len(products))
	for _, p := range products {
		out = append(out, string(p.ID()))
	}
	return out
}

func TestListFiltering(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	store := adapter.NewInMemory()
	is.NoErr(seedListFixtures(ctx, store))
	appServ := app.NewProductService(store)

	t.Run("numeric range", func(t *testing.T) {
		is := is.New(t)
		got, err := appServ.List(ctx, app.ProductQuery{
			NumericRanges: map[string]app.Range{"weight": {Min: fptr(3), Max: fptr(8)}},
		})
		is.NoErr(err)
		is.Equal(ids(got), []string{"prod-b"})
	})

	t.Run("enum OR within attribute", func(t *testing.T) {
		is := is.New(t)
		got, err := appServ.List(ctx, app.ProductQuery{
			EnumSelections: map[string][]string{"material": {"Steel", "Cotton"}},
		})
		is.NoErr(err)
		is.Equal(ids(got), []string{"prod-a", "prod-b", "prod-c"})
	})

	t.Run("category", func(t *testing.T) {
		is := is.New(t)
		got, err := appServ.List(ctx, app.ProductQuery{CategorySlug: "tools"})
		is.NoErr(err)
		is.Equal(ids(got), []string{"prod-a", "prod-b"})
	})

	t.Run("combined AND across attribute and category", func(t *testing.T) {
		is := is.New(t)
		got, err := appServ.List(ctx, app.ProductQuery{
			CategorySlug:   "tools",
			EnumSelections: map[string][]string{"material": {"Steel"}},
		})
		is.NoErr(err)
		is.Equal(ids(got), []string{"prod-a"})
	})
}

func TestFacets(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	store := adapter.NewInMemory()
	is.NoErr(seedListFixtures(ctx, store))
	appServ := app.NewProductService(store)

	facets, err := appServ.Facets(ctx, "")
	is.NoErr(err)
	is.Equal(len(facets), 2)

	// Ordered by position: weight (numeric) then material (enum).
	is.Equal(facets[0].Type.ID(), "weight")
	is.True(facets[0].Min != nil && *facets[0].Min == 2)
	is.True(facets[0].Max != nil && *facets[0].Max == 10)

	is.Equal(facets[1].Type.ID(), "material")
	is.Equal(facets[1].Values, []string{"Cotton", "Steel"})

	// Scoped to category tools excludes prod-c (weight 10).
	scoped, err := appServ.Facets(ctx, "tools")
	is.NoErr(err)
	is.Equal(scoped[0].Type.ID(), "weight")
	is.True(*scoped[0].Max == 5)
}
