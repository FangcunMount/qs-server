package answer_values

// OptionValue 选项值
type OptionValue struct {
	Code string
}

// Raw 原始值
func (v OptionValue) Raw() any { return v.Code }
