package domain

import "strconv"

// AttributeKind distinguishes the two predefined attribute shapes: numeric
// (a number with an optional unit, e.g. Weight in kg) and enum (a value drawn
// from a fixed set, e.g. Material).
type AttributeKind string

const (
	AttributeNumeric AttributeKind = "numeric"
	AttributeEnum    AttributeKind = "enum"
)

// AttributeType is a predefined product attribute (its definition, not a
// product's value). It is a value object hydrated from storage; the
// filterable flag marks types the listing page can filter on.
type AttributeType struct {
	id         string
	name       string
	unit       string
	kind       AttributeKind
	filterable bool
	position   int
}

// RebuildAttributeType reconstructs an AttributeType from storage.
func RebuildAttributeType(id, name, unit string, kind AttributeKind, filterable bool, position int) AttributeType {
	return AttributeType{
		id:         id,
		name:       name,
		unit:       unit,
		kind:       kind,
		filterable: filterable,
		position:   position,
	}
}

func (t AttributeType) ID() string          { return t.id }
func (t AttributeType) Name() string        { return t.name }
func (t AttributeType) Unit() string        { return t.unit }
func (t AttributeType) Kind() AttributeKind { return t.kind }
func (t AttributeType) Filterable() bool    { return t.filterable }
func (t AttributeType) Position() int       { return t.position }
func (t AttributeType) IsNumeric() bool     { return t.kind == AttributeNumeric }
func (t AttributeType) IsEnum() bool        { return t.kind == AttributeEnum }

// AttributeValue pairs an AttributeType with a single product's value for it.
// numValue is used for numeric types, textValue for enum types.
type AttributeValue struct {
	attrType  AttributeType
	numValue  float64
	textValue string
}

// NewNumericValue builds a numeric attribute value.
func NewNumericValue(t AttributeType, v float64) AttributeValue {
	return AttributeValue{attrType: t, numValue: v}
}

// NewEnumValue builds an enum attribute value.
func NewEnumValue(t AttributeType, v string) AttributeValue {
	return AttributeValue{attrType: t, textValue: v}
}

func (a AttributeValue) Type() AttributeType { return a.attrType }
func (a AttributeValue) NumValue() float64   { return a.numValue }
func (a AttributeValue) TextValue() string   { return a.textValue }

// Display formats the value for the UI. Numeric values render the number with
// up to two decimal places (trailing zeros trimmed) plus the unit when set,
// e.g. "2 kg" or "10.5 cm". Enum values render their text verbatim.
func (a AttributeValue) Display() string {
	if a.attrType.IsEnum() {
		return a.textValue
	}
	s := strconv.FormatFloat(a.numValue, 'f', -1, 64)
	if a.attrType.unit != "" {
		return s + " " + a.attrType.unit
	}
	return s
}
