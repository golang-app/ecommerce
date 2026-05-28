package main

import "fmt"

type config struct {
	ServerPort int `conf:"default:8080,SERVER_PORT"`
	Postgres   postgresConfig
	Env        string `conf:"default:dev"`
	// UploadsDir is the host-side directory the disk image store writes to.
	// It is also the directory the /uploads/* HTTP route serves from. The
	// container default lines up with the docker-compose bind mount.
	UploadsDir string `conf:"default:/uploads"`
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
}

// defaultSessionSecret is the placeholder value SessionSecret must NOT keep
// in production. Exported via a constant so main.go can compare against it
// without re-typing the literal.
const defaultSessionSecret = "dev-only-do-not-use-in-production"

type postgresConfig struct {
	User     string `conf:"default:postgres"`
	Password string `conf:"default:postgres"`
	Port     int    `conf:"default:5432"`
	Host     string `conf:"default:localhost"`
	Db       string `conf:"default:ecommerce"`
}

func (pc postgresConfig) connectionString() string {
	var conn string

	if pc.Password != "" {
		conn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", pc.Host, pc.Port, pc.User, pc.Password, pc.Db)
	} else {
		conn = fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable", pc.Host, pc.Port, pc.User, pc.Db)
	}

	return conn
}
