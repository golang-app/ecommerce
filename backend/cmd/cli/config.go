package main

import "fmt"

type config struct {
	Postgres postgresConfig
}

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
