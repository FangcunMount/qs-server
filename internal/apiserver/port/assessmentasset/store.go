package assessmentasset

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

// ObjectStore stores opaque binary objects and exposes upload/download operations.
// Public access is deliberately owned by the REST proxy, not by the storage port.
type ObjectStore interface {
	Put(ctx context.Context, key string, contentType string, body []byte) error
	Get(ctx context.Context, key string) (*ObjectReader, error)
}
