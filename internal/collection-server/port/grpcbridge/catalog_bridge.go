package grpcbridge

import "reflect"

// callCatalog 在 inner 就绪时执行 fetch，成功后将 raw 转为 application DTO。
func callCatalog[I any, Raw any, Out any](inner I, fetch func() (Raw, error), convert func(Raw) Out) (Out, error) {
	var zero Out
	if any(inner) == nil {
		return zero, nil
	}
	raw, err := fetch()
	if err != nil {
		return zero, err
	}
	if isNilValue(raw) {
		return zero, nil
	}
	return convert(raw), nil
}

func isNilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}
