package answer_values

// OptionsValue 选项值
type OptionsValue struct {
	V []OptionValue
}

// Raw 原始值
func (v OptionsValue) Raw() any { return v.V }
