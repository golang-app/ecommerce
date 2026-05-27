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

// Bus is a synchronous, in-process publish/subscribe dispatcher.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	logger   logrus.FieldLogger
}

func New(logger logrus.FieldLogger) *Bus {
	return &Bus{handlers: map[string][]Handler{}, logger: logger}
}

// Subscribe registers a handler for an event name. Handlers fire in
// registration order.
func (b *Bus) Subscribe(eventName string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventName] = append(b.handlers[eventName], h)
}

// Publish dispatches an event to all subscribed handlers synchronously.
func (b *Bus) Publish(ctx context.Context, e Event) {
	b.mu.RLock()
	hs := b.handlers[e.EventName()]
	b.mu.RUnlock()

	for _, h := range hs {
		if err := h(ctx, e); err != nil && b.logger != nil {
			b.logger.WithError(err).WithField("event", e.EventName()).Error("event handler failed")
		}
	}
}
