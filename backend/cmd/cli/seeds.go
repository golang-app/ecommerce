package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

// seedAdmin email/password for the demo admin account. The password is
// hashed with the same bcrypt cost the auth service uses (bcrypt.DefaultCost)
// so the seeded credentials work against the normal login flow.
const (
	seedAdminEmail    = "admin@example.com"
	seedAdminPassword = "Admin123!"
)

// upsertAdmin idempotently inserts (or refreshes) the demo admin account
// in the auth_admin table (the split admin aggregate; see migration
// 000038). must_change_password = true is always re-asserted on
// re-seed so the "change password on first login" gate is reliably
// testable when developers re-run `seeds`.
const upsertAdmin = `INSERT INTO auth_admin (id, email, password_hash, role, must_change_password)
	VALUES ($1, $1, $2, 'admin', true)
	ON CONFLICT (id) DO UPDATE SET
		email = EXCLUDED.email,
		password_hash = EXCLUDED.password_hash,
		role = EXCLUDED.role,
		must_change_password = true`

// seedAdminUser idempotently creates (or refreshes) the demo admin
// account. The post-split target is auth_admin, not auth_customer:
// admins live in their own aggregate now.
func seedAdminUser(ctx context.Context, db *sql.DB) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(seedAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	if _, err := db.ExecContext(ctx, upsertAdmin, seedAdminEmail, string(hash)); err != nil {
		return fmt.Errorf("seed admin user: %w", err)
	}
	return nil
}

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

type variantSeed struct {
	id          string
	name        string
	description string
	currency    string
	thumbnail   string
	optionTypes []app.OptionTypeInput
	variants    []app.VariantInput
}

// Per-colour apron images, shared across that colour's size variants so
// changing size keeps the image and changing colour swaps it.
const (
	natApron  = "https://loremflickr.com/800/800/apron,linen?lock=22"
	navyApron = "https://loremflickr.com/800/800/apron,navy?lock=222"
)

// variantSeeds are products with selectable options where each variant has
// its own price — the mug in two colours and the apron as a Colour×Size
// matrix.
var variantSeeds = []variantSeed{
	{
		id:          "ceramic-mug",
		name:        "Ceramic Mug",
		description: "A small-batch mug thrown by a single potter in Tokyo. Speckled stoneware, holds 350ml.",
		currency:    "USD",
		thumbnail:   "https://loremflickr.com/800/800/ceramic,mug?lock=11",
		optionTypes: []app.OptionTypeInput{
			{Name: "Color", Values: []string{"Cream", "Charcoal"}},
		},
		variants: []app.VariantInput{
			{ID: "ceramic-mug-cream", SKU: "MUG-CRM", Options: map[string]string{"Color": "Cream"}, Price: 2400, Image: "https://loremflickr.com/800/800/ceramic,mug,cream?lock=11", Stock: 40},
			{ID: "ceramic-mug-charcoal", SKU: "MUG-CHR", Options: map[string]string{"Color": "Charcoal"}, Price: 2600, Image: "https://loremflickr.com/800/800/ceramic,mug,black?lock=211", Stock: 0},
		},
	},
	{
		id:          "linen-apron",
		name:        "Linen Apron",
		description: "Heavyweight Belgian linen apron. Long cotton ties, double-stitched seams. Washes softer with every use.",
		currency:    "USD",
		thumbnail:   "https://loremflickr.com/800/800/apron,linen?lock=22",
		optionTypes: []app.OptionTypeInput{
			{Name: "Color", Values: []string{"Natural", "Navy"}},
			{Name: "Size", Values: []string{"S", "M", "L"}},
		},
		variants: []app.VariantInput{
			{ID: "linen-apron-nat-s", SKU: "APR-NAT-S", Options: map[string]string{"Color": "Natural", "Size": "S"}, Price: 5400, Image: natApron, Stock: 12},
			{ID: "linen-apron-nat-m", SKU: "APR-NAT-M", Options: map[string]string{"Color": "Natural", "Size": "M"}, Price: 5800, Image: natApron, Stock: 8},
			{ID: "linen-apron-nat-l", SKU: "APR-NAT-L", Options: map[string]string{"Color": "Natural", "Size": "L"}, Price: 6200, Image: natApron, Stock: 5},
			{ID: "linen-apron-navy-s", SKU: "APR-NVY-S", Options: map[string]string{"Color": "Navy", "Size": "S"}, Price: 5600, Image: navyApron, Stock: 10},
			{ID: "linen-apron-navy-m", SKU: "APR-NVY-M", Options: map[string]string{"Color": "Navy", "Size": "M"}, Price: 6000, Image: navyApron, Stock: 6},
			{ID: "linen-apron-navy-l", SKU: "APR-NVY-L", Options: map[string]string{"Color": "Navy", "Size": "L"}, Price: 6400, Image: navyApron, Stock: 3},
		},
	},
}

// wipeTables clears all product and cart data so seed re-runs are
// idempotent. CASCADE clears the variant/option-type and cart tables that
// reference the wiped products.
const wipeTables = `TRUNCATE TABLE
	productcatalog_product,
	cart_cart_item,
	cart_cart
RESTART IDENTITY CASCADE`

// attributeTypeSeed describes a predefined product attribute type. numeric
// attributes store their value in product_attribute.num_value; enum
// attributes store it in text_value.
type attributeTypeSeed struct {
	id         string
	name       string
	unit       string
	kind       string // "numeric" or "enum"
	filterable bool
	position   int
}

// attributeTypeSeeds are the predefined attribute types. origin is the
// non-filterable example so the UI can prove that non-filterable attributes
// display but are excluded from facet filtering.
var attributeTypeSeeds = []attributeTypeSeed{
	{id: "weight", name: "Weight", unit: "kg", kind: "numeric", filterable: true, position: 1},
	{id: "width", name: "Width", unit: "cm", kind: "numeric", filterable: true, position: 2},
	{id: "material", name: "Material", unit: "", kind: "enum", filterable: true, position: 3},
	{id: "origin", name: "Country of origin", unit: "", kind: "enum", filterable: false, position: 4},
}

// categorySeed describes a predefined product category. slug equals id.
type categorySeed struct {
	id       string
	name     string
	position int
}

var categorySeeds = []categorySeed{
	{id: "kitchen", name: "Kitchen", position: 1},
	{id: "office", name: "Office", position: 2},
	{id: "outdoor", name: "Outdoor", position: 3},
	{id: "living", name: "Living", position: 4},
}

// productCategorySeeds assigns each seeded product (simple + variant) to one
// or more categories. The model is many-to-many; a few products
// intentionally appear in two categories.
var productCategorySeeds = []struct {
	productID   string
	categoryIDs []string
}{
	{"brass-paperclips", []string{"office"}},
	{"walnut-serving-spoon", []string{"kitchen"}},
	{"wool-throw-charcoal", []string{"living"}},
	{"glass-carafe-1l", []string{"kitchen", "living"}}, // multi-category
	{"leather-notebook-a5", []string{"office"}},
	{"cast-iron-skillet-10in", []string{"kitchen", "outdoor"}}, // multi-category
	{"stoneware-vase-grey", []string{"living"}},
	{"cotton-tea-towels-set", []string{"kitchen"}},
	{"ceramic-mug", []string{"kitchen", "office"}}, // multi-category
	{"linen-apron", []string{"kitchen"}},
}

// productAttributeSeed assigns one attribute value to a product. Exactly one
// of num / text is meaningful depending on the attribute type's kind: numeric
// types use num (text empty), enum types use text (num ignored).
type productAttributeSeed struct {
	productID       string
	attributeTypeID string
	num             float64
	text            string
}

// productAttributeSeeds gives every product a sensible set of attribute
// values. Most have weight + material; a few also carry width. origin
// (non-filterable) is set on a handful of products only.
var productAttributeSeeds = []productAttributeSeed{
	// brass-paperclips
	{productID: "brass-paperclips", attributeTypeID: "weight", num: 0.05},
	{productID: "brass-paperclips", attributeTypeID: "material", text: "brass"},
	// walnut-serving-spoon
	{productID: "walnut-serving-spoon", attributeTypeID: "weight", num: 0.08},
	{productID: "walnut-serving-spoon", attributeTypeID: "material", text: "walnut"},
	{productID: "walnut-serving-spoon", attributeTypeID: "origin", text: "USA"},
	// wool-throw-charcoal
	{productID: "wool-throw-charcoal", attributeTypeID: "weight", num: 1.2},
	{productID: "wool-throw-charcoal", attributeTypeID: "material", text: "wool"},
	{productID: "wool-throw-charcoal", attributeTypeID: "origin", text: "New Zealand"},
	// glass-carafe-1l
	{productID: "glass-carafe-1l", attributeTypeID: "weight", num: 0.6},
	{productID: "glass-carafe-1l", attributeTypeID: "material", text: "borosilicate glass"},
	// leather-notebook-a5
	{productID: "leather-notebook-a5", attributeTypeID: "weight", num: 0.4},
	{productID: "leather-notebook-a5", attributeTypeID: "width", num: 15},
	{productID: "leather-notebook-a5", attributeTypeID: "material", text: "leather"},
	// cast-iron-skillet-10in
	{productID: "cast-iron-skillet-10in", attributeTypeID: "weight", num: 2.5},
	{productID: "cast-iron-skillet-10in", attributeTypeID: "width", num: 26},
	{productID: "cast-iron-skillet-10in", attributeTypeID: "material", text: "cast iron"},
	{productID: "cast-iron-skillet-10in", attributeTypeID: "origin", text: "USA"},
	// stoneware-vase-grey
	{productID: "stoneware-vase-grey", attributeTypeID: "weight", num: 0.9},
	{productID: "stoneware-vase-grey", attributeTypeID: "material", text: "stoneware"},
	// cotton-tea-towels-set
	{productID: "cotton-tea-towels-set", attributeTypeID: "weight", num: 0.3},
	{productID: "cotton-tea-towels-set", attributeTypeID: "material", text: "cotton"},
	// ceramic-mug
	{productID: "ceramic-mug", attributeTypeID: "weight", num: 0.35},
	{productID: "ceramic-mug", attributeTypeID: "material", text: "stoneware"},
	{productID: "ceramic-mug", attributeTypeID: "origin", text: "Japan"},
	// linen-apron
	{productID: "linen-apron", attributeTypeID: "weight", num: 0.45},
	{productID: "linen-apron", attributeTypeID: "width", num: 70},
	{productID: "linen-apron", attributeTypeID: "material", text: "linen"},
}

// storeSeed describes a predefined storefront facade. The two seeded
// stores model a US storefront on the canonical localhost host and a EU
// storefront on a subdomain alias — operators are expected to point an
// `eu.localhost` /etc/hosts entry at 127.0.0.1 during local dev. The
// rows are idempotent on re-seed via ON CONFLICT (id) DO UPDATE.
type storeSeed struct {
	id        string
	slug      string
	name      string
	currency  string
	host      string
	isDefault bool
	position  int
}

var storeSeeds = []storeSeed{
	{id: "us", slug: "us", name: "GoCommerce US", currency: "USD", host: "localhost:8080", isDefault: true, position: 1},
	{id: "eu", slug: "eu", name: "GoCommerce EU", currency: "EUR", host: "eu.localhost:8080", isDefault: false, position: 2},
}

const (
	insertAttributeType = `INSERT INTO productcatalog_attribute_type
		(id, name, unit, kind, filterable, position)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			unit = EXCLUDED.unit,
			kind = EXCLUDED.kind,
			filterable = EXCLUDED.filterable,
			position = EXCLUDED.position`

	insertCategory = `INSERT INTO productcatalog_category
		(id, name, slug, position)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			slug = EXCLUDED.slug,
			position = EXCLUDED.position`

	insertProductCategory = `INSERT INTO productcatalog_product_category
		(product_id, category_id)
		VALUES ($1, $2)
		ON CONFLICT (product_id, category_id) DO NOTHING`

	insertProductAttributeNumeric = `INSERT INTO productcatalog_product_attribute
		(product_id, attribute_type_id, num_value, text_value)
		VALUES ($1, $2, $3, NULL)
		ON CONFLICT (product_id, attribute_type_id) DO UPDATE SET
			num_value = EXCLUDED.num_value,
			text_value = EXCLUDED.text_value`

	insertProductAttributeEnum = `INSERT INTO productcatalog_product_attribute
		(product_id, attribute_type_id, num_value, text_value)
		VALUES ($1, $2, NULL, $3)
		ON CONFLICT (product_id, attribute_type_id) DO UPDATE SET
			num_value = EXCLUDED.num_value,
			text_value = EXCLUDED.text_value`

	// upsertStore re-asserts every editable field of a store row on
	// re-seed. The is_default flag is set explicitly here too — the
	// migration's partial unique index plus the storeSeeds ordering
	// (us first, default=true) is what guarantees exactly one default
	// row after the loop completes.
	upsertStore = `INSERT INTO store
		(id, slug, name, currency, host, is_default, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			slug = EXCLUDED.slug,
			name = EXCLUDED.name,
			currency = EXCLUDED.currency,
			host = EXCLUDED.host,
			is_default = EXCLUDED.is_default,
			position = EXCLUDED.position`
)

// seedStores idempotently inserts (or updates) the configured
// storefront facades. To honour the partial unique index on
// (is_default) WHERE is_default, any other row's is_default flag is
// cleared before the upsert that introduces a new default.
func seedStores(ctx context.Context, db *sql.DB) error {
	for _, s := range storeSeeds {
		if s.isDefault {
			if _, err := db.ExecContext(ctx, `UPDATE store SET is_default = false WHERE id <> $1`, s.id); err != nil {
				return fmt.Errorf("clear existing defaults before seeding %s: %w", s.id, err)
			}
		}
		if _, err := db.ExecContext(ctx, upsertStore,
			s.id, s.slug, s.name, s.currency, s.host, s.isDefault, s.position); err != nil {
			return fmt.Errorf("seed store %s: %w", s.id, err)
		}
	}
	return nil
}

// countCategoryAssignments returns the total number of (product, category)
// pairs across all products — a product in two categories counts twice.
func countCategoryAssignments() int {
	n := 0
	for _, pc := range productCategorySeeds {
		n += len(pc.categoryIDs)
	}
	return n
}

// seedReferenceData idempotently inserts attribute types, categories, and the
// per-product category/attribute assignments. It must run AFTER the products
// exist because product_category/product_attribute reference
// productcatalog_product.
func seedReferenceData(ctx context.Context, db *sql.DB) error {
	for _, at := range attributeTypeSeeds {
		if _, err := db.ExecContext(ctx, insertAttributeType,
			at.id, at.name, at.unit, at.kind, at.filterable, at.position); err != nil {
			return fmt.Errorf("seed attribute type %s: %w", at.id, err)
		}
	}

	for _, c := range categorySeeds {
		if _, err := db.ExecContext(ctx, insertCategory,
			c.id, c.name, c.id, c.position); err != nil {
			return fmt.Errorf("seed category %s: %w", c.id, err)
		}
	}

	for _, pc := range productCategorySeeds {
		for _, catID := range pc.categoryIDs {
			if _, err := db.ExecContext(ctx, insertProductCategory,
				pc.productID, catID); err != nil {
				return fmt.Errorf("seed product-category %s/%s: %w", pc.productID, catID, err)
			}
		}
	}

	kindByType := make(map[string]string, len(attributeTypeSeeds))
	for _, at := range attributeTypeSeeds {
		kindByType[at.id] = at.kind
	}

	for _, pa := range productAttributeSeeds {
		switch kindByType[pa.attributeTypeID] {
		case "numeric":
			if _, err := db.ExecContext(ctx, insertProductAttributeNumeric,
				pa.productID, pa.attributeTypeID, pa.num); err != nil {
				return fmt.Errorf("seed product-attribute %s/%s: %w", pa.productID, pa.attributeTypeID, err)
			}
		default: // enum
			if _, err := db.ExecContext(ctx, insertProductAttributeEnum,
				pa.productID, pa.attributeTypeID, pa.text); err != nil {
				return fmt.Errorf("seed product-attribute %s/%s: %w", pa.productID, pa.attributeTypeID, err)
			}
		}
	}

	return nil
}

// seededProductIDsInOrder returns the seeded product ids in the order they are
// inserted (simple products first, then variant products).
func seededProductIDsInOrder() []string {
	ids := make([]string, 0, len(seedProducts)+len(variantSeeds))
	for _, p := range seedProducts {
		ids = append(ids, p.id)
	}
	for _, p := range variantSeeds {
		ids = append(ids, p.id)
	}
	return ids
}

// seedCreatedAt makes the catalogue ordering deterministic so "new arrivals"
// is stable: each seeded product's created_at is set to now() minus a per-row
// day offset. The first-seeded product gets the largest offset (oldest) and
// the last-seeded product the smallest (newest), so admin-created products
// (which default to now()) sort newest of all. Idempotent: re-running simply
// resets the timestamps.
func seedCreatedAt(ctx context.Context, db *sql.DB) error {
	ids := seededProductIDsInOrder()
	n := len(ids)
	for i, id := range ids {
		offset := n - i // first row => largest offset (oldest)
		if _, err := db.ExecContext(ctx,
			`UPDATE productcatalog_product SET created_at = now() - make_interval(days => $2) WHERE id = $1`,
			id, offset); err != nil {
			return fmt.Errorf("seed created_at for %s: %w", id, err)
		}
	}
	return nil
}

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

			for _, p := range variantSeeds {
				if err := pc.AddVariantProduct(ctx, p.id, p.name, p.description, p.currency, p.thumbnail, p.optionTypes, p.variants); err != nil {
					return fmt.Errorf("seed variant product %s: %w", p.id, err)
				}
			}

			if err := seedCreatedAt(ctx, db); err != nil {
				return err
			}

			if err := seedReferenceData(ctx, db); err != nil {
				return err
			}

			if err := seedAdminUser(ctx, db); err != nil {
				return err
			}

			if err := seedStores(ctx, db); err != nil {
				return err
			}

			fmt.Printf("seeded %d simple + %d variant products, %d attribute types, %d categories, %d category assignments, %d attribute values, %d stores, admin user %s (password reset required on first login)\n",
				len(seedProducts), len(variantSeeds), len(attributeTypeSeeds), len(categorySeeds),
				countCategoryAssignments(), len(productAttributeSeeds), len(storeSeeds), seedAdminEmail)
			return nil
		},
	}

	return cmd
}
