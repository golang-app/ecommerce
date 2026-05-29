package inbox

import (
	"context"
	"io"

	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/sirupsen/logrus"
)

// MarkHandler is the narrow seam Wrap depends on, satisfied by
// *Postgres. Keeping it an interface (and not the concrete *Postgres)
// lets dedupe_test.go drop in a tiny in-memory fake without a real DB.
type MarkHandler interface {
	MarkHandled(ctx context.Context, subscriber string, eventID int64) (alreadyHandled bool, err error)
}

// discardLogger is the package-level fallback used when no context
// logger is available. Each entry it produces goes to io.Discard so a
// background subscriber firing outside the HTTP middleware (which is
// what binds observability.Logger) does not silently nil-panic.
var discardLogger logrus.FieldLogger = func() logrus.FieldLogger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}()

// Wrap turns an Outbox-driven HandlerWithID into one that dedupes on
// (subscriber, eventID) using the supplied store before invoking the
// underlying handler.
//
// Behaviour by eventID:
//   - eventID == 0  -> the publisher had no durable id (the compat
//     Publish path on eventbus.Bus); dedupe cannot apply, so the
//     wrapper passes the event straight through to h. Callers that
//     register through SubscribeWithID and rely on Wrap MUST drive
//     deliveries through the Outbox — otherwise this branch silently
//     disables dedupe and you have at-least-once again.
//   - eventID != 0  -> MarkHandled is called. On alreadyHandled the
//     wrapper logs an info-level "skip duplicate delivery" and
//     returns nil (success: the side effect has already happened). On
//     a storage error MarkHandled's err is returned and h is NOT
//     called; the Outbox dispatcher will retry the same row next
//     tick. On a fresh insert the wrapper invokes h and returns its
//     error verbatim.
//
// Logging routes through observability.Logger(ctx), which the HTTP
// middleware binds. The Outbox dispatcher runs in a background
// goroutine without that middleware, so observability.Logger falls
// back to a fresh logrus.Logger and we route to a package-level
// io.Discard sink — the dedupe path is hot and not normally
// interesting. Errors from MarkHandled and the underlying handler
// still propagate to the bus, which logs them at Error.
func Wrap(subscriber string, store MarkHandler, h eventbus.HandlerWithID) eventbus.HandlerWithID {
	return func(ctx context.Context, eventID int64, e eventbus.Event) error {
		if eventID == 0 {
			// Documented pass-through: no dedupe key, no dedupe.
			return h(ctx, eventID, e)
		}
		alreadyHandled, err := store.MarkHandled(ctx, subscriber, eventID)
		if err != nil {
			return err
		}
		if alreadyHandled {
			logger(ctx).WithFields(logrus.Fields{
				"inbox.subscriber": subscriber,
				"event":            e.EventName(),
				"event.id":         eventID,
			}).Info("inbox: skip duplicate delivery")
			return nil
		}
		return h(ctx, eventID, e)
	}
}

// logger returns the context logger bound by the HTTP middleware (or
// by tests). When no logger is bound observability.Logger falls back
// to a fresh logrus.Logger writing to stderr — useful in the HTTP
// path but noisy in the background Outbox dispatcher path, where
// there is no middleware. A nil context (only really seen in tests
// that call the wrapper directly) routes to the package-level
// io.Discard sink so the wrapper stays panic-free.
func logger(ctx context.Context) logrus.FieldLogger {
	if ctx == nil {
		return discardLogger
	}
	return observability.Logger(ctx)
}
