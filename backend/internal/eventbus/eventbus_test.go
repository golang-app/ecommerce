package eventbus_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
)

type testEvent struct{ name string }

func (e testEvent) EventName() string {
	if e.name == "" {
		return "test.Event"
	}
	return e.name
}

func TestPublish_DeliversToSubscriber(t *testing.T) {
	bus := eventbus.New(nil)
	var got int
	bus.Subscribe("test.Event", func(context.Context, eventbus.Event) error {
		got++
		return nil
	})

	bus.Publish(context.Background(), testEvent{})

	if got != 1 {
		t.Errorf("handler called %d times, want 1", got)
	}
}

func TestPublish_HandlersFireInRegistrationOrder(t *testing.T) {
	bus := eventbus.New(nil)
	var order []int
	bus.Subscribe("test.Event", func(context.Context, eventbus.Event) error { order = append(order, 1); return nil })
	bus.Subscribe("test.Event", func(context.Context, eventbus.Event) error { order = append(order, 2); return nil })

	bus.Publish(context.Background(), testEvent{})

	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("handler order = %v, want [1 2]", order)
	}
}

func TestPublish_HandlerErrorDoesNotStopOthers(t *testing.T) {
	bus := eventbus.New(nil) // nil logger: errors are swallowed, not logged
	var second bool
	bus.Subscribe("test.Event", func(context.Context, eventbus.Event) error { return errors.New("boom") })
	bus.Subscribe("test.Event", func(context.Context, eventbus.Event) error { second = true; return nil })

	bus.Publish(context.Background(), testEvent{})

	if !second {
		t.Error("second handler should still run after the first returns an error")
	}
}

func TestPublish_NoSubscribersIsNoop(t *testing.T) {
	bus := eventbus.New(nil)
	// must not panic
	bus.Publish(context.Background(), testEvent{name: "test.Unheard"})
}

func TestSubscribe_ScopedByEventName(t *testing.T) {
	bus := eventbus.New(nil)
	var a, b int
	bus.Subscribe("test.A", func(context.Context, eventbus.Event) error { a++; return nil })
	bus.Subscribe("test.B", func(context.Context, eventbus.Event) error { b++; return nil })

	bus.Publish(context.Background(), testEvent{name: "test.A"})

	if a != 1 || b != 0 {
		t.Errorf("a=%d b=%d, want a=1 b=0 (handlers are scoped by event name)", a, b)
	}
}
