package main

import (
	"testing"
	"time"

	"github.com/ardanlabs/conf"
	"github.com/matryer/is"
)

func TestPostgresConfig_ConnectionString(t *testing.T) {
	is := is.New(t)

	cfg := postgresConfig{
		User:     "user1",
		Password: "secretpassword",
		Host:     "db.example.com",
		Port:     5432,
		Db:       "mydb",
		SSLMode:  "require",
	}

	conn := cfg.connectionString()
	is.Equal(conn, "host=db.example.com port=5432 user=user1 password=secretpassword dbname=mydb sslmode=require")

	// Default fallback to sslmode=disable when empty
	cfgDefault := postgresConfig{
		User: "user2",
		Host: "localhost",
		Port: 5432,
		Db:   "mydb",
	}
	is.Equal(cfgDefault.connectionString(), "host=localhost port=5432 user=user2 dbname=mydb sslmode=disable")
}

func TestPostgresConfig_Defaults(t *testing.T) {
	is := is.New(t)

	var cfg config
	err := conf.Parse([]string{}, "", &cfg)
	is.NoErr(err)

	is.Equal(cfg.Postgres.SSLMode, "disable")
	is.Equal(cfg.Postgres.MaxOpenConns, 25)
	is.Equal(cfg.Postgres.MaxIdleConns, 25)
	is.Equal(cfg.Postgres.ConnMaxLifetime, 15*time.Minute)
	is.Equal(cfg.Postgres.ConnMaxIdleTime, 5*time.Minute)
}
