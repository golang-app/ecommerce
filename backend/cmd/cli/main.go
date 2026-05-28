package main

import (
	"errors"
	"fmt"

	"github.com/ardanlabs/conf"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
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
		logrus.WithError(err).Fatal("cannot parse configuration")
	}

	connString := cfg.Postgres.connectionString()
	db, err := otelsql.Open("postgres", connString)
	if err != nil {
		logrus.WithError(err).Fatal("cannot open connection to the DB")
	}

	defer func() { _ = db.Close() }()

	Execute(db)
}
