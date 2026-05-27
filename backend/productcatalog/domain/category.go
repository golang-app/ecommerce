package domain

// Category is a flat catalog grouping a product can belong to (many-to-many).
// It is a value object hydrated from storage.
type Category struct {
	id       string
	name     string
	slug     string
	position int
}

// RebuildCategory reconstructs a Category from storage.
func RebuildCategory(id, name, slug string, position int) Category {
	return Category{id: id, name: name, slug: slug, position: position}
}

func (c Category) ID() string    { return c.id }
func (c Category) Name() string  { return c.name }
func (c Category) Slug() string  { return c.slug }
func (c Category) Position() int { return c.position }
