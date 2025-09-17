package answer_values

// NumberValue 数值
type NumberValue struct {
	V int
}

// Raw 原始值
func (v NumberValue) Raw() any { return v.V }
