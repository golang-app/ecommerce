// Package outbox implements the Transactional Outbox pattern: producers
// stage integration events into a database table inside their own write
// transaction, and a separate dispatcher publishes those rows onto the
// in-process event bus after the commit succeeds.
//
// The package is content-agnostic — it only deals with a kind (event
// name) and an already-encoded JSON payload. The producer is
// responsible for encoding; the dispatcher is given a decode callback
// by the composition root so it can rebuild the integration event from
// (kind, payload).
package outbox

import "time"

// Event is the in-package representation of a single staged integration
// event. Kind is the wire name (matches eventbus.Event.EventName());
// Payload is its JSON-encoded body. CreatedAt is when the producer
// staged the row.
type Event struct {
	Kind      string
	Payload   []byte
	CreatedAt time.Time
}
