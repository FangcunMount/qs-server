package meta

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
)

// Height 表示以 0.1 单位存储的身高（单位：厘米，例如 170.5cm -> 1705）
type Height struct {
	tenths int64 // internal: value * 10
}

// NewHeightFromFloat 创建一个新的 Height 实例，接受一个浮点数（单位：厘米）
func NewHeightFromFloat(f float64) (Height, error) {
	if f < 0 {
		return Height{}, fmt.Errorf("height must be >= 0")
	}
	// 四舍五入到 0.1
	t := int64(math.Round(f * 10.0))
	return Height{tenths: t}, nil
}

// NewHeightFromTenths 从以 0.1 为单位的整数创建 Height 实例
func NewHeightFromTenths(t int64) Height {
	return Height{tenths: t}
}

// Float 返回以厘米为单位的浮点数表示
func (h Height) Float() float64 {
	return float64(h.tenths) / 10.0
}

// Centimeters 返回以厘米为单位的整数表示
func (h Height) Tenths() int64 {
	return h.tenths
}

// Centimeters 返回以厘米为单位的整数表示
func (h Height) String() string {
	return fmt.Sprintf("%.1f", h.Float())
}

// JSON 序列化，输出 number（例如 170.5）
func (h Height) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.Float())
}

// JSON 反序列化，接受 number（例如 170.5）
func (h *Height) UnmarshalJSON(b []byte) error {
	var v float64
	if err := json.Unmarshal(b, &v); err == nil {
		hh, err := NewHeightFromFloat(v)
		if err != nil {
			return err
		}
		*h = hh
		return nil
	}
	// 也可接受整数写入（单位为 tenths）或字符串，视需要扩展
	return fmt.Errorf("invalid height json")
}

// Value 转换为数据库值
func (h Height) Value() (driver.Value, error) {
	return h.tenths, nil
}

// Scan 从数据库读取值
func (h *Height) Scan(src any) error {
	switch v := src.(type) {
	case int64:
		*h = NewHeightFromTenths(v)
		return nil
	case float64:
		hh, err := NewHeightFromFloat(v)
		if err != nil {
			return err
		}
		*h = hh
		return nil
	default:
		return fmt.Errorf("unsupported scan type %T", src)
	}
}
