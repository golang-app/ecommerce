package app

import (
	"context"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/matryer/is"
)

func TestIDReservation(t *testing.T) {
	is := is.New(t)
	// given
	storage := reservationStorage{}
	reserv := productIdReservation{storage: storage}

	// when
	id, err := reserv.Reserve(context.Background(), "my name")

	// then
	is.NoErr(err)
	t.Log(id)
	is.True(id == domain.ProductID("my-name"))
}

type reservationStorage struct {
	reserved []string
}

func (s reservationStorage) Reserve(ctx context.Context, name string) error {
	if stringInSlice(name, s.reserved) {
		return ErrIDInUse
	}

	return nil
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
