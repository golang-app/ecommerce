package domain

import (
	"errors"
	"regexp"
)

// ErrInvalidCategory is returned when a Category fails validation.
var ErrInvalidCategory = errors.New("invalid category")

// slugPattern validates a slug: lowercase letters, digits and hyphens only.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Category is a flat catalog grouping a product can belong to (many-to-many).
// It is a value object hydrated from storage.
type Category struct {
	id       string
	name     string
	slug     string
	position int
}

// NewCategory builds a validated Category. It errors when the name or slug is
// empty, or when the slug is not lowercase letters/digits/hyphens.
func NewCategory(id, name, slug string, position int) (Category, error) {
	if name == "" {
		return Category{}, ErrInvalidCategory
	}
	if slug == "" || !slugPattern.MatchString(slug) {
		return Category{}, ErrInvalidCategory
	}
	return Category{id: id, name: name, slug: slug, position: position}, nil
}

// RebuildCategory reconstructs a Category from storage.
func RebuildCategory(id, name, slug string, position int) Category {
	return Category{id: id, name: name, slug: slug, position: position}
}

func (c Category) ID() string    { return c.id }
func (c Category) Name() string  { return c.name }
func (c Category) Slug() string  { return c.slug }
func (c Category) Position() int { return c.position }
