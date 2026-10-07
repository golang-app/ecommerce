package main

import (
	"fmt"
	"time"
)

type config struct {
	ServerPort int `conf:"default:8080,SERVER_PORT"`
	Postgres   postgresConfig
	Env        string `conf:"default:dev"`
	// UploadsDir is the host-side directory the disk image store writes to.
	// It is also the directory the /uploads/* HTTP route serves from. The
	// container default lines up with the docker-compose bind mount.
	UploadsDir string `conf:"default:/uploads"`
	// StorageBackend selects the image storage provider: "disk" (default) or "s3".
	StorageBackend string `conf:"default:disk,STORAGE_BACKEND"`
	// S3Bucket is the target cloud object storage bucket for uploaded images.
	S3Bucket string `conf:"STORAGE_S3_BUCKET"`
	// S3Region is the cloud region for the object storage bucket (e.g. us-east-1).
	S3Region string `conf:"default:us-east-1,STORAGE_S3_REGION"`
	// S3Endpoint is an optional custom endpoint (for MinIO, Cloudflare R2, LocalStack).
	S3Endpoint string `conf:"STORAGE_S3_ENDPOINT"`
	// S3AccessKey is the access key ID for object storage authentication.
	S3AccessKey string `conf:"STORAGE_S3_ACCESS_KEY"`
	// S3SecretKey is the secret access key for object storage authentication.
	S3SecretKey string `conf:"mask,STORAGE_S3_SECRET_KEY"`
	// S3CDNURL is the public CDN base URL used to serve images (e.g. https://cdn.example.com).
	S3CDNURL string `conf:"STORAGE_S3_CDN_URL"`
	// S3ForcePathStyle enables path-style S3 URLs (e.g. http://endpoint/bucket/key).
	S3ForcePathStyle bool `conf:"default:false,STORAGE_S3_FORCE_PATH_STYLE"`
	// SessionSecret is the symmetric key gorilla/sessions uses to authenticate
	// cookies. It MUST be set to a strong random value (32+ bytes from a CSPRNG)
	// in production via the SESSION_SECRET env var. The default below is a
	// placeholder that allows the app to start in dev but emits a loud WARN on
	// every boot; in production (Env == "prod" / "production") the app refuses
	// to start with the default value.
	SessionSecret string `conf:"default:dev-only-do-not-use-in-production"`
	// CookieSecure controls the Secure flag on every session cookie. Set
	// COOKIE_SECURE=true when serving over HTTPS (production); leave false for
	// plain-HTTP local/docker-compose development so the browser still accepts
	// the cookie.
	CookieSecure bool `conf:"default:false"`
	// CSRFEnabled toggles the request-level CSRF check. It defaults to true
	// — production never wants this off. The only legitimate reason to set
	// CSRF_ENABLED=false is a short-lived local debugging session where you
	// want to curl/wget unsafe endpoints without minting a token first; do
	// NOT ship a deployment with this disabled.
	CSRFEnabled bool `conf:"default:true"`
	// PProfEnabled controls whether the pprof debugging HTTP server is started.
	// It defaults to false — production never wants this on because pprof endpoints
	// allow unauthenticated heap/CPU profiling and data leakage. Set PPROF_ENABLED=true
	// only during local debugging or internal diagnostics.
	PProfEnabled bool `conf:"default:false,PPROF_ENABLED"`
	// AppDemo enables Demo Mode across the storefront and admin panel.
	// Defaults to false. Set APP_DEMO=true to activate.
	AppDemo bool `conf:"default:false,APP_DEMO"`
	// SMTPHost is the host:port of the outbound SMTP relay (e.g. mailhog:1025
	// in dev, an SES/SendGrid SMTP endpoint in production). Leave blank to
	// disable real delivery — the app then falls back to a LogMailer that
	// writes the rendered email to the structured log. The fallback emits a
	// loud WARN at boot so a forgotten SMTP_HOST in staging cannot go
	// unnoticed.
	SMTPHost string
	// SMTPUsername / SMTPPassword authenticate against the relay. When
	// Username is empty, no auth is sent (MailHog accepts unauthenticated
	// SMTP). The password is masked in `conf` usage output.
	SMTPUsername string
	SMTPPassword string `conf:"mask"`
	// MailFrom is the default From: header on outbound mail. Caller-supplied
	// Message.From overrides it; otherwise every email is sent as MailFrom.
	MailFrom string `conf:"default:no-reply@gocommerce.local"`
	// BaseURL is the absolute base URL of the storefront, used to render
	// absolute links inside emails (the order detail link, the password
	// reset link). Defaulting to localhost:8080 keeps `docker compose up`
	// working out of the box; production deployments MUST override.
	BaseURL string `conf:"default:http://localhost:8080"`
	// ReservationTTL is how long a pending order may hold its stock
	// reservation before the sweeper expires it. Tune to match the longest
	// legitimate payment confirmation window for your processor; orders
	// abandoned past this point have their stock released back to the
	// catalogue. 30 minutes is a safe default for synchronous card flows.
	ReservationTTL time.Duration `conf:"default:30m"`
	// ReservationSweepInterval is how often the reservation TTL sweeper
	// runs. Set to 0 (or any non-positive value) to disable the sweeper
	// entirely. Default keeps lag low without hammering the read side.
	ReservationSweepInterval time.Duration `conf:"default:5m"`
	// OutboxInterval is how often the Transactional Outbox dispatcher
	// polls outbox_event for unsent rows and republishes them onto the
	// in-process bus. A non-positive value disables the dispatcher
	// entirely (integration events stay durable in the table but no
	// subscriber will be notified — useful only for offline diagnostics).
	// Default keeps the publish lag sub-second under normal load.
	OutboxInterval time.Duration `conf:"default:1s"`
	// TaxRatePercent is the flat tax rate applied to every order's subtotal
	// at checkout time, expressed as a percentage (e.g. 8.875 for 8.875%).
	// Defaults to 0, i.e. tax-free; tax is applied to the subtotal and is
	// part of the historical order via OrderPlaced.Tax.
	TaxRatePercent float64 `conf:"default:0"`
	// FreeShippingThreshold is the minimum order subtotal (in minor units —
	// e.g. cents) at or above which the chosen shipping method's cost is
	// overridden to 0 at place time. 0 disables the threshold.
	FreeShippingThreshold int64 `conf:"default:0"`
	// DefaultCurrency is the ISO 4217 code orders are placed in — the
	// storage currency. Every Price in the system is assumed to be
	// denominated in it; the display-only currency picker converts FROM
	// this code TO whatever the shopper selected.
	DefaultCurrency string `conf:"default:USD"`
	// SupportedCurrencies is a comma-separated list of ISO 4217 codes the
	// header picker offers (e.g. "USD,EUR,GBP"). The DefaultCurrency is
	// always prepended if missing. When only the default is listed (the
	// out-of-the-box value), the picker stays hidden.
	SupportedCurrencies string `conf:"default:USD"`
	// FXRates are the static, operator-configured conversion multipliers
	// FROM DefaultCurrency TO each supported display currency, in
	// "CCY:rate" pairs (e.g. "EUR:0.92,GBP:0.79"). They are NOT a live
	// feed — operators must update them by hand when the market drifts.
	// Upgrading to a real provider only requires a different
	// implementation of internal/fx.Rates; every storefront template and
	// the /currency handler talk through that single seam.
	FXRates string
	// StripeWebhookSecret is the shared secret used to verify the HMAC
	// signature on inbound payment webhooks (Stripe-Signature header).
	// Defaults to a dev placeholder so the local stack can boot; the
	// payments webhook route is skipped entirely if the secret is left
	// blank. Production operators MUST override.
	StripeWebhookSecret string `conf:"default:whsec_dev_only_do_not_use_in_production,STRIPE_WEBHOOK_SECRET"`
	// StripeFailCardEndingIn drives the fake provider's failure mode.
	// Set it to e.g. "0000" so a card token ending in 0000 triggers a
	// declined charge (status "failed"); leave empty and every charge
	// the fake provider sees succeeds. Dev/test only — production
	// payment failures come from the real provider's logic.
	StripeFailCardEndingIn string `conf:"default:0000,STRIPE_FAIL_CARD_ENDING_IN"`
	// TrustedProxies is a comma-separated list of CIDR ranges or IP addresses
	// of trusted reverse proxies. When empty, defaults to loopback and private networks.
	TrustedProxies string `conf:"TRUSTED_PROXIES"`
	// PaymentGateway selects the payment gateway provider: "fakestripe" (default) or "stripe".
	PaymentGateway string `conf:"default:fakestripe,PAYMENT_GATEWAY"`
	// StripeSecretKey is the secret API key used for authenticating with Stripe's PaymentIntents API.
	StripeSecretKey string `conf:"mask,STRIPE_SECRET_KEY"`
	// StripePublishableKey is the publishable key rendered on checkout pages for client-side tokenization.
	StripePublishableKey string `conf:"STRIPE_PUBLISHABLE_KEY"`
}

// defaultSessionSecret is the placeholder value SessionSecret must NOT keep
// in production. Exported via a constant so main.go can compare against it
// without re-typing the literal.
const defaultSessionSecret = "dev-only-do-not-use-in-production"

type postgresConfig struct {
	User            string        `conf:"default:postgres"`
	Password        string        `conf:"default:postgres"`
	Port            int           `conf:"default:5432"`
	Host            string        `conf:"default:localhost"`
	Db              string        `conf:"default:ecommerce"`
	SSLMode         string        `conf:"default:disable"`
	MaxOpenConns    int           `conf:"default:25"`
	MaxIdleConns    int           `conf:"default:25"`
	ConnMaxLifetime time.Duration `conf:"default:15m"`
	ConnMaxIdleTime time.Duration `conf:"default:5m"`
}

func (pc postgresConfig) connectionString() string {
	var conn string
	sslMode := pc.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	if pc.Password != "" {
		conn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s", pc.Host, pc.Port, pc.User, pc.Password, pc.Db, sslMode)
	} else {
		conn = fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=%s", pc.Host, pc.Port, pc.User, pc.Db, sslMode)
	}

	return conn
}
