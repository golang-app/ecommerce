package domain

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var ErrIDInUse = errors.New("the product id is already reserved or in use")
var regexpMultipleDashes = regexp.MustCompile("-+")
var productIDRegCleanUp = regexp.MustCompile(`[^\w\d\-]+`)

type ProductIdReservation struct {
	storage productIDReservationStorage
}

func NewProductIdReservation(storage productIDReservationStorage) ProductIdReservation {
	return ProductIdReservation{storage: storage}
}

type productIDReservationStorage interface {
	Reserve(ctx context.Context, name string) error
}

// for the given name of the product, it returns next reserved ID
func (r ProductIdReservation) Reserve(ctx context.Context, name string) (productID, error) {
	// remove all unnecessary characters
	id := strings.TrimSpace(name)
	id = productIDRegCleanUp.ReplaceAllString(name, "-")
	id = regexpMultipleDashes.ReplaceAllString(id, "-")
	err := r.storage.Reserve(ctx, id)

	if errors.Is(err, ErrIDInUse) {
		return r.reserveIterating(ctx, id)
	}

	if err != nil {
		return "", fmt.Errorf("cannot reserve the product ID (%s): %w", id, err)
	}

	return productID(id), nil
}

func (r ProductIdReservation) reserveIterating(ctx context.Context, id string) (productID, error) {
	for i := 1; ; i++ {
		nid := fmt.Sprintf("%s-%d", id, i)

		err := r.storage.Reserve(ctx, nid)
		if errors.Is(err, ErrIDInUse) {
			continue
		}

		if err != nil {
			return "", fmt.Errorf("cannot reserve the product ID (%s): %w", id, err)
		}

		return productID(nid), nil
	}
}
