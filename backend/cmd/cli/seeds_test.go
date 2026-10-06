package main

import (
	"testing"

	"github.com/matryer/is"
	"golang.org/x/crypto/bcrypt"
)

func TestSeedCustomerUser_Credentials(t *testing.T) {
	is := is.New(t)

	is.Equal(seedCustomerEmail, "customer@example.com")
	is.Equal(seedCustomerPassword, "Customer123!")

	hash, err := bcrypt.GenerateFromPassword([]byte(seedCustomerPassword), bcrypt.DefaultCost)
	is.NoErr(err)

	err = bcrypt.CompareHashAndPassword(hash, []byte("Customer123!"))
	is.NoErr(err)
}
