package adapter

import (
	"context"
	"database/sql"
	"fmt"
)

// RebuildReadModels walks every row of checkout_events in (aggregate_id,
// sequence) order, decodes each event, and applies projectEventTx to the
// supplied transaction. Returns the number of events replayed and the
// number of distinct aggregates encountered. The caller owns the tx — it
// should TRUNCATE the read-model tables (checkout_order, checkout_order_item,
// analytics_daily_sales) before invoking this and Commit on success.
//
// Why in-package: projectEventTx and unmarshalEvent are unexported (they
// guard the codec/projection seam from leaking into the CLI). Exposing this
// orchestrator keeps the truncation choice with the operator script while
// reusing the same projection code that the live Save path uses, so the
// rebuild is deterministic by construction.
func RebuildReadModels(ctx context.Context, tx *sql.Tx) (int, int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT aggregate_id, event_type, payload
		FROM checkout_events
		ORDER BY aggregate_id, sequence
	`)
	if err != nil {
		return 0, 0, fmt.Errorf("query events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	eventCount := 0
	aggCount := 0
	var lastAgg string
	for rows.Next() {
		var aggID, eventType string
		var payload []byte
		if err := rows.Scan(&aggID, &eventType, &payload); err != nil {
			return eventCount, aggCount, fmt.Errorf("scan event: %w", err)
		}
		if aggID != lastAgg {
			aggCount++
			lastAgg = aggID
		}
		e, err := unmarshalEvent(eventType, payload)
		if err != nil {
			return eventCount, aggCount, err
		}
		if err := projectEventTx(ctx, tx, e); err != nil {
			return eventCount, aggCount, fmt.Errorf("project %s for %s: %w", eventType, aggID, err)
		}
		eventCount++
	}
	if err := rows.Err(); err != nil {
		return eventCount, aggCount, fmt.Errorf("rows: %w", err)
	}
	return eventCount, aggCount, nil
}
