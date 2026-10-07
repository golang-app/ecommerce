package domain_test

import (
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/banner/domain"
)

func TestNewBanner_Validation(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		slot         domain.Slot
		title        string
		displayOrder int
		wantErr      error
	}{
		{
			name:         "valid top announcement banner",
			id:           "banner-1",
			slot:         domain.SlotTopAnnouncement,
			title:        "Free shipping over $50",
			displayOrder: 1,
			wantErr:      nil,
		},
		{
			name:         "valid homepage hero banner",
			id:           "banner-2",
			slot:         domain.SlotHomepageHero,
			title:        "Spring Collection",
			displayOrder: 0,
			wantErr:      nil,
		},
		{
			name:         "valid product guarantee banner",
			id:           "banner-3",
			slot:         domain.SlotProductGuarantee,
			title:        "2-Year Warranty",
			displayOrder: 2,
			wantErr:      nil,
		},
		{
			name:         "empty id",
			id:           "",
			slot:         domain.SlotTopAnnouncement,
			title:        "Title",
			displayOrder: 0,
			wantErr:      domain.ErrEmptyID,
		},
		{
			name:         "invalid slot",
			id:           "banner-4",
			slot:         domain.Slot("non_existent_slot"),
			title:        "Title",
			displayOrder: 0,
			wantErr:      domain.ErrInvalidSlot,
		},
		{
			name:         "empty title",
			id:           "banner-5",
			slot:         domain.SlotTopAnnouncement,
			title:        "   ",
			displayOrder: 0,
			wantErr:      domain.ErrEmptyTitle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := domain.NewBanner(tt.id, tt.slot, tt.title, tt.displayOrder)
			if err != tt.wantErr {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			if tt.wantErr == nil {
				if b.ID != tt.id || b.Slot != tt.slot || b.Title != tt.title || !b.IsActive {
					t.Fatalf("unexpected banner state: %+v", b)
				}
			}
		})
	}
}
