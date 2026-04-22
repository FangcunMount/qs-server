package port

import (
	"context"
	"errors"
	"io"
)

var ErrObjectNotFound = errors.New("object not found")

// ObjectReader represents an opened object stream.
type ObjectReader struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
	CacheControl  string
}

// PublicObjectStore stores QR code objects and exposes upload/download operations.
type PublicObjectStore interface {
	Put(ctx context.Context, key string, contentType string, body []byte) error
	Get(ctx context.Context, key string) (*ObjectReader, error)
}
