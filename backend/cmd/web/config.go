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
