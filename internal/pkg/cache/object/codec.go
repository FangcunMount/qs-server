package object

import "fmt"

// CacheEntryCodec converts between a domain object and its Redis entry payload.
type Codec[T any] struct {
	EncodeFunc func(*T) ([]byte, error)
	DecodeFunc func([]byte) (*T, error)
}

func (c Codec[T]) Encode(value *T) ([]byte, error) {
	if c.EncodeFunc == nil {
		return nil, fmt.Errorf("object cache encode func is nil")
	}
	return c.EncodeFunc(value)
}

func (c Codec[T]) Decode(data []byte) (*T, error) {
	if c.DecodeFunc == nil {
		return nil, fmt.Errorf("object cache decode func is nil")
	}
	return c.DecodeFunc(data)
}
