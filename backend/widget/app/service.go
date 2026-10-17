package app

import (
	"context"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/widget/domain"
)

type Service struct {
	repo WidgetRepository
}

func NewService(repo WidgetRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListAll(ctx context.Context) ([]domain.Widget, error) {
	return s.repo.ListAll(ctx)
}

func (s *Service) ListActiveByPages(ctx context.Context, pages ...domain.Page) ([]domain.Widget, error) {
	return s.repo.ListActiveByPages(ctx, pages...)
}

func (s *Service) Get(ctx context.Context, id string) (domain.Widget, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, w domain.Widget) error {
	w.CreatedAt = time.Now()
	w.UpdatedAt = w.CreatedAt
	return s.repo.Store(ctx, w)
}

func (s *Service) Update(ctx context.Context, w domain.Widget) error {
	w.UpdatedAt = time.Now()
	return s.repo.Update(ctx, w)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) ToggleActive(ctx context.Context, id string) error {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	w.IsActive = !w.IsActive
	w.UpdatedAt = time.Now()
	return s.repo.Update(ctx, w)
}

func (s *Service) AddSlide(ctx context.Context, slide domain.Slide) error {
	slide.CreatedAt = time.Now()
	slide.UpdatedAt = slide.CreatedAt
	return s.repo.StoreSlide(ctx, slide)
}

func (s *Service) DeleteSlide(ctx context.Context, widgetID, slideID string) error {
	return s.repo.DeleteSlide(ctx, widgetID, slideID)
}
