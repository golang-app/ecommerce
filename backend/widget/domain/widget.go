package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Page string

const (
	PageGlobal   Page = "global"
	PageHome     Page = "home"
	PageProduct  Page = "product"
	PageCategory Page = "category"
)

type Zone string

const (
	ZoneTop    Zone = "top"
	ZoneBody   Zone = "body"
	ZoneBottom Zone = "bottom"
)

type Type string

const (
	TypeBannerCarousel   Type = "banner_carousel"
	TypeNewArrivals      Type = "new_arrivals"
	TypeCategoryProducts Type = "category_products"
)

var (
	ErrEmptyID        = errors.New("widget ID cannot be empty")
	ErrEmptyWidgetID  = errors.New("parent widget ID cannot be empty")
	ErrInvalidPage    = errors.New("invalid widget page")
	ErrInvalidZone    = errors.New("invalid widget zone")
	ErrInvalidType    = errors.New("invalid widget type")
	ErrEmptyTitle     = errors.New("widget title cannot be empty")
	ErrWidgetNotFound = errors.New("widget not found")
	ErrSlideNotFound  = errors.New("slide not found")
)

type Widget struct {
	ID           string
	Page         Page
	Zone         Zone
	Type         Type
	Title        string
	Config       json.RawMessage
	DisplayOrder int
	IsActive     bool
	Slides       []Slide
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Slide struct {
	ID           string
	WidgetID     string
	Title        string
	Subtitle     string
	LinkURL      string
	LinkText     string
	PromoCode    string
	Badge        string
	ImageURL     string
	DisplayOrder int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type BannerCarouselConfig struct {
	AutoplayIntervalMs int  `json:"autoplay_interval_ms,omitempty"`
	ShowArrows         bool `json:"show_arrows"`
	ShowDots           bool `json:"show_dots"`
}

type NewArrivalsConfig struct {
	ProductLimit int `json:"product_limit"`
}

type CategoryProductsConfig struct {
	CategoryID   string `json:"category_id"`
	ProductLimit int    `json:"product_limit"`
}

func (p Page) IsValid() bool {
	switch p {
	case PageGlobal, PageHome, PageProduct, PageCategory:
		return true
	default:
		return false
	}
}

func (z Zone) IsValid() bool {
	switch z {
	case ZoneTop, ZoneBody, ZoneBottom:
		return true
	default:
		return false
	}
}

func (t Type) IsValid() bool {
	switch t {
	case TypeBannerCarousel, TypeNewArrivals, TypeCategoryProducts:
		return true
	default:
		return false
	}
}

func NewWidget(id string, page Page, zone Zone, widgetType Type, title string, displayOrder int) (Widget, error) {
	if strings.TrimSpace(id) == "" {
		return Widget{}, ErrEmptyID
	}
	if !page.IsValid() {
		return Widget{}, ErrInvalidPage
	}
	if !zone.IsValid() {
		return Widget{}, ErrInvalidZone
	}
	if !widgetType.IsValid() {
		return Widget{}, ErrInvalidType
	}
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		return Widget{}, ErrEmptyTitle
	}

	now := time.Now()
	return Widget{
		ID:           id,
		Page:         page,
		Zone:         zone,
		Type:         widgetType,
		Title:        trimmedTitle,
		Config:       json.RawMessage("{}"),
		DisplayOrder: displayOrder,
		IsActive:     true,
		Slides:       []Slide{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func NewSlide(id, widgetID string) (Slide, error) {
	if strings.TrimSpace(id) == "" {
		return Slide{}, ErrEmptyID
	}
	if strings.TrimSpace(widgetID) == "" {
		return Slide{}, ErrEmptyWidgetID
	}
	now := time.Now()
	return Slide{
		ID:        id,
		WidgetID:  widgetID,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}
