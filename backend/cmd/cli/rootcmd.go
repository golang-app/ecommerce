package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ecommerce",
	Short: "A CLI for the ecommerce app",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func Execute(db *sql.DB) {
	storage := adapter.NewPostgres(db)
	// CLI catalogue writes do not maintain the search index — the CLI's
	// `reindex` subcommand owns the index lifecycle separately, building
	// its own search.New(db). Wiring the NoopSearchIndexer keeps every
	// other subcommand index-agnostic.
	appServ := app.NewProductService(storage).WithSearchIndexer(app.NoopSearchIndexer)
	rootCmd.AddCommand(newProductCatalogCmd(appServ))
	rootCmd.AddCommand(newSeedsCmd(appServ, db))
	rootCmd.AddCommand(newReindexCmd(db))
	rootCmd.AddCommand(newRebuildReadModelsCmd(db))
	rootCmd.AddCommand(newEventsCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
