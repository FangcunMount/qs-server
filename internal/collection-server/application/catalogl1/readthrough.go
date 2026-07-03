package catalogl1

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/loadguard"
)

// ReadThrough 执行 L1 get → optional coalescer → load → set → clone。
// useSingleflight 为 true 时 coalescer 必须非 nil（通常 loadguard.NewCoalescer(true)）。
func ReadThrough[T any](
	key string,
	get func() (T, bool),
	set func(T),
	load func() (T, error),
	clone func(T) T,
	coalescer loadguard.Coalescer,
	useSingleflight bool,
) (T, error) {
	if useSingleflight {
		return readThroughCoalescer(key, get, set, load, clone, coalescer)
	}
	return readThroughDirect(key, get, set, load, clone)
}

func readThroughDirect[T any](
	_ string,
	get func() (T, bool),
	set func(T),
	load func() (T, error),
	clone func(T) T,
) (T, error) {
	if get != nil {
		if cached, ok := get(); ok {
			return cached, nil
		}
	}
	resp, err := load()
	if err != nil || set == nil {
		return resp, err
	}
	return finalizeLoaded(resp, set, clone), nil
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
	// Coalescer 合同：ctx 取消不传播；singleflight 合并以 key 为粒度。
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
		return finalizeLoaded(resp, set, clone), nil
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

func finalizeLoaded[T any](resp T, set func(T), clone func(T) T) T {
	if isNilValue(resp) {
		return resp
	}
	if set != nil {
		set(resp)
	}
	if clone != nil {
		return clone(resp)
	}
	return resp
}
