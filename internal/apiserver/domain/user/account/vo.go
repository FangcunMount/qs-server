package account

// AccountID 账户ID值对象
type AccountID struct {
	value uint64
}

// NewAccountID 创建账户ID
func NewAccountID(value uint64) AccountID {
	return AccountID{value: value}
}

// Value 获取ID值
func (id AccountID) Value() uint64 {
	return id.value
}

// Equals 判断ID是否相等
func (id AccountID) Equals(other AccountID) bool {
	return id.value == other.value
}

// IsZero 判断是否为零值
func (id AccountID) IsZero() bool {
	return id.value == 0
}
