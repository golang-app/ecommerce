package domain_test

import (
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

func TestAttributeValueDisplay(t *testing.T) {
	weight := domain.RebuildAttributeType("weight", "Weight", "kg", domain.AttributeNumeric, true, 0)
	width := domain.RebuildAttributeType("width", "Width", "cm", domain.AttributeNumeric, true, 1)
	dimensionless := domain.RebuildAttributeType("count", "Count", "", domain.AttributeNumeric, false, 2)
	material := domain.RebuildAttributeType("material", "Material", "", domain.AttributeEnum, true, 3)

	cases := []struct {
		name string
		val  domain.AttributeValue
		want string
	}{
		{"numeric integer with unit", domain.NewNumericValue(weight, 2), "2 kg"},
		{"numeric decimal with unit", domain.NewNumericValue(width, 10.5), "10.5 cm"},
		{"numeric without unit", domain.NewNumericValue(dimensionless, 3), "3"},
		{"numeric trims trailing zeros", domain.NewNumericValue(width, 10.50), "10.5 cm"},
		{"enum", domain.NewEnumValue(material, "Cotton"), "Cotton"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.val.Display(); got != tc.want {
				t.Errorf("Display() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAttributeTypeKindHelpers(t *testing.T) {
	num := domain.RebuildAttributeType("w", "Weight", "kg", domain.AttributeNumeric, true, 0)
	enum := domain.RebuildAttributeType("m", "Material", "", domain.AttributeEnum, false, 1)

	if !num.IsNumeric() || num.IsEnum() {
		t.Errorf("numeric type: IsNumeric=%v IsEnum=%v", num.IsNumeric(), num.IsEnum())
	}
	if !enum.IsEnum() || enum.IsNumeric() {
		t.Errorf("enum type: IsEnum=%v IsNumeric=%v", enum.IsEnum(), enum.IsNumeric())
	}
	if !num.Filterable() || enum.Filterable() {
		t.Errorf("filterable flags wrong: num=%v enum=%v", num.Filterable(), enum.Filterable())
	}
	if num.Unit() != "kg" || num.Name() != "Weight" || num.ID() != "w" || num.Position() != 0 {
		t.Errorf("numeric getters wrong: %+v", num)
	}
}
