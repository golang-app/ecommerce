package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/widget/domain"
)

func TestWidgetRepository(t *testing.T, repo WidgetRepository) {
	ctx := context.Background()

	t.Run("store, get, update, delete widget with slides", func(t *testing.T) {
		w, err := domain.NewWidget("w-conf-1", domain.PageHome, domain.ZoneTop, domain.TypeBannerCarousel, "Hero Banner", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		w.Config = json.RawMessage(`{"show_dots":true}`)

		if err := repo.Store(ctx, w); err != nil {
			t.Fatalf("failed to store widget: %v", err)
		}

		// Store slide
		slide, err := domain.NewSlide("s-conf-1", w.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		slide.Title = "Slide 1"
		slide.DisplayOrder = 1
		slide.ImageURL = "/images/slide1.jpg"

		if err := repo.StoreSlide(ctx, slide); err != nil {
			t.Fatalf("failed to store slide: %v", err)
		}

		// Get widget with attached slides
		got, err := repo.Get(ctx, w.ID)
		if err != nil {
			t.Fatalf("failed to get widget: %v", err)
		}
		if got.Title != "Hero Banner" {
			t.Errorf("expected title 'Hero Banner', got '%s'", got.Title)
		}
		if len(got.Slides) != 1 {
			t.Fatalf("expected 1 slide, got %d", len(got.Slides))
		}
		if got.Slides[0].Title != "Slide 1" {
			t.Errorf("expected slide title 'Slide 1', got '%s'", got.Slides[0].Title)
		}

		// Update widget
		got.Title = "Updated Banner"
		got.DisplayOrder = 5
		if err := repo.Update(ctx, got); err != nil {
			t.Fatalf("failed to update widget: %v", err)
		}

		updated, err := repo.Get(ctx, w.ID)
		if err != nil {
			t.Fatalf("failed to get updated widget: %v", err)
		}
		if updated.Title != "Updated Banner" || updated.DisplayOrder != 5 {
			t.Errorf("update was not persisted: %+v", updated)
		}

		// Delete slide
		if err := repo.DeleteSlide(ctx, w.ID, slide.ID); err != nil {
			t.Fatalf("failed to delete slide: %v", err)
		}
		afterSlideDelete, err := repo.Get(ctx, w.ID)
		if err != nil {
			t.Fatalf("failed to get widget: %v", err)
		}
		if len(afterSlideDelete.Slides) != 0 {
			t.Errorf("expected 0 slides after delete, got %d", len(afterSlideDelete.Slides))
		}

		// Delete widget
		if err := repo.Delete(ctx, w.ID); err != nil {
			t.Fatalf("failed to delete widget: %v", err)
		}
		_, err = repo.Get(ctx, w.ID)
		if !errors.Is(err, domain.ErrWidgetNotFound) {
			t.Fatalf("expected ErrWidgetNotFound, got %v", err)
		}
	})

	t.Run("list all and list active by pages", func(t *testing.T) {
		wGlobal, _ := domain.NewWidget("w-global", domain.PageGlobal, domain.ZoneTop, domain.TypeBannerCarousel, "Global Announcement", 1)
		wHomeActive, _ := domain.NewWidget("w-home-act", domain.PageHome, domain.ZoneBody, domain.TypeNewArrivals, "New Arrivals", 1)
		wHomeInactive, _ := domain.NewWidget("w-home-inact", domain.PageHome, domain.ZoneBottom, domain.TypeCategoryProducts, "Old Grid", 2)
		wHomeInactive.IsActive = false
		wProduct, _ := domain.NewWidget("w-prod", domain.PageProduct, domain.ZoneTop, domain.TypeBannerCarousel, "Product Banner", 1)

		for _, w := range []domain.Widget{wGlobal, wHomeActive, wHomeInactive, wProduct} {
			_ = repo.Store(ctx, w)
		}

		all, err := repo.ListAll(ctx)
		if err != nil {
			t.Fatalf("failed to list all: %v", err)
		}
		if len(all) < 4 {
			t.Errorf("expected at least 4 widgets, got %d", len(all))
		}

		// Query active for Home + Global
		activeHome, err := repo.ListActiveByPages(ctx, domain.PageGlobal, domain.PageHome)
		if err != nil {
			t.Fatalf("failed to list active: %v", err)
		}

		foundGlobal := false
		foundHomeActive := false
		foundHomeInactive := false
		for _, w := range activeHome {
			if w.ID == "w-global" {
				foundGlobal = true
			}
			if w.ID == "w-home-act" {
				foundHomeActive = true
			}
			if w.ID == "w-home-inact" {
				foundHomeInactive = true
			}
		}

		if !foundGlobal {
			t.Errorf("expected global widget in active list for home")
		}
		if !foundHomeActive {
			t.Errorf("expected active home widget in active list")
		}
		if foundHomeInactive {
			t.Errorf("inactive widget should not be returned in active list")
		}
	})
}
