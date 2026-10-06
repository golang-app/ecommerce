package domain

import (
	"errors"
	"regexp"
)

// ErrInvalidCategory is returned when a Category fails validation.
var ErrInvalidCategory = errors.New("invalid category")

// ErrCategoryHasChildren is returned when attempting to delete a category that has child subcategories.
var ErrCategoryHasChildren = errors.New("category has child subcategories and cannot be deleted")

// ErrCyclicCategoryHierarchy is returned when assigning a parent would create a loop in the tree.
var ErrCyclicCategoryHierarchy = errors.New("cyclic category hierarchy detected")

// slugPattern validates a slug: lowercase letters, digits and hyphens only.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Category is a hierarchical catalog grouping a product can belong to (many-to-many).
// It is a value object hydrated from storage.
type Category struct {
	id       string
	name     string
	slug     string
	position int
	parentID string
}

// NewCategory builds a validated Category. It errors when the name or slug is
// empty, when the slug is invalid, or when parentID equals id (self-parenting).
func NewCategory(id, name, slug string, position int, parentID string) (Category, error) {
	if name == "" {
		return Category{}, ErrInvalidCategory
	}
	if slug == "" || !slugPattern.MatchString(slug) {
		return Category{}, ErrInvalidCategory
	}
	if id != "" && parentID != "" && id == parentID {
		return Category{}, ErrInvalidCategory
	}
	return Category{id: id, name: name, slug: slug, position: position, parentID: parentID}, nil
}

// RebuildCategory reconstructs a Category from storage.
func RebuildCategory(id, name, slug string, position int, parentID string) Category {
	return Category{id: id, name: name, slug: slug, position: position, parentID: parentID}
}

func (c Category) ID() string       { return c.id }
func (c Category) Name() string     { return c.name }
func (c Category) Slug() string     { return c.slug }
func (c Category) Position() int    { return c.position }
func (c Category) ParentID() string { return c.parentID }
func (c Category) IsRoot() bool     { return c.parentID == "" }
