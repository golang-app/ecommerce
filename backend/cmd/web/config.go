package main

import "fmt"

type config struct {
	ServerPort int `default:"8080" envconfig:"SERVER_PORT"`
	Postgres   postgresConfig
}

type postgresConfig struct {
	User     string `default:"postgres"`
	Password string
	Port     int    `default:"5432"`
	Host     string `default:"localhost"`
	Db       string `default:"ecommerce"`
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
