package qrcodeasset

import "context"

// ImageStore persists generated QR code images and returns a public URL.
type ImageStore interface {
	StorePNG(ctx context.Context, fileName string, data []byte) (string, error)
}
