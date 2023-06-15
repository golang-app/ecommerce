package auth_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/auth/app"
	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/matryer/is"
)

var authStorage app.CustomerStorage
var sessStorage app.SessStorage

func TestCreateNewCustomer(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	// given
	email := randomEmail()
	pass := randomPassword()
	appServ := app.NewAuth(authStorage, sessStorage)

	// when
	err := appServ.CreateNewCustomer(ctx, email, pass)

	// then
	is.NoErr(err)
}

func TestRegisterMultipleTimesWithTheSameEmail(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	// given
	email := randomEmail()
	pass := randomPassword()
	appServ := app.NewAuth(authStorage, sessStorage)

	// when
	err := appServ.CreateNewCustomer(ctx, email, pass)

	// then
	is.NoErr(err)

	// when
	err = appServ.CreateNewCustomer(ctx, email, pass)

	// then
	is.True(errors.Is(err, domain.ErrCustomerExists))
}

func TestLoginToAlreadyExistingAccount(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	// given
	email := randomEmail()
	pass := randomPassword()
	appServ := app.NewAuth(authStorage, sessStorage)
	err := appServ.CreateNewCustomer(ctx, email, pass)
	is.NoErr(err)

	// when
	client, err := appServ.Login(ctx, email, pass)

	// then
	is.NoErr(err)
	is.True(client != nil)
}

func TestInvalidEmail(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	// given
	email := "invalid_email"
	pass := randomPassword()
	appServ := app.NewAuth(authStorage, sessStorage)

	// when
	err := appServ.CreateNewCustomer(ctx, email, pass)

	// then
	is.True(err != nil)
}

func TestPassToShort(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	// given
	email := randomEmail()
	pass := "123"
	appServ := app.NewAuth(authStorage, sessStorage)

	// when
	err := appServ.CreateNewCustomer(ctx, email, pass)

	// then
	var e domain.PasswordPolicyError

	is.True(errors.As(err, &e))
}

func randomEmail() (email string) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	email = fmt.Sprintf("user_%X%X%X%X%X@example.com", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])

	return
}

func randomPassword() (pass string) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	pass = fmt.Sprintf("%X%X%X%X%XaZ!1", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])

	return
}
