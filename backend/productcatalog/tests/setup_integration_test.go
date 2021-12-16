//go:build integration

package tests

import "fmt"
import "github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
import "os"

func init() {
	pass := getEnv("POSTGRES_PASSWORD", "")
	var conn string

	if pass != "" {
		conn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", getEnv("POSTGRES_HOST", "localhost"), getEnv("POSTGRES_PORT", "5432"), getEnv("POSTGRES_USER", "bartlomiejklimczak"), pass, getEnv("POSTGRES_DB", "ecommerce"))
	} else {
		conn = fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable", getEnv("POSTGRES_HOST", "localhost"), getEnv("POSTGRES_PORT", "5432"), getEnv("POSTGRES_USER", "bartlomiejklimczak"), getEnv("POSTGRES_DB", "ecommerce"))
	}

	s, err := adapter.NewPostgres(conn)
	if err != nil {
		panic("cannot establish connection to postgres: " + err.Error())
	}

	storage = s
}

func getEnv(name, def string) string {
	v := os.Getenv(name)
	if v == "" {
		v = def
	}

	return v
}
