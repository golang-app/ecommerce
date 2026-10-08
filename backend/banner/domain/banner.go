package domain

import (
	"errors"
	"strings"
	"time"
)

type Slot string

const (
	SlotTopAnnouncement  Slot = "top_announcement"
	SlotHomepageHero     Slot = "homepage_hero"
	SlotProductGuarantee Slot = "product_guarantee"
)

var (
	ErrEmptyID        = errors.New("banner ID cannot be empty")
	ErrInvalidSlot    = errors.New("invalid banner slot")
	ErrEmptyTitle     = errors.New("banner title cannot be empty")
	ErrBannerNotFound = errors.New("banner not found")
)

func ValidateSlot(s Slot) error {
	if !s.IsValid() {
		return ErrInvalidSlot
	}
	return nil
}

func (s Slot) IsValid() bool {
	switch s {
	case SlotTopAnnouncement, SlotHomepageHero, SlotProductGuarantee:
		return true
	default:
		return false
	}
}

type Banner struct {
	ID           string
	Slot         Slot
	Title        string
	Body         string
	LinkURL      string
	LinkText     string
	PromoCode    string
	Icon         string
	ImageURL     string
	DisplayOrder int
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (b Banner) Validate() error {
	if strings.TrimSpace(b.ID) == "" {
		return ErrEmptyID
	}
	if err := ValidateSlot(b.Slot); err != nil {
		return err
	}
	if strings.TrimSpace(b.Title) == "" {
		return ErrEmptyTitle
	}
	return nil
}

func NewBanner(id string, slot Slot, title string, displayOrder int) (Banner, error) {
	b := Banner{
		ID:           strings.TrimSpace(id),
		Slot:         slot,
		Title:        strings.TrimSpace(title),
		DisplayOrder: displayOrder,
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := b.Validate(); err != nil {
		return Banner{}, err
	}
	return b, nil
}
