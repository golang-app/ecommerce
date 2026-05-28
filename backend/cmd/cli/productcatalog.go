package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type productCatalog interface {
	Add(ctx context.Context, id, name, desc string, priceMinorUnits int64, currency, thumbnail string) error
	AddVariantProduct(ctx context.Context, id, name, desc, currency, thumbnail string, optionTypes []app.OptionTypeInput, variants []app.VariantInput) error
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
	var price int64
	var id, name, shortDesc, currency, thumbnail string
	var optionFlags, variantFlags []string

	cmd := &cobra.Command{
		Use:   "product-add",
		Short: "Add a product. Use --option/--variant to author variants, or --price for a simple product.",
		Long: `Add a product to the catalog.

Simple product (single price):
  product-add --id mug --name "Mug" --price 2400 --thumbnail URL

Product with variants (each variant priced independently):
  product-add --id mug --name "Mug" \
    --option "Color:Cream,Charcoal" \
    --variant "Color=Cream;2400;MUG-CRM;IMG_URL" \
    --variant "Color=Charcoal;2600;MUG-CHR;IMG_URL"

--option grammar:  "<Name>:<v1>,<v2>,..."
--variant grammar: "<opt1>=<val1>,<opt2>=<val2>;<priceMinorUnits>[;<sku>[;<imageURL>[;<stock>]]]"
  (leave the options part empty for a single default variant; stock defaults to 100)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if len(variantFlags) > 0 {
				optionTypes, err := parseOptionTypes(optionFlags)
				if err != nil {
					return err
				}
				variants, err := parseVariants(variantFlags, id)
				if err != nil {
					return err
				}
				return pc.AddVariantProduct(ctx, id, name, shortDesc, currency, thumbnail, optionTypes, variants)
			}

			if !cmd.Flags().Changed("price") {
				return fmt.Errorf("--price is required for a simple product (or pass --variant to author variants)")
			}
			return pc.Add(ctx, id, name, shortDesc, price, currency, thumbnail)
		},
	}

	cmd.Flags().StringVarP(&id, "id", "", "", "product id")
	if err := cmd.MarkFlagRequired("id"); err != nil {
		logrus.WithError(err).Error("cannot mark --id flag as required")
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "product name")
	if err := cmd.MarkFlagRequired("name"); err != nil {
		logrus.WithError(err).Error("cannot mark --name flag as required")
	}

	cmd.Flags().Int64VarP(&price, "price", "", 0, "product price in minor units (e.g. cents) — for a simple product")
	cmd.Flags().StringVarP(&currency, "currency", "c", "USD", "ISO 4217 currency code")
	cmd.Flags().StringVarP(&shortDesc, "short-desc", "s", "", "product short description")
	cmd.Flags().StringVarP(&thumbnail, "thumbnail", "t", "", "product thumbnail URL")
	cmd.Flags().StringArrayVar(&optionFlags, "option", nil, `option type, repeatable: "Color:Cream,Charcoal"`)
	cmd.Flags().StringArrayVar(&variantFlags, "variant", nil, `variant, repeatable: "Color=Cream;2400;SKU;IMG"`)

	return cmd
}

func parseOptionTypes(flags []string) ([]app.OptionTypeInput, error) {
	out := make([]app.OptionTypeInput, 0, len(flags))
	for _, f := range flags {
		name, valuesCSV, found := strings.Cut(f, ":")
		name = strings.TrimSpace(name)
		if !found || name == "" {
			return nil, fmt.Errorf("--option %q must be \"Name:v1,v2,...\"", f)
		}
		var values []string
		for _, v := range strings.Split(valuesCSV, ",") {
			if v = strings.TrimSpace(v); v != "" {
				values = append(values, v)
			}
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("--option %q has no values", f)
		}
		out = append(out, app.OptionTypeInput{Name: name, Values: values})
	}
	return out, nil
}

func parseVariants(flags []string, productID string) ([]app.VariantInput, error) {
	out := make([]app.VariantInput, 0, len(flags))
	for i, f := range flags {
		parts := strings.Split(f, ";")
		if len(parts) < 2 {
			return nil, fmt.Errorf("--variant %q must be \"opts;price[;sku[;image]]\"", f)
		}

		price, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("--variant %q: price must be an integer (minor units): %w", f, err)
		}

		var sku, image string
		stock := 100
		if len(parts) >= 3 {
			sku = strings.TrimSpace(parts[2])
		}
		if len(parts) >= 4 {
			image = strings.TrimSpace(parts[3])
		}
		if len(parts) >= 5 {
			s, err := strconv.Atoi(strings.TrimSpace(parts[4]))
			if err != nil {
				return nil, fmt.Errorf("--variant %q: stock must be an integer: %w", f, err)
			}
			stock = s
		}

		options := map[string]string{}
		if optsCSV := strings.TrimSpace(parts[0]); optsCSV != "" {
			for _, kv := range strings.Split(optsCSV, ",") {
				k, v, found := strings.Cut(kv, "=")
				if !found || strings.TrimSpace(k) == "" {
					return nil, fmt.Errorf("--variant %q: option %q must be \"Name=Value\"", f, kv)
				}
				options[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}

		vid := fmt.Sprintf("%s-%d", productID, i)
		if sku != "" {
			vid = productID + "-" + strings.ToLower(sku)
		}
		out = append(out, app.VariantInput{ID: vid, SKU: sku, Image: image, Options: options, Price: price, Stock: stock})
	}
	return out, nil
}
