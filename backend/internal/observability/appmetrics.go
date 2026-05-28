package observability

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// meterName is the single instrumentation scope used for every application
// metric emitted by gocommerce. Keeping one scope makes Prometheus filtering
// trivial (`otel_scope_name="gocommerce"`).
const meterName = "gocommerce"

// instruments groups every package-level metric handle constructed by
// InitMetrics. They are stored in a single value so callers see a coherent
// snapshot: either every instrument was created successfully, or none was
// (the noop fallback kicks in).
type instruments struct {
	OrdersPlaced       metric.Int64Counter
	OrdersFinalized    metric.Int64Counter
	PaymentsCharged    metric.Int64Counter
	PaymentsFailed     metric.Int64Counter
	RevenueMinorUnits  metric.Int64Counter
	StockReserved      metric.Int64Counter
	StockReleased      metric.Int64Counter
	Logins             metric.Int64Counter
	Registrations      metric.Int64Counter
	CartItemsAdded     metric.Int64Counter
	SearchQueries      metric.Int64Counter
	EmailsSent         metric.Int64Counter
	DBQueryDurationSec metric.Float64Histogram
}

var (
	instrumentsMu sync.RWMutex
	current       *instruments = noopInstruments()
)

// InitMetrics constructs every package-level instrument against the currently
// installed global MeterProvider. Call this AFTER RuntimeMetrics has set the
// provider (otherwise the instruments resolve against the no-op default and
// silently drop). InitMetrics is safe to invoke once; later calls overwrite
// the previously-stored handles.
//
// On construction failure of any single instrument the function logs a warning
// and substitutes a no-op handle for that field, so callers can always emit
// metrics unconditionally without nil-checking.
func InitMetrics() {
	meter := otel.Meter(meterName)
	noop := noopInstruments()

	i := *noop // start from no-op handles; replace each on success.

	if c, err := meter.Int64Counter(
		"orders_placed_total",
		metric.WithDescription("Orders successfully placed (post stock reservation, pre payment outcome)."),
		metric.WithUnit("1"),
	); err == nil {
		i.OrdersPlaced = c
	} else {
		logrus.WithError(err).Warn("observability: failed to create orders_placed counter")
	}

	if c, err := meter.Int64Counter(
		"orders_finalized_total",
		metric.WithDescription("Orders that reached a terminal/transition status (paid, failed, cancelled, shipped, delivered, refunded)."),
		metric.WithUnit("1"),
	); err == nil {
		i.OrdersFinalized = c
	} else {
		logrus.WithError(err).Warn("observability: failed to create orders_finalized counter")
	}

	if c, err := meter.Int64Counter(
		"payments_charged_total",
		metric.WithDescription("Successful payment captures."),
		metric.WithUnit("1"),
	); err == nil {
		i.PaymentsCharged = c
	} else {
		logrus.WithError(err).Warn("observability: failed to create payments_charged counter")
	}

	if c, err := meter.Int64Counter(
		"payments_failed_total",
		metric.WithDescription("Failed payment attempts (declines, gateway errors)."),
		metric.WithUnit("1"),
	); err == nil {
		i.PaymentsFailed = c
	} else {
		logrus.WithError(err).Warn("observability: failed to create payments_failed counter")
	}

	if c, err := meter.Int64Counter(
		"revenue_minor_units_total",
		metric.WithDescription("Captured revenue summed in the order's minor currency units."),
		metric.WithUnit("1"),
	); err == nil {
		i.RevenueMinorUnits = c
	} else {
		logrus.WithError(err).Warn("observability: failed to create revenue counter")
	}

	if c, err := meter.Int64Counter(
		"stock_reserved_total",
		metric.WithDescription("Variant-units atomically reserved by checkout."),
		metric.WithUnit("1"),
	); err == nil {
		i.StockReserved = c
	} else {
		logrus.WithError(err).Warn("observability: failed to create stock_reserved counter")
	}

	if c, err := meter.Int64Counter(
		"stock_released_total",
		metric.WithDescription("Variant-units returned to the catalogue (cancel, refund, expired pending, failed payment)."),
		metric.WithUnit("1"),
	); err == nil {
		i.StockReleased = c
	} else {
		logrus.WithError(err).Warn("observability: failed to create stock_released counter")
	}

	if c, err := meter.Int64Counter(
		"logins_total",
		metric.WithDescription("Login attempts grouped by outcome."),
		metric.WithUnit("1"),
	); err == nil {
		i.Logins = c
	} else {
		logrus.WithError(err).Warn("observability: failed to create logins counter")
	}

	if c, err := meter.Int64Counter(
		"registrations_total",
		metric.WithDescription("Customer registrations grouped by outcome."),
		metric.WithUnit("1"),
	); err == nil {
		i.Registrations = c
	} else {
		logrus.WithError(err).Warn("observability: failed to create registrations counter")
	}

	if c, err := meter.Int64Counter(
		"cart_items_added_total",
		metric.WithDescription("Variant additions to a cart (successful AddToCart calls)."),
		metric.WithUnit("1"),
	); err == nil {
		i.CartItemsAdded = c
	} else {
		logrus.WithError(err).Warn("observability: failed to create cart_items_added counter")
	}

	if c, err := meter.Int64Counter(
		"search_queries_total",
		metric.WithDescription("Product listing requests that carried a non-empty search term."),
		metric.WithUnit("1"),
	); err == nil {
		i.SearchQueries = c
	} else {
		logrus.WithError(err).Warn("observability: failed to create search_queries counter")
	}

	if c, err := meter.Int64Counter(
		"emails_sent_total",
		metric.WithDescription("Outbound emails grouped by kind and outcome."),
		metric.WithUnit("1"),
	); err == nil {
		i.EmailsSent = c
	} else {
		logrus.WithError(err).Warn("observability: failed to create emails_sent counter")
	}

	if h, err := meter.Float64Histogram(
		"db_query_duration_seconds",
		metric.WithDescription("Duration of selected hot DB queries, in seconds."),
		metric.WithUnit("s"),
	); err == nil {
		i.DBQueryDurationSec = h
	} else {
		logrus.WithError(err).Warn("observability: failed to create db_query_duration histogram")
	}

	instrumentsMu.Lock()
	current = &i
	instrumentsMu.Unlock()
}

// Metrics returns the currently-installed instrument set. It always returns a
// non-nil value; until InitMetrics runs the returned struct holds no-op
// handles so callers can call Add/Record unconditionally.
func Metrics() *instruments {
	instrumentsMu.RLock()
	defer instrumentsMu.RUnlock()
	return current
}

// noopInstruments builds an instrument set whose handles are all no-ops. Used
// before InitMetrics fires and as a fallback for individual instruments whose
// construction failed.
func noopInstruments() *instruments {
	noopMeter := otel.GetMeterProvider().Meter("gocommerce-noop")
	mustC := func(name string) metric.Int64Counter {
		c, err := noopMeter.Int64Counter(name)
		if err != nil {
			// Fall back to a counter built against the no-op provider's
			// default scope; constructing against a nil meter is not
			// possible, so we just log and continue with whatever we got.
			logrus.WithError(err).Warn("observability: failed to create noop counter " + name)
		}
		return c
	}
	mustH := func(name string) metric.Float64Histogram {
		h, err := noopMeter.Float64Histogram(name)
		if err != nil {
			logrus.WithError(err).Warn("observability: failed to create noop histogram " + name)
		}
		return h
	}
	return &instruments{
		OrdersPlaced:       mustC("noop_orders_placed"),
		OrdersFinalized:    mustC("noop_orders_finalized"),
		PaymentsCharged:    mustC("noop_payments_charged"),
		PaymentsFailed:     mustC("noop_payments_failed"),
		RevenueMinorUnits:  mustC("noop_revenue"),
		StockReserved:      mustC("noop_stock_reserved"),
		StockReleased:      mustC("noop_stock_released"),
		Logins:             mustC("noop_logins"),
		Registrations:      mustC("noop_registrations"),
		CartItemsAdded:     mustC("noop_cart_items_added"),
		SearchQueries:      mustC("noop_search_queries"),
		EmailsSent:         mustC("noop_emails_sent"),
		DBQueryDurationSec: mustH("noop_db_duration"),
	}
}

// Helpers below provide ergonomic call sites for the most common increment
// patterns. They are thin wrappers around metric.Add that read the current
// instrument set once and avoid leaking the package-level state to callers.

// OrdersPlacedInc records one placed order tagged by the chosen
// payment/shipping methods.
func OrdersPlacedInc(ctx context.Context, paymentMethod, shipMethod string) {
	Metrics().OrdersPlaced.Add(ctx, 1, metric.WithAttributes(
		attribute.String("payment_method", paymentMethod),
		attribute.String("ship_method", shipMethod),
	))
}

// OrdersFinalizedInc records one order state transition.
func OrdersFinalizedInc(ctx context.Context, status string) {
	Metrics().OrdersFinalized.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", status),
	))
}

// PaymentsChargedInc records a successful charge in the given currency.
func PaymentsChargedInc(ctx context.Context, currency string) {
	Metrics().PaymentsCharged.Add(ctx, 1, metric.WithAttributes(
		attribute.String("currency", currency),
	))
}

// PaymentsFailedInc records a payment failure.
func PaymentsFailedInc(ctx context.Context) {
	Metrics().PaymentsFailed.Add(ctx, 1)
}

// RevenueAdd accumulates captured revenue in minor units.
func RevenueAdd(ctx context.Context, amountMinor int64, currency string) {
	Metrics().RevenueMinorUnits.Add(ctx, amountMinor, metric.WithAttributes(
		attribute.String("currency", currency),
	))
}

// StockReservedAdd accumulates reserved variant-units.
func StockReservedAdd(ctx context.Context, units int64) {
	if units <= 0 {
		return
	}
	Metrics().StockReserved.Add(ctx, units)
}

// StockReleasedAdd accumulates released variant-units.
func StockReleasedAdd(ctx context.Context, units int64) {
	if units <= 0 {
		return
	}
	Metrics().StockReleased.Add(ctx, units)
}

// LoginsInc records a login attempt outcome ("success" or "failure").
func LoginsInc(ctx context.Context, outcome string) {
	Metrics().Logins.Add(ctx, 1, metric.WithAttributes(
		attribute.String("outcome", outcome),
	))
}

// RegistrationsInc records a registration attempt outcome.
func RegistrationsInc(ctx context.Context, outcome string) {
	Metrics().Registrations.Add(ctx, 1, metric.WithAttributes(
		attribute.String("outcome", outcome),
	))
}

// CartItemsAddedInc records one successful AddToCart.
func CartItemsAddedInc(ctx context.Context) {
	Metrics().CartItemsAdded.Add(ctx, 1)
}

// SearchQueriesInc records one product listing request that carried a search
// term.
func SearchQueriesInc(ctx context.Context) {
	Metrics().SearchQueries.Add(ctx, 1)
}

// EmailsSentInc records one outbound email by kind and outcome
// ("success" or "failure").
func EmailsSentInc(ctx context.Context, kind, outcome string) {
	Metrics().EmailsSent.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", kind),
		attribute.String("outcome", outcome),
	))
}
