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

func NewBanner(id string, slot Slot, title string, displayOrder int) (Banner, error) {
	if strings.TrimSpace(id) == "" {
		return Banner{}, ErrEmptyID
	}
	if !slot.IsValid() {
		return Banner{}, ErrInvalidSlot
	}
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		return Banner{}, ErrEmptyTitle
	}
	now := time.Now().UTC()
	return Banner{
		ID:           id,
		Slot:         slot,
		Title:        trimmedTitle,
		DisplayOrder: displayOrder,
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}
