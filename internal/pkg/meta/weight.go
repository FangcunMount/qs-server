package meta

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
)

// Weight 表示以 0.1 单位存储的体重（单位：千克，例如 70.5kg -> 705）
type Weight struct {
	tenths int64 // internal: value * 10
}

// NewWeightFromFloat 创建一个新的 Weight 实例，接受一个浮点数（单位：千克）
func NewWeightFromFloat(f float64) (Weight, error) {
	if f < 0 {
		return Weight{}, fmt.Errorf("weight must be >= 0")
	}
	// 四舍五入到 0.1
	t := int64(math.Round(f * 10.0))
	return Weight{tenths: t}, nil
}

// NewWeightFromTenths 从以 0.1 为单位的整数创建 Weight 实例
func NewWeightFromTenths(t int64) Weight {
	return Weight{tenths: t}
}

// Float 返回以千克为单位的浮点数表示
func (w Weight) Float() float64 {
	return float64(w.tenths) / 10.0
}

// Centimeters 返回以千克为单位的整数表示
func (w Weight) Tenths() int64 {
	return w.tenths
}

// Centimeters 返回以千克为单位的整数表示
func (w Weight) String() string {
	return fmt.Sprintf("%.1f", w.Float())
}

// JSON 序列化，输出 number（例如 170.5）
func (w Weight) MarshalJSON() ([]byte, error) {
	return json.Marshal(w.Float())
}

// JSON 反序列化，接受 number（例如 170.5）
func (w *Weight) UnmarshalJSON(b []byte) error {
	var v float64
	if err := json.Unmarshal(b, &v); err == nil {
		hh, err := NewWeightFromFloat(v)
		if err != nil {
			return err
		}
		*w = hh
		return nil
	}
	// 也可接受整数写入（单位为 tenths）或字符串，视需要扩展
	return fmt.Errorf("invalid weight json")
}

// Value 转换为数据库值
func (w Weight) Value() (driver.Value, error) {
	return w.tenths, nil
}

// Scan 从数据库读取值
func (w *Weight) Scan(src any) error {
	switch v := src.(type) {
	case int64:
		*w = NewWeightFromTenths(v)
		return nil
	case float64:
		hh, err := NewWeightFromFloat(v)
		if err != nil {
			return err
		}
		*w = hh
		return nil
	default:
		return fmt.Errorf("unsupported scan type %T", src)
	}
}
