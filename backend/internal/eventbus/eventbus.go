// Package eventbus provides a tiny synchronous, in-process publish/subscribe
// dispatcher used for integration between bounded contexts. A context
// publishes an integration event; other contexts subscribe and react. The
// publisher stays unaware of who (if anyone) is listening.
package eventbus

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"
)

// Event is an integration event published by one bounded context for others
// to react to. Implementations belong to the publishing context's published
// language.
type Event interface {
	EventName() string
}

// Handler reacts to a published event. A handler error is logged but does not
// abort publishing or stop the remaining handlers: subscribers are
// best-effort side effects, decoupled from the publisher's own transaction.
type Handler func(ctx context.Context, e Event) error

// HandlerWithID is the Outbox-aware variant of Handler. It receives the
// originating event's durable id alongside the event, so the subscriber
// (or a wrapper such as internal/inbox.Wrap) can use it as a stable
// dedupe key across redeliveries.
//
// When the publisher cannot supply a meaningful id — direct in-process
// Publish calls, anything that did not originate from the Outbox — the
// id is 0. Wrappers that depend on the id for dedupe MUST treat 0 as
// "no id available" and pass the event through unfiltered; otherwise
// every direct publish would dedupe against the same zero key.
type HandlerWithID func(ctx context.Context, eventID int64, e Event) error

// Bus is a synchronous, in-process publish/subscribe dispatcher.
//
// The Bus keeps two parallel handler lists per event name: classic
// Handler closures registered via Subscribe and HandlerWithID closures
// registered via SubscribeWithID. Both lists fire on every publish; the
// id-aware list also receives the supplied event id (or 0 from the
// compat Publish path).
type Bus struct {
	mu             sync.RWMutex
	handlers       map[string][]Handler
	handlersWithID map[string][]HandlerWithID
	logger         logrus.FieldLogger
}

func New(logger logrus.FieldLogger) *Bus {
	return &Bus{
		handlers:       map[string][]Handler{},
		handlersWithID: map[string][]HandlerWithID{},
		logger:         logger,
	}
}

// Subscribe registers a handler for an event name. Handlers fire in
// registration order.
func (b *Bus) Subscribe(eventName string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventName] = append(b.handlers[eventName], h)
}

// SubscribeWithID registers an id-aware handler for an event name.
// Handlers fire in registration order within their own list; the
// classic id-less Subscribe list always runs before the id-aware list
// for the same event name, matching the order Publish iterates them.
func (b *Bus) SubscribeWithID(eventName string, h HandlerWithID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlersWithID[eventName] = append(b.handlersWithID[eventName], h)
}

// Publish dispatches an event to all subscribed handlers synchronously.
// This is the backwards-compatible entry point for in-process publishers
// that have no Outbox row id to share — it forwards to PublishWithID
// with eventID==0, the documented "no dedupe id available" sentinel.
func (b *Bus) Publish(ctx context.Context, e Event) {
	b.PublishWithID(ctx, 0, e)
}

// PublishWithID dispatches an event to all subscribed handlers
// synchronously and threads the supplied originating event id through
// the id-aware handler list. eventID==0 means "no id available"; the
// classic id-less handlers always receive only (ctx, e).
//
// The Outbox dispatcher calls this with the outbox_event.id of the row
// it is draining so id-aware wrappers (internal/inbox.Wrap) can use the
// id as a content-stable dedupe key across redeliveries.
func (b *Bus) PublishWithID(ctx context.Context, eventID int64, e Event) {
	b.mu.RLock()
	hs := b.handlers[e.EventName()]
	hsID := b.handlersWithID[e.EventName()]
	b.mu.RUnlock()

	for _, h := range hs {
		if err := h(ctx, e); err != nil && b.logger != nil {
			b.logger.WithError(err).WithField("event", e.EventName()).Error("event handler failed")
		}
	}
	for _, h := range hsID {
		if err := h(ctx, eventID, e); err != nil && b.logger != nil {
			b.logger.WithError(err).WithFields(logrus.Fields{
				"event":    e.EventName(),
				"event.id": eventID,
			}).Error("event handler failed")
		}
	}
}
