package flag

// StringFlag 是一个字符串标志，兼容 flags 和 pflags，并跟踪是否提供了值
type StringFlag struct {
	// 如果 Set 被调用，则此值为 true
	provided bool
	// 标志的精确值
	value string
}

// NewStringFlag 创建一个字符串标志
func NewStringFlag(defaultVal string) StringFlag {
	return StringFlag{value: defaultVal}
}

// Default 设置默认值
func (f *StringFlag) Default(value string) {
	f.value = value
}

// String 返回标志的值
func (f StringFlag) String() string {
	return f.value
}

// Value 返回标志的值
func (f StringFlag) Value() string {
	return f.value
}

// Set 设置标志的值
func (f *StringFlag) Set(value string) error {
	f.value = value
	f.provided = true

	return nil
}

// Provided 返回标志是否被提供
func (f StringFlag) Provided() bool {
	return f.provided
}

// Type 返回标志的类型
func (f *StringFlag) Type() string {
	return "string"
}
