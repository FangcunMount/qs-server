package idutil

import (
	"database/sql/driver"
	"fmt"
	"strconv"
)

// ID 通用ID类型,使用泛型支持不同的底层类型
// T 可以是 uint64, int64, string 等类型
type ID[T comparable] struct {
	value T
}

// NewID 创建新的ID
func NewID[T comparable](value T) ID[T] {
	return ID[T]{value: value}
}

// Value 获取ID值
func (id ID[T]) Value() T {
	return id.value
}

// Equals 判断ID是否相等
func (id ID[T]) Equals(other ID[T]) bool {
	return id.value == other.value
}

// IsZero 判断是否为零值
func (id ID[T]) IsZero() bool {
	var zero T
	return id.value == zero
}

// String 获取ID字符串表示
func (id ID[T]) String() string {
	return fmt.Sprintf("%v", id.value)
}

// UInt64ID 是基于 uint64 的ID类型
type UInt64ID = ID[uint64]

// NewUInt64ID 创建 uint64 类型的ID
func NewUInt64ID(value uint64) UInt64ID {
	return NewID[uint64](value)
}

// StringID 是基于 string 的ID类型
type StringID = ID[string]

// NewStringID 创建 string 类型的ID
func NewStringID(value string) StringID {
	return NewID[string](value)
}

// Int64ID 是基于 int64 的ID类型
type Int64ID = ID[int64]

// NewInt64ID 创建 int64 类型的ID
func NewInt64ID(value int64) Int64ID {
	return NewID[int64](value)
}

// Scan 实现 sql.Scanner 接口,用于从数据库读取
func (id *ID[T]) Scan(value interface{}) error {
	if value == nil {
		var zero T
		id.value = zero
		return nil
	}

	switch v := value.(type) {
	case int64:
		// 尝试类型断言
		if val, ok := any(v).(T); ok {
			id.value = val
			return nil
		}
		// 如果 T 是 uint64,进行转换
		if val, ok := any(uint64(v)).(T); ok {
			id.value = val
			return nil
		}
	case uint64:
		if val, ok := any(v).(T); ok {
			id.value = val
			return nil
		}
	case string:
		if val, ok := any(v).(T); ok {
			id.value = val
			return nil
		}
		// 尝试将字符串转换为 uint64
		if intVal, err := strconv.ParseUint(v, 10, 64); err == nil {
			if val, ok := any(intVal).(T); ok {
				id.value = val
				return nil
			}
		}
	case []byte:
		str := string(v)
		if val, ok := any(str).(T); ok {
			id.value = val
			return nil
		}
		// 尝试将字符串转换为 uint64
		if intVal, err := strconv.ParseUint(str, 10, 64); err == nil {
			if val, ok := any(intVal).(T); ok {
				id.value = val
				return nil
			}
		}
	}

	return fmt.Errorf("cannot scan %T into ID[%T]", value, id.value)
}

// DBValue 实现 driver.Valuer 接口,用于写入数据库
func (id ID[T]) DBValue() (driver.Value, error) {
	return id.value, nil
}

// MarshalJSON 实现 JSON 序列化
func (id ID[T]) MarshalJSON() ([]byte, error) {
	switch v := any(id.value).(type) {
	case uint64, int64, int, uint:
		return []byte(fmt.Sprintf("%d", v)), nil
	case string:
		return []byte(fmt.Sprintf(`"%s"`, v)), nil
	default:
		return []byte(fmt.Sprintf(`"%v"`, v)), nil
	}
}

// UnmarshalJSON 实现 JSON 反序列化
func (id *ID[T]) UnmarshalJSON(data []byte) error {
	str := string(data)

	// 移除引号(如果有)
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}

	// 尝试根据 T 的类型解析
	var zero T
	switch any(zero).(type) {
	case uint64:
		val, err := strconv.ParseUint(str, 10, 64)
		if err != nil {
			return err
		}
		if v, ok := any(val).(T); ok {
			id.value = v
			return nil
		}
	case int64:
		val, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return err
		}
		if v, ok := any(val).(T); ok {
			id.value = v
			return nil
		}
	case string:
		if v, ok := any(str).(T); ok {
			id.value = v
			return nil
		}
	}

	return fmt.Errorf("cannot unmarshal %s into ID[%T]", str, zero)
}
