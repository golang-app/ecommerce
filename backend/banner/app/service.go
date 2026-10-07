package app

import (
	"context"
	"sort"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/banner/domain"
)

type BannerRepository interface {
	Get(ctx context.Context, id string) (domain.Banner, error)
	ListAll(ctx context.Context) ([]domain.Banner, error)
	ListActiveBySlot(ctx context.Context, slot domain.Slot) ([]domain.Banner, error)
	Store(ctx context.Context, b domain.Banner) error
	Update(ctx context.Context, b domain.Banner) error
	Delete(ctx context.Context, id string) error
}

type Service struct {
	repo BannerRepository
}

func NewService(repo BannerRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListAll(ctx context.Context) ([]domain.Banner, error) {
	banners, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(banners, func(i, j int) bool {
		if banners[i].Slot == banners[j].Slot {
			return banners[i].DisplayOrder < banners[j].DisplayOrder
		}
		return banners[i].Slot < banners[j].Slot
	})
	return banners, nil
}

func (s *Service) ListActiveBySlot(ctx context.Context, slot domain.Slot) ([]domain.Banner, error) {
	banners, err := s.repo.ListActiveBySlot(ctx, slot)
	if err != nil {
		return nil, err
	}
	sort.Slice(banners, func(i, j int) bool {
		return banners[i].DisplayOrder < banners[j].DisplayOrder
	})
	return banners, nil
}

func (s *Service) Get(ctx context.Context, id string) (domain.Banner, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, b domain.Banner) error {
	return s.repo.Store(ctx, b)
}

func (s *Service) Update(ctx context.Context, b domain.Banner) error {
	b.UpdatedAt = time.Now().UTC()
	return s.repo.Update(ctx, b)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) ToggleActive(ctx context.Context, id string) error {
	b, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	b.IsActive = !b.IsActive
	b.UpdatedAt = time.Now().UTC()
	return s.repo.Update(ctx, b)
}
