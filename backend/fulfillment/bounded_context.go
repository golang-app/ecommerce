// Package fulfillment is the composition root for the fulfillment
// (Process Manager) bounded context. It wires the Postgres storage
// adapter and the in-process eventbus publisher into the application
// service, and exposes both the application.BoundedContext envelope
// and the concrete *app.Service so the layout package can declare a
// narrow interface against its public methods without leaking the
// storage type.
package fulfillment

import (
	"context"
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/app"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
)

// New wires the production adapter onto the supplied *sql.DB and binds
// the application service to the in-process bus. The returned *app
// .Service is intentionally exposed so the composition root can call
// the optional With… seams (stock releaser, order line source)
// without re-importing storage types.
func New(db *sql.DB, bus *eventbus.Bus) (application.BoundedContext, *app.Service) {
	storage := adapter.NewPostgres(db)
	srv := app.NewService(storage).WithPublisher(busPublisher{bus: bus})
	return &boundedContext{}, srv
}

// busPublisher is the trivial adapter from app.EventPublisher onto the
// eventbus.Bus's Publish signature (Publish on the bus returns no
// error; the bus logs handler failures itself).
type busPublisher struct {
	bus *eventbus.Bus
}

func (p busPublisher) Publish(ctx context.Context, e eventbus.Event) {
	if p.bus == nil {
		return
	}
	p.bus.Publish(ctx, e)
}

type boundedContext struct{}
