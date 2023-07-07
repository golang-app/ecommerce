package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
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
	storage := productcatalog.NewPostgres(db)
	appServ := productcatalog.NewProductService(storage)
	rootCmd.AddCommand(newProductCatalogCmd(appServ))
	rootCmd.AddCommand(newSeedsCmd(appServ))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
