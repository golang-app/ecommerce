package domain

import "errors"

// ErrInvalidAttributeSet is returned when an AttributeSet fails validation.
var ErrInvalidAttributeSet = errors.New("invalid attribute set")

// ErrAttributeSetNotFound is returned when a lookup for an attribute set by id
// finds no matching set.
var ErrAttributeSetNotFound = errors.New("attribute set not found")

// AttributeSet is a named, reusable grouping that selects which attribute types
// a product should have and in what order. It is a value object hydrated from
// storage; members holds the set's attribute types in their configured order.
type AttributeSet struct {
	id       string
	name     string
	position int
	members  []AttributeType
}

// NewAttributeSet builds a validated AttributeSet. It errors when the name is
// empty. Members are attached separately via WithMembers (they are stored in a
// join table).
func NewAttributeSet(id, name string, position int) (AttributeSet, error) {
	if name == "" {
		return AttributeSet{}, ErrInvalidAttributeSet
	}
	return AttributeSet{
		id:       id,
		name:     name,
		position: position,
	}, nil
}

// RebuildAttributeSet reconstructs an AttributeSet (with its members) from
// storage.
func RebuildAttributeSet(id, name string, position int, members []AttributeType) AttributeSet {
	return AttributeSet{
		id:       id,
		name:     name,
		position: position,
		members:  members,
	}
}

func (s AttributeSet) ID() string               { return s.id }
func (s AttributeSet) Name() string             { return s.name }
func (s AttributeSet) Position() int            { return s.position }
func (s AttributeSet) Members() []AttributeType { return s.members }

// WithMembers returns a copy of the set with its members replaced (used by the
// storage layer after loading them).
func (s AttributeSet) WithMembers(members []AttributeType) AttributeSet {
	s.members = members
	return s
}
