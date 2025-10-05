package wechat

// AppID 微信应用ID值对象
type AppID struct {
	value uint64
}

// NewAppID 创建应用ID
func NewAppID(value uint64) AppID {
	return AppID{value: value}
}

// Value 获取ID值
func (id AppID) Value() uint64 {
	return id.value
}

// Equals 判断ID是否相等
func (id AppID) Equals(other AppID) bool {
	return id.value == other.value
}

// IsZero 判断是否为零值
func (id AppID) IsZero() bool {
	return id.value == 0
}
