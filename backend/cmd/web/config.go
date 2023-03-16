package main

import "fmt"

type config struct {
	ServerPort int `conf:"default:8080,SERVER_PORT"`
	Postgres   postgresConfig
	Env        string `conf:"default:dev"`
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
