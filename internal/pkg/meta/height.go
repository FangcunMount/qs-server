package meta

// Height 表示以 0.1 单位存储的身高（单位：厘米，例如 170.5cm -> 1705）
type Height struct {
	tenthsMeasurement
}

// NewHeightFromFloat 创建一个新的 Height 实例，接受一个浮点数（单位：厘米）
func NewHeightFromFloat(f float64) (Height, error) {
	measurement, err := newTenthsMeasurement("height", f)
	if err != nil {
		return Height{}, err
	}
	return Height{tenthsMeasurement: measurement}, nil
}

// NewHeightFromTenths 从以 0.1 为单位的整数创建 Height 实例
func NewHeightFromTenths(t int64) Height {
	return Height{tenthsMeasurement: newTenthsMeasurementFromTenths(t)}
}

// JSON 反序列化，接受 number（例如 170.5）
func (h *Height) UnmarshalJSON(b []byte) error {
	return h.UnmarshalJSONWithKind("height", b)
}

// Scan 从数据库读取值
func (h *Height) Scan(src any) error {
	return h.ScanWithKind("height", src)
}
