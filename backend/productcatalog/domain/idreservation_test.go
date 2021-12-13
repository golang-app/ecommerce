package domain

import (
	"context"
	"testing"

	"github.com/matryer/is"
)

func TestIDReservation(t *testing.T) {
	is := is.New(t)
	// given
	storage := reservationStorage{}
	reserv := ProductIdReservation{storage: storage}

	// when
	id, err := reserv.Reserve(context.Background(), "my name")

	// then
	is.NoErr(err)
	t.Log(id)
	is.True(id == productID("my-name"))
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
