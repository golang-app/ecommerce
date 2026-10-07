package app

import (
	"context"
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/banner/domain"
)

func TestBannerRepository(t *testing.T, repo BannerRepository) {
	ctx := context.Background()

	t.Run("store, get, update, delete workflow", func(t *testing.T) {
		b, err := domain.NewBanner("banner-test-1", domain.SlotTopAnnouncement, "Initial Title", 5)
		if err != nil {
			t.Fatalf("unexpected error creating banner: %v", err)
		}
		b.Body = "Test Body"
		b.PromoCode = "CODE10"

		if err := repo.Store(ctx, b); err != nil {
			t.Fatalf("unexpected store error: %v", err)
		}

		fetched, err := repo.Get(ctx, b.ID)
		if err != nil {
			t.Fatalf("unexpected get error: %v", err)
		}
		if fetched.Title != "Initial Title" || fetched.PromoCode != "CODE10" {
			t.Fatalf("fetched banner mismatch: %+v", fetched)
		}

		b.Title = "Updated Title"
		b.IsActive = false
		if err := repo.Update(ctx, b); err != nil {
			t.Fatalf("unexpected update error: %v", err)
		}

		updated, err := repo.Get(ctx, b.ID)
		if err != nil {
			t.Fatalf("unexpected get after update error: %v", err)
		}
		if updated.Title != "Updated Title" || updated.IsActive {
			t.Fatalf("updated banner mismatch: %+v", updated)
		}

		if err := repo.Delete(ctx, b.ID); err != nil {
			t.Fatalf("unexpected delete error: %v", err)
		}

		_, err = repo.Get(ctx, b.ID)
		if !errors.Is(err, domain.ErrBannerNotFound) {
			t.Fatalf("expected ErrBannerNotFound, got %v", err)
		}
	})
}
