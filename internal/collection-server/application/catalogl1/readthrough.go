package catalogl1

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/catalogreadthrough"
	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
	"golang.org/x/sync/singleflight"
)

// ReadThrough 执行 L1 get → optional singleflight → load → set → clone。
func ReadThrough[T any](
	key string,
	get func() (T, bool),
	set func(T),
	load func() (T, error),
	clone func(T) T,
	coalescer loadguard.Coalescer,
	useSingleflight bool,
) (T, error) {
	var setFn func(T)
	if set != nil {
		setFn = set
	}
	var sf *singleflight.Group
	if useSingleflight && coalescer == nil {
		sf = &singleflight.Group{}
	}
	if coalescer != nil && useSingleflight {
		return readThroughCoalescer(key, get, setFn, load, clone, coalescer)
	}
	return catalogreadthrough.ReadThrough(key, get, setFn, load, clone, sf, useSingleflight)
}

func readThroughCoalescer[T any](
	key string,
	get func() (T, bool),
	set func(T),
	load func() (T, error),
	clone func(T) T,
	coalescer loadguard.Coalescer,
) (T, error) {
	var zero T
	if get != nil {
		if cached, ok := get(); ok {
			return cached, nil
		}
	}
	value, err := coalescer.Do(context.TODO(), key, func() (any, error) {
		if get != nil {
			if cached, ok := get(); ok {
				return cached, nil
			}
		}
		resp, loadErr := load()
		if loadErr != nil {
			return nil, loadErr
		}
		if isNilValue(resp) {
			return resp, nil
		}
		if set != nil {
			set(resp)
		}
		if clone != nil {
			return clone(resp), nil
		}
		return resp, nil
	})
	if err != nil {
		return zero, err
	}
	if value == nil {
		coalescer.Forget(key)
		return zero, nil
	}
	typed, ok := value.(T)
	if !ok || isNilValue(typed) {
		coalescer.Forget(key)
		return zero, nil
	}
	return typed, nil
}
