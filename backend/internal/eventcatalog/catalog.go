// Package eventcatalog is a hand-maintained registry of every event type
// the codebase emits — both the internal, event-sourced domain events
// (checkout/domain, fulfillment/domain) and the published integration
// events that cross context boundaries (checkout/integration,
// fulfillment/integration).
//
// WHY HAND-CURATED, NOT AST-PARSED
//
// A real AST walker over the source tree would be more elegant but
// brittle: it has to recognise the conventions used to declare an
// event (EventType / EventName methods, marker interfaces), track
// versioning through codec switches, and stay correct when the team
// changes those conventions. A hand-maintained list is cheap, total,
// and trivially reviewable. The discipline is the same one the
// docs/glossary.md follows — when you introduce a new event type,
// append a row to Catalog() in the same commit. The unit test in
// catalog_test.go enforces basic consistency (no duplicates, valid
// kinds, version ≥ 1) so accidental drift fails CI rather than the
// docs.
//
// The Catalog feeds the `ecommerce events` CLI subcommand, which
// renders a Markdown table that is regenerated from source — living
// architecture documentation that cannot silently fall behind the
// code.
package eventcatalog

// Kind is the qualitative category of an event. Two values only:
// "domain" for internal events that live inside a single bounded
// context (event-sourced into its own event log or used as an
// in-memory bridge to the publisher), and "integration" for events
// published across the bus as the published language of one context
// to its subscribers.
const (
	KindDomain      = "domain"
	KindIntegration = "integration"
)

// Event describes one event type for the catalog. Fields are kept
// human-friendly because the immediate consumer is a Markdown table —
// the value is documentation, not machine schema.
type Event struct {
	// Name is the wire / EventType identifier as used in storage or
	// on the bus. For integration events this matches EventName()
	// (e.g. "checkout.OrderPaid"); for domain events we prefix
	// the package shortname so the table groups naturally (e.g.
	// "checkout.OrderPlaced", "fulfillment.FulfillmentScheduled").
	Name string

	// Kind is one of KindDomain or KindIntegration. Enforced by
	// the consistency test.
	Kind string

	// Package is the import path slice that owns the event type
	// (e.g. "checkout/domain", "fulfillment/integration"). Kept as
	// a short relative path so the table column stays narrow.
	Package string

	// Version is the latest payload version known to the codebase.
	// Domain events that have never been re-shaped stay at 1;
	// event-sourced events that have been migrated through an
	// upcaster (e.g. checkout.OrderPlaced today) carry the latest
	// version the writer emits.
	Version int

	// Producer is the bounded context that emits the event, in
	// human-readable form (e.g. "checkout", "fulfillment").
	Producer string

	// Consumers lists the contexts / subscribers that react to the
	// event, again in human-readable form. May be empty for purely
	// internal domain events.
	Consumers []string

	// Description is a one-line summary written for readers of the
	// generated table. Note whether the event is notification-style
	// (IDs only) or event-carried-state-transfer (full snapshot)
	// here when relevant.
	Description string
}

// Catalog returns every event type in the codebase.
//
// HAND-MAINTAINED — when you introduce a new event, add it here in
// the same commit. The CI catalog-drift test (see catalog_test.go)
// guards basic shape; semantic accuracy is on the author.
//
// Ordering inside the slice is not load-bearing: the CLI sorts by
// Name before rendering.
func Catalog() []Event {
	return []Event{
		// --- checkout / domain (event-sourced) ---
		{
			Name:        "checkout.OrderPlaced",
			Kind:        KindDomain,
			Package:     "checkout/domain",
			Version:     2,
			Producer:    "checkout",
			Description: "An order has been placed. Carries the full line snapshot, ship and pay choices, tax, shipping cost, discount, and originating sales channel. v2 added Channel; the codec upcasts v1 rows to 'unknown'.",
		},
		{
			Name:        "checkout.PaymentSucceeded",
			Kind:        KindDomain,
			Package:     "checkout/domain",
			Version:     1,
			Producer:    "checkout",
			Description: "The payment for an order has been captured.",
		},
		{
			Name:        "checkout.PaymentFailed",
			Kind:        KindDomain,
			Package:     "checkout/domain",
			Version:     1,
			Producer:    "checkout",
			Description: "The payment for an order was declined; carries the failure reason.",
		},
		{
			Name:        "checkout.OrderCancelled",
			Kind:        KindDomain,
			Package:     "checkout/domain",
			Version:     1,
			Producer:    "checkout",
			Description: "A placed order was cancelled by the customer; carries the cancellation reason.",
		},
		{
			Name:        "checkout.OrderShipped",
			Kind:        KindDomain,
			Package:     "checkout/domain",
			Version:     1,
			Producer:    "checkout",
			Description: "Legacy commercial-side shipment event written to the order's event log; carrier and tracking code are optional. The operational lifecycle now lives in fulfillment/domain.",
		},
		{
			Name:        "checkout.OrderDelivered",
			Kind:        KindDomain,
			Package:     "checkout/domain",
			Version:     1,
			Producer:    "checkout",
			Description: "Legacy commercial-side delivery event written to the order's event log. Retained for replay; operational deliveries now live in fulfillment/domain.",
		},
		{
			Name:        "checkout.OrderRefunded",
			Kind:        KindDomain,
			Package:     "checkout/domain",
			Version:     1,
			Producer:    "checkout",
			Description: "A paid/shipped/delivered order has been refunded; carries the refund reason. Refund returns goods so reserved/sold stock is added back to the catalogue.",
		},

		// --- fulfillment / domain (state-stored, in-memory bridge) ---
		{
			Name:        "fulfillment.FulfillmentScheduled",
			Kind:        KindDomain,
			Package:     "fulfillment/domain",
			Version:     1,
			Producer:    "fulfillment",
			Description: "A Fulfillment record has been spawned by OnOrderPaid in StatusScheduled. Not persisted: in-memory bridge drained by the service after a successful save.",
		},
		{
			Name:        "fulfillment.FulfillmentLabeled",
			Kind:        KindDomain,
			Package:     "fulfillment/domain",
			Version:     1,
			Producer:    "fulfillment",
			Description: "A shipping label was printed and the carrier + tracking code recorded; transitions Scheduled to Labeled.",
		},
		{
			Name:        "fulfillment.FulfillmentShipped",
			Kind:        KindDomain,
			Package:     "fulfillment/domain",
			Version:     1,
			Producer:    "fulfillment",
			Description: "The carrier has accepted the parcel; transitions Labeled to Shipped.",
		},
		{
			Name:        "fulfillment.FulfillmentDelivered",
			Kind:        KindDomain,
			Package:     "fulfillment/domain",
			Version:     1,
			Producer:    "fulfillment",
			Description: "The parcel was delivered; transitions Shipped to the happy-path terminal Delivered state.",
		},
		{
			Name:        "fulfillment.FulfillmentRefunded",
			Kind:        KindDomain,
			Package:     "fulfillment/domain",
			Version:     1,
			Producer:    "fulfillment",
			Description: "The fulfillment was refunded from any active state; carries the operator-supplied reason.",
		},

		// --- checkout / integration (published language) ---
		{
			Name:     "checkout.OrderPaid",
			Kind:     KindIntegration,
			Package:  "checkout/integration",
			Version:  1,
			Producer: "checkout",
			Consumers: []string{
				"cart (clear basket)",
				"email (order confirmation)",
				"fulfillment (schedule fulfillment)",
			},
			Description: "Published when an order's payment succeeds. Notification-style: carries OrderID / SessionID / CustomerID / At only, subscribers re-fetch state if they need details.",
		},

		// --- fulfillment / integration (published language) ---
		{
			Name:        "fulfillment.OrderShipped",
			Kind:        KindIntegration,
			Package:     "fulfillment/integration",
			Version:     1,
			Producer:    "fulfillment",
			Description: "Published when a fulfillment reaches the Shipped state. Notification-style with the carrier + tracking code echoed for convenience; no external subscribers wired yet.",
		},
		{
			Name:        "fulfillment.OrderDelivered",
			Kind:        KindIntegration,
			Package:     "fulfillment/integration",
			Version:     1,
			Producer:    "fulfillment",
			Description: "Published when a fulfillment reaches the Delivered terminal state. Notification-style; no external subscribers wired yet.",
		},
		{
			Name:        "fulfillment.OrderRefunded",
			Kind:        KindIntegration,
			Package:     "fulfillment/integration",
			Version:     1,
			Producer:    "fulfillment",
			Description: "Published when a fulfillment is refunded; carries the operator-supplied reason. Notification-style; no external subscribers wired yet.",
		},
	}
}
