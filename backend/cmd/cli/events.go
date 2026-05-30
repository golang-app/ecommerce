package main

import (
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/internal/eventcatalog"
	"github.com/spf13/cobra"
)

// newEventsCmd is the `events` subcommand: it walks the hand-curated
// event catalog and prints it to stdout as living architecture
// documentation. The catalog itself lives in
// internal/eventcatalog so this command stays a thin Cobra wrapper.
//
// Flags:
//   --by-context  group the table by producing bounded context
//                  (one H2 + sub-table per producer) instead of a
//                  single flat sorted table.
func newEventsCmd() *cobra.Command {
	var byContext bool

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Print the catalog of domain and integration events as a Markdown table",
		Long: `Walks the hand-curated event catalog (internal/eventcatalog)
and prints it to stdout as a Markdown table. The default output is one
table sorted by event name; --by-context groups by producing context
instead. The artefact is intended as living architecture documentation
that is regenerated from the source.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			events := eventcatalog.Catalog()
			w := cmd.OutOrStdout()
			if byContext {
				if err := eventcatalog.RenderMarkdownByContext(w, events); err != nil {
					return fmt.Errorf("render by-context: %w", err)
				}
				return nil
			}
			if err := eventcatalog.RenderMarkdown(w, events); err != nil {
				return fmt.Errorf("render markdown: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&byContext, "by-context", false,
		"group the table into one section per producing bounded context")

	return cmd
}
