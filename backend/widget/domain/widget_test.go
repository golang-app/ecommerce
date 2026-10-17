package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/widget/domain"
)

func TestNewWidget_Validation(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		page         domain.Page
		zone         domain.Zone
		widgetType   domain.Type
		title        string
		displayOrder int
		wantErr      error
	}{
		{
			name:         "valid widget",
			id:           "w-1",
			page:         domain.PageHome,
			zone:         domain.ZoneTop,
			widgetType:   domain.TypeBannerCarousel,
			title:        "Spring Promotion",
			displayOrder: 1,
			wantErr:      nil,
		},
		{
			name:         "empty id",
			id:           "",
			page:         domain.PageHome,
			zone:         domain.ZoneTop,
			widgetType:   domain.TypeBannerCarousel,
			title:        "Spring Promotion",
			displayOrder: 1,
			wantErr:      domain.ErrEmptyID,
		},
		{
			name:         "invalid page",
			id:           "w-1",
			page:         "invalid_page",
			zone:         domain.ZoneTop,
			widgetType:   domain.TypeBannerCarousel,
			title:        "Spring Promotion",
			displayOrder: 1,
			wantErr:      domain.ErrInvalidPage,
		},
		{
			name:         "invalid zone",
			id:           "w-1",
			page:         domain.PageHome,
			zone:         "invalid_zone",
			widgetType:   domain.TypeBannerCarousel,
			title:        "Spring Promotion",
			displayOrder: 1,
			wantErr:      domain.ErrInvalidZone,
		},
		{
			name:         "invalid type",
			id:           "w-1",
			page:         domain.PageHome,
			zone:         domain.ZoneTop,
			widgetType:   "unknown_type",
			title:        "Spring Promotion",
			displayOrder: 1,
			wantErr:      domain.ErrInvalidType,
		},
		{
			name:         "empty title",
			id:           "w-1",
			page:         domain.PageHome,
			zone:         domain.ZoneTop,
			widgetType:   domain.TypeBannerCarousel,
			title:        "   ",
			displayOrder: 1,
			wantErr:      domain.ErrEmptyTitle,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, err := domain.NewWidget(tc.id, tc.page, tc.zone, tc.widgetType, tc.title, tc.displayOrder)
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if w.ID != tc.id || w.Page != tc.page || w.Zone != tc.zone || w.Type != tc.widgetType || w.Title != tc.title {
				t.Errorf("widget fields mismatch: %+v", w)
			}
			if !w.IsActive {
				t.Errorf("new widget should be active by default")
			}
			if string(w.Config) != "{}" {
				t.Errorf("new widget config should default to {}, got %s", string(w.Config))
			}
		})
	}
}

func TestNewSlide_Validation(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		widgetID string
		wantErr  error
	}{
		{
			name:     "valid slide",
			id:       "s-1",
			widgetID: "w-1",
			wantErr:  nil,
		},
		{
			name:     "empty id",
			id:       "",
			widgetID: "w-1",
			wantErr:  domain.ErrEmptyID,
		},
		{
			name:     "empty widget id",
			id:       "s-1",
			widgetID: "",
			wantErr:  domain.ErrEmptyWidgetID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := domain.NewSlide(tc.id, tc.widgetID)
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.ID != tc.id || s.WidgetID != tc.widgetID {
				t.Errorf("slide fields mismatch: %+v", s)
			}
		})
	}
}

func TestWidget_Configs(t *testing.T) {
	newArrivalsCfg := domain.NewArrivalsConfig{ProductLimit: 8}
	raw, err := json.Marshal(newArrivalsCfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	w, err := domain.NewWidget("w-1", domain.PageHome, domain.ZoneBody, domain.TypeNewArrivals, "New Arrivals", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	w.Config = raw

	var parsed domain.NewArrivalsConfig
	if err := json.Unmarshal(w.Config, &parsed); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	if parsed.ProductLimit != 8 {
		t.Errorf("expected ProductLimit 8, got %d", parsed.ProductLimit)
	}
}
