package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSeedsCmd(pc productCatalog) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seeds",
		Short: "populate the database with initial data",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			for i := 0; i < 3; i++ {
				pID := fmt.Sprintf("product-%d", i)
				name := fmt.Sprintf("Product %d", i)
				desc := fmt.Sprintf("Product %d", i)

				if err := pc.Add(ctx, pID, name, desc, 100, "USD"); err != nil {
					return err
				}

			}

			return nil
		},
	}

	return cmd
}
