package adapter_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/widget/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/widget/app"
	"github.com/bkielbasa/go-ecommerce/backend/widget/domain"
)

func TestInMemory_Conformance(t *testing.T) {
	repo := adapter.NewInMemory()
	app.TestWidgetRepository(t, repo)
}

func TestInMemory_InitialSlidesAndConfigCloning(t *testing.T) {
	ctx := context.Background()
	repo := adapter.NewInMemory()

	s1, _ := domain.NewSlide("s1", "w1")
	s1.Title = "Slide 1"
	s1.DisplayOrder = 2

	s2, _ := domain.NewSlide("s2", "w1")
	s2.Title = "Slide 2"
	s2.DisplayOrder = 1

	w, _ := domain.NewWidget("w1", domain.PageHome, domain.ZoneTop, domain.TypeBannerCarousel, "Carousel", 1)
	cfg := []byte(`{"show_arrows":true}`)
	w.Config = json.RawMessage(cfg)
	w.Slides = []domain.Slide{s1, s2}

	if err := repo.Store(ctx, w); err != nil {
		t.Fatalf("failed to store widget with slides: %v", err)
	}

	// Mutate caller's config slice
	cfg[0] = 'X'

	// Get widget and verify initial slides are present and sorted by DisplayOrder (s2 first)
	got, err := repo.Get(ctx, "w1")
	if err != nil {
		t.Fatalf("failed to get widget: %v", err)
	}

	if string(got.Config) == `X{"show_arrows":true}` {
		t.Errorf("Config was not cloned, caller mutation affected repo")
	}

	if len(got.Slides) != 2 {
		t.Fatalf("expected 2 slides, got %d", len(got.Slides))
	}
	if got.Slides[0].ID != "s2" || got.Slides[1].ID != "s1" {
		t.Errorf("expected slides sorted by DisplayOrder (s2, s1), got (%s, %s)", got.Slides[0].ID, got.Slides[1].ID)
	}

	// Mutate got.Config slice
	got.Config[0] = 'Y'
	got2, _ := repo.Get(ctx, "w1")
	if string(got2.Config) == string(got.Config) {
		t.Errorf("Config was not cloned on Get, caller mutation affected repo")
	}

	// Test Update with slides
	s3, _ := domain.NewSlide("s3", "w1")
	s3.Title = "Slide 3"
	s3.DisplayOrder = 3

	got.Slides = append(got.Slides, s3)
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("failed to update widget with slides: %v", err)
	}

	got3, _ := repo.Get(ctx, "w1")
	if len(got3.Slides) != 3 {
		t.Fatalf("expected 3 slides after update, got %d", len(got3.Slides))
	}
}

