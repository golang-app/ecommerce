package main

import (
	"log"

	"github.com/kelseyhightower/envconfig"
)

func main() {
	cfg := config{}

	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal(err)
	}
}
