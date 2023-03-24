package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/ardanlabs/conf"
	_ "github.com/lib/pq"
	"github.com/uptrace/opentelemetry-go-extra/otelsql"
)

func main() {
	cfg := config{}

	err := conf.Parse([]string{}, "", &cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Println(conf.Usage("", &cfg))
			return
		}
		log.Fatal(err)
	}

	connString := cfg.Postgres.connectionString()
	db, err := otelsql.Open("postgres", connString)
	if err != nil {
		log.Fatalf("cannot open connection to the DB: %s", err)
	}

	defer db.Close()

	Execute(db) 
}
