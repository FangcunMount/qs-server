package meta

import "database/sql/driver"

type Birthday struct {
	day string // YYYY-MM-DD
}

// NewBirthday 创建一个新的 Birthday 实例
func NewBirthday(day string) Birthday {
	return Birthday{day: day}
}

// Day 返回生日字符串，格式为 YYYY-MM-DD
func (b Birthday) Day() string {
	return b.day
}

// String 返回生日字符串
func (b Birthday) String() string {
	return b.day
}

// Equal 比较两个 Birthday 是否相等
func (b Birthday) Equal(other Birthday) bool {
	return b.day == other.day
}

// IsEmpty 判断生日是否为空
func (b Birthday) IsEmpty() bool {
	return b.day == ""
}

// Value 实现 driver.Valuer 接口，返回数据库存储值
func (b Birthday) Value() (driver.Value, error) {
	if b.IsEmpty() {
		return nil, nil
	}
	return b.day, nil
}

// Scan 实现 sql.Scanner 接口，从数据库读取值
func (b *Birthday) Scan(src interface{}) error {
	if src == nil {
		*b = Birthday{}
		return nil
	}
	switch v := src.(type) {
	case string:
		*b = Birthday{day: v}
		return nil
	case []byte:
		*b = Birthday{day: string(v)}
		return nil
	default:
		return nil
	}
}
