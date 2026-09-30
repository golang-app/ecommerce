package domain

import (
	"errors"
	"strings"
)

type ImagePosition string

const (
	ImagePositionPrimary ImagePosition = "PRIMARY"
	ImagePositionGallery ImagePosition = "GALLERY"
)

func (p ImagePosition) IsValid() bool {
	return p == ImagePositionPrimary || p == ImagePositionGallery
}

type ProductImage struct {
	id        string
	productID string
	url       string
	position  ImagePosition
}

func NewProductImage(id, productID, url string, pos ImagePosition) (ProductImage, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ProductImage{}, errors.New("image id is required")
	}
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return ProductImage{}, errors.New("product id is required")
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return ProductImage{}, errors.New("image url is required")
	}
	if !pos.IsValid() {
		return ProductImage{}, errors.New("invalid image position")
	}
	return ProductImage{
		id:        id,
		productID: productID,
		url:       url,
		position:  pos,
	}, nil
}

func (img ProductImage) ID() string             { return img.id }
func (img ProductImage) ProductID() string      { return img.productID }
func (img ProductImage) URL() string            { return img.url }
func (img ProductImage) Position() ImagePosition { return img.position }
func (img ProductImage) IsPrimary() bool        { return img.position == ImagePositionPrimary }
