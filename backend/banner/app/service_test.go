package app_test

import (
	"context"
	"sync"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/banner/app"
	"github.com/bkielbasa/go-ecommerce/backend/banner/domain"
)

type inMemoryRepo struct {
	mu      sync.Mutex
	banners map[string]domain.Banner
}

func newInMemoryRepo() *inMemoryRepo {
	return &inMemoryRepo{
		banners: make(map[string]domain.Banner),
	}
}

func (r *inMemoryRepo) Get(ctx context.Context, id string) (domain.Banner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.banners[id]
	if !ok {
		return domain.Banner{}, domain.ErrBannerNotFound
	}
	return b, nil
}

func (r *inMemoryRepo) ListAll(ctx context.Context) ([]domain.Banner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []domain.Banner
	for _, b := range r.banners {
		list = append(list, b)
	}
	return list, nil
}

func (r *inMemoryRepo) ListActiveBySlot(ctx context.Context, slot domain.Slot) ([]domain.Banner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []domain.Banner
	for _, b := range r.banners {
		if b.Slot == slot && b.IsActive {
			list = append(list, b)
		}
	}
	return list, nil
}

func (r *inMemoryRepo) Store(ctx context.Context, b domain.Banner) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.banners[b.ID] = b
	return nil
}

func (r *inMemoryRepo) Update(ctx context.Context, b domain.Banner) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.banners[b.ID]; !ok {
		return domain.ErrBannerNotFound
	}
	r.banners[b.ID] = b
	return nil
}

func (r *inMemoryRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.banners, id)
	return nil
}

func TestService_ListActiveBySlot(t *testing.T) {
	repo := newInMemoryRepo()
	svc := app.NewService(repo)
	ctx := context.Background()

	b1, _ := domain.NewBanner("b1", domain.SlotTopAnnouncement, "Announcement 1", 20)
	b2, _ := domain.NewBanner("b2", domain.SlotTopAnnouncement, "Announcement 2", 10)
	b3, _ := domain.NewBanner("b3", domain.SlotTopAnnouncement, "Inactive Announcement", 5)
	b3.IsActive = false
	b4, _ := domain.NewBanner("b4", domain.SlotHomepageHero, "Hero Banner", 1)

	_ = repo.Store(ctx, b1)
	_ = repo.Store(ctx, b2)
	_ = repo.Store(ctx, b3)
	_ = repo.Store(ctx, b4)

	active, err := svc.ListActiveBySlot(ctx, domain.SlotTopAnnouncement)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(active) != 2 {
		t.Fatalf("expected 2 active banners, got %d", len(active))
	}
	if active[0].ID != "b2" || active[1].ID != "b1" {
		t.Fatalf("expected ordered by display_order (b2 then b1), got: %v, %v", active[0].ID, active[1].ID)
	}
}

func TestService_ToggleActive(t *testing.T) {
	repo := newInMemoryRepo()
	svc := app.NewService(repo)
	ctx := context.Background()

	b, _ := domain.NewBanner("b1", domain.SlotTopAnnouncement, "Title", 1)
	_ = repo.Store(ctx, b)

	if err := svc.ToggleActive(ctx, "b1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, err := svc.Get(ctx, "b1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.IsActive {
		t.Fatalf("expected banner to be inactive after toggle")
	}
}

func TestInMemoryRepo_Conformance(t *testing.T) {
	repo := newInMemoryRepo()
	app.TestBannerRepository(t, repo)
}
