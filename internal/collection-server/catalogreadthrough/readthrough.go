package catalogreadthrough

import (
	"reflect"

	"golang.org/x/sync/singleflight"
)

// ReadThrough 执行 get → optional singleflight → load → set → clone 读穿透。
func ReadThrough[T any](
	key string,
	get func() (T, bool),
	set func(T),
	load func() (T, error),
	clone func(T) T,
	sf *singleflight.Group,
	useSingleflight bool,
) (T, error) {
	var zero T
	if get != nil {
		if cached, ok := get(); ok {
			return cached, nil
		}
	}

	if useSingleflight && sf != nil && set != nil {
		value, err, _ := sf.Do(key, func() (interface{}, error) {
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
				sf.Forget(key)
				return resp, nil
			}
			return finalizeLoaded(resp, nil, set, clone)
		})
		if err != nil {
			return zero, err
		}
		if value == nil {
			sf.Forget(key)
			return zero, nil
		}
		typed, ok := value.(T)
		if !ok || isNilValue(typed) {
			sf.Forget(key)
			return zero, nil
		}
		return typed, nil
	}

	resp, err := load()
	if err != nil || set == nil {
		return resp, err
	}
	return finalizeLoaded(resp, nil, set, clone)
}

func finalizeLoaded[T any](resp T, err error, set func(T), clone func(T) T) (T, error) {
	var zero T
	if err != nil {
		return zero, err
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
}

func isNilValue[T any](v T) bool {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return true
	}
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Interface, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}
