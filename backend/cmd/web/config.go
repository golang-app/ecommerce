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
