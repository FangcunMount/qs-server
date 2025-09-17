package answer_values

// StringValue 字符串值
type StringValue struct {
	V string
}

// Raw 原始值
func (v StringValue) Raw() any { return v.V }
