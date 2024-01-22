package shipment

import "context"

type appService struct {
	storage storage
}

type storage interface {
	List(ctx context.Context, customerID string) ([]Address, error)
}

func newAppService(s storage) appService {
	return appService{storage: s}
}

func (app appService) List(ctx context.Context, customerID string) ([]Address, error) {
	return app.storage.List(ctx, customerID)
}
