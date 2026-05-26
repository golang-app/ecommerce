package main

import (
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"
)

type seedProduct struct {
	id              string
	name            string
	description     string
	priceMinorUnits int64
	currency        string
	thumbnail       string
}

// seedProducts is a hand-picked catalogue of artisan home goods. Prices are
// stored in minor units (cents). Thumbnail URLs use loremflickr with a fixed
// `lock` per product so each row consistently returns the same real Flickr
// photo matching the keywords. If a specific photo doesn't look right, bump
// the lock number for that product.
var seedProducts = []seedProduct{
	{
		id:              "ceramic-mug-cream",
		name:            "Ceramic Mug",
		description:     "A small-batch mug thrown by a single potter in Tokyo. Cream glaze over speckled stoneware. Holds 350ml. Sold individually.",
		priceMinorUnits: 2400,
		currency:        "USD",
		thumbnail:       "https://loremflickr.com/800/800/ceramic,mug?lock=11",
	},
	{
		id:              "linen-apron-navy",
		name:            "Linen Apron",
		description:     "Heavyweight Belgian linen apron in deep navy. Long cotton ties, double-stitched seams. Washes softer with every use.",
		priceMinorUnits: 5800,
		currency:        "USD",
		thumbnail:       "https://loremflickr.com/800/800/apron,linen?lock=22",
	},
	{
		id:              "brass-paperclips",
		name:            "Brass Paperclips",
		description:     "A set of twelve solid brass clips in a small kraft box. Patina deepens over time.",
		priceMinorUnits: 900,
		currency:        "USD",
		thumbnail:       "https://loremflickr.com/800/800/paperclip,brass?lock=33",
	},
	{
		id:              "walnut-serving-spoon",
		name:            "Walnut Serving Spoon",
		description:     "Hand-carved from a single piece of black walnut. Finished with food-safe oil. 25cm long.",
		priceMinorUnits: 1800,
		currency:        "USD",
		thumbnail:       "https://loremflickr.com/800/800/wooden,spoon?lock=44",
	},
	{
		id:              "wool-throw-charcoal",
		name:            "Wool Throw",
		description:     "100% New Zealand wool blanket in charcoal. Edges left raw with a short fringe. 130 x 180cm.",
		priceMinorUnits: 14500,
		currency:        "USD",
		thumbnail:       "https://loremflickr.com/800/800/wool,blanket?lock=55",
	},
	{
		id:              "glass-carafe-1l",
		name:            "Glass Carafe",
		description:     "Mouth-blown borosilicate carafe with a flat cork stopper. Holds one litre. Dishwasher-safe.",
		priceMinorUnits: 4200,
		currency:        "USD",
		thumbnail:       "https://loremflickr.com/800/800/carafe,water?lock=66",
	},
	{
		id:              "leather-notebook-a5",
		name:            "Leather Notebook",
		description:     "Vegetable-tanned cover wrapping 192 pages of unlined cream paper. Bound flat so it stays open on a desk.",
		priceMinorUnits: 3600,
		currency:        "USD",
		thumbnail:       "https://loremflickr.com/800/800/leather,notebook?lock=77",
	},
	{
		id:              "cast-iron-skillet-10in",
		name:            "Cast Iron Skillet",
		description:     "A 10-inch pre-seasoned cast iron pan with a helper handle. American foundry. One of those buy-it-once tools.",
		priceMinorUnits: 8900,
		currency:        "USD",
		thumbnail:       "https://loremflickr.com/800/800/castiron,skillet?lock=88",
	},
	{
		id:              "stoneware-vase-grey",
		name:            "Stoneware Vase",
		description:     "Matte-glaze stoneware vase in dove grey. Built for a single stem or a short bunch. 18cm tall.",
		priceMinorUnits: 3200,
		currency:        "USD",
		thumbnail:       "https://loremflickr.com/800/800/vase,pottery?lock=99",
	},
	{
		id:              "cotton-tea-towels-set",
		name:            "Cotton Tea Towels",
		description:     "A set of three loose-weave cotton towels in natural, sand, and stone. Absorbent from the first wash.",
		priceMinorUnits: 2200,
		currency:        "USD",
		thumbnail:       "https://loremflickr.com/800/800/teatowel,kitchen?lock=110",
	},
}

// wipeTables clears all product and cart data so seed re-runs are
// idempotent. CASCADE keeps the cart tables consistent with the wiped
// products.
const wipeTables = `TRUNCATE TABLE
	productcatalog_product,
	cart_cart_item,
	cart_cart
RESTART IDENTITY CASCADE`

func newSeedsCmd(pc productCatalog, db *sql.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seeds",
		Short: "populate the database with a fresh demo catalogue",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if _, err := db.ExecContext(ctx, wipeTables); err != nil {
				return fmt.Errorf("wipe failed: %w", err)
			}

			for _, p := range seedProducts {
				if err := pc.Add(ctx, p.id, p.name, p.description, p.priceMinorUnits, p.currency, p.thumbnail); err != nil {
					return fmt.Errorf("seed %s: %w", p.id, err)
				}
			}

			fmt.Printf("seeded %d products\n", len(seedProducts))
			return nil
		},
	}

	return cmd
}
