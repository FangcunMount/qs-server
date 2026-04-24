package cache

import "fmt"

// CacheEntryCodec converts between a domain object and its Redis entry payload.
type CacheEntryCodec[T any] struct {
	EncodeFunc func(*T) ([]byte, error)
	DecodeFunc func([]byte) (*T, error)
}

func (c CacheEntryCodec[T]) Encode(value *T) ([]byte, error) {
	if c.EncodeFunc == nil {
		return nil, fmt.Errorf("object cache encode func is nil")
	}
	return c.EncodeFunc(value)
}

func (c CacheEntryCodec[T]) Decode(data []byte) (*T, error) {
	if c.DecodeFunc == nil {
		return nil, fmt.Errorf("object cache decode func is nil")
	}
	return c.DecodeFunc(data)
}
