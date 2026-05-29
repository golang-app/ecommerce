package main

import (
	"database/sql"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/adapter"
	"github.com/spf13/cobra"
)

// newRebuildReadModelsCmd wipes the checkout read model (checkout_order +
// checkout_order_item) and the analytics_daily_sales projection, then
// replays the entire checkout_events log through projectEventTx within a
// single transaction. The exercise proves the read side is derived state:
// dropping it and re-projecting yields the same rows the live projection
// has been writing all along.
//
// The order matters: events for a given aggregate must be applied in
// sequence order so the analytics projection can SELECT the freshly-written
// checkout_order row when PaymentSucceeded fires. We order by aggregate_id
// then sequence so that within each aggregate the OrderPlaced row lands
// before any PaymentSucceeded; cross-aggregate interleaving is irrelevant
// because each aggregate's rows are independent.
func newRebuildReadModelsCmd(db *sql.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rebuild-readmodels",
		Short: "Replay the checkout event log into a fresh read model + analytics projection",
		Long: `Truncates checkout_order, checkout_order_item and
analytics_daily_sales, then walks checkout_events in
(aggregate_id, sequence) order, re-running the existing
projection for each event. Demonstrates that the read side
of a CQRS context is derived state — wiping and rebuilding
it does not change the source of truth.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return fmt.Errorf("begin tx: %w", err)
			}
			defer func() {
				if err != nil {
					_ = tx.Rollback()
				}
			}()

			if _, err = tx.ExecContext(ctx, `TRUNCATE TABLE checkout_order, checkout_order_item, analytics_daily_sales RESTART IDENTITY`); err != nil {
				return fmt.Errorf("truncate read models: %w", err)
			}

			eventCount, aggCount, err := adapter.RebuildReadModels(ctx, tx)
			if err != nil {
				return err
			}

			if err = tx.Commit(); err != nil {
				return fmt.Errorf("commit: %w", err)
			}

			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "rebuilt read models from %d events across %d aggregates\n", eventCount, aggCount); err != nil {
				return fmt.Errorf("write rebuild summary: %w", err)
			}
			return nil
		},
	}
	return cmd
}
