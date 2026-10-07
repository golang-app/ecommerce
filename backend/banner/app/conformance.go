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

	t.Run("list all", func(t *testing.T) {
		b1, _ := domain.NewBanner("list-all-1", domain.SlotTopAnnouncement, "Title 1", 1)
		b2, _ := domain.NewBanner("list-all-2", domain.SlotHomepageHero, "Title 2", 2)

		if err := repo.Store(ctx, b1); err != nil {
			t.Fatalf("unexpected store error: %v", err)
		}
		if err := repo.Store(ctx, b2); err != nil {
			t.Fatalf("unexpected store error: %v", err)
		}

		all, err := repo.ListAll(ctx)
		if err != nil {
			t.Fatalf("unexpected list all error: %v", err)
		}

		found1, found2 := false, false
		for _, b := range all {
			if b.ID == b1.ID {
				found1 = true
			}
			if b.ID == b2.ID {
				found2 = true
			}
		}
		if !found1 || !found2 {
			t.Fatalf("expected both stored banners in ListAll results, got: %+v", all)
		}
	})

	t.Run("list active by slot", func(t *testing.T) {
		b1, _ := domain.NewBanner("active-slot-1", domain.SlotProductGuarantee, "Active Guarantee", 1)
		b2, _ := domain.NewBanner("active-slot-2", domain.SlotProductGuarantee, "Inactive Guarantee", 2)
		b2.IsActive = false
		b3, _ := domain.NewBanner("active-slot-3", domain.SlotHomepageHero, "Active Hero", 3)

		if err := repo.Store(ctx, b1); err != nil {
			t.Fatalf("unexpected store error: %v", err)
		}
		if err := repo.Store(ctx, b2); err != nil {
			t.Fatalf("unexpected store error: %v", err)
		}
		if err := repo.Store(ctx, b3); err != nil {
			t.Fatalf("unexpected store error: %v", err)
		}

		active, err := repo.ListActiveBySlot(ctx, domain.SlotProductGuarantee)
		if err != nil {
			t.Fatalf("unexpected list active by slot error: %v", err)
		}

		found1 := false
		for _, b := range active {
			if b.ID == b1.ID {
				found1 = true
			}
			if b.ID == b2.ID {
				t.Fatalf("found inactive banner %s in ListActiveBySlot", b2.ID)
			}
			if b.Slot != domain.SlotProductGuarantee {
				t.Fatalf("found banner with slot %s in ListActiveBySlot for %s", b.Slot, domain.SlotProductGuarantee)
			}
		}
		if !found1 {
			t.Fatalf("expected active banner %s in ListActiveBySlot, got: %+v", b1.ID, active)
		}
	})
}
