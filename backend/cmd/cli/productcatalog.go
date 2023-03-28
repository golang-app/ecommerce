package main

import (
	"context"
	"log"

	"github.com/spf13/cobra"
)

type productCatalog interface {
	Add(ctx context.Context, id, name, desc string, price float64, currency string) error
}

func newProductCatalogCmd(pc productCatalog) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "productcatalog",
		Short: "Manage the product catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	cmd.AddCommand(newProductCatalogAddCmd(pc))

	return cmd
}

func newProductCatalogAddCmd(pc productCatalog) *cobra.Command {
	price := 0.0
	var id, name, shortDesc, currency string

	cmd := &cobra.Command{
		Use: "product-add",
		RunE: func(cmd *cobra.Command, args []string) error {

			err := pc.Add(cmd.Context(), id, name, shortDesc, price, currency)
			return err
		},
	}

	cmd.Flags().StringVarP(&id, "id", "", "", "product id")
	if err := cmd.MarkFlagRequired("id"); err != nil {
		log.Print(err)
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "product name")
	if err := cmd.MarkFlagRequired("name"); err != nil {
		log.Print(err)
	}

	cmd.Flags().Float64VarP(&price, "price", "", 0, "product price")
	if err := cmd.MarkFlagRequired("price"); err != nil {
		log.Print(err)
	}

	cmd.Flags().StringVarP(&currency, "currency", "c", "USD", "product price")
	if err := cmd.MarkFlagRequired("currency"); err != nil {
		log.Print(err)
	}

	cmd.Flags().StringVarP(&shortDesc, "short-desc", "s", "", "product short description")

	return cmd
}
