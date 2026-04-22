package assembler

func applyOptionalParam[T any](params []interface{}, idx int, setter func(T)) {
	if len(params) <= idx {
		return
	}
	value, ok := params[idx].(T)
	if !ok {
		return
	}
	setter(value)
}
