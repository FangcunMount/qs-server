package codeutil

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// Code 通用编码类型
// 用于表示各种实体的唯一编码标识
type Code struct {
	value string
}

// NewCode 创建新的编码
func NewCode(value string) Code {
	return Code{value: value}
}

// GenerateNewCode 生成新的唯一编码
func GenerateNewCode() (Code, error) {
	codeStr, err := GenerateCode()
	if err != nil {
		return Code{}, err
	}
	return Code{value: codeStr}, nil
}

// Value 获取编码值
func (c Code) Value() string {
	return c.value
}

// String 获取编码字符串表示
func (c Code) String() string {
	return c.value
}

// Equals 判断编码是否相等
func (c Code) Equals(other Code) bool {
	return c.value == other.value
}

// IsZero 判断是否为零值
func (c Code) IsZero() bool {
	return c.value == ""
}

// IsEmpty 判断编码是否为空
func (c Code) IsEmpty() bool {
	return c.value == ""
}

// Scan 实现 sql.Scanner 接口,用于从数据库读取
func (c *Code) Scan(value interface{}) error {
	if value == nil {
		c.value = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		c.value = v
		return nil
	case []byte:
		c.value = string(v)
		return nil
	default:
		return fmt.Errorf("cannot scan %T into Code", value)
	}
}

// DBValue 实现 driver.Valuer 接口,用于写入数据库
func (c Code) DBValue() (driver.Value, error) {
	return c.value, nil
}

// MarshalJSON 实现 JSON 序列化
func (c Code) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.value)
}

// UnmarshalJSON 实现 JSON 反序列化
func (c *Code) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	c.value = s
	return nil
}
