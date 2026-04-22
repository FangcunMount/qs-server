package meta

// Weight 表示以 0.1 单位存储的体重（单位：千克，例如 70.5kg -> 705）
type Weight struct {
	tenthsMeasurement
}

// NewWeightFromFloat 创建一个新的 Weight 实例，接受一个浮点数（单位：千克）
func NewWeightFromFloat(f float64) (Weight, error) {
	measurement, err := newTenthsMeasurement("weight", f)
	if err != nil {
		return Weight{}, err
	}
	return Weight{tenthsMeasurement: measurement}, nil
}

// NewWeightFromTenths 从以 0.1 为单位的整数创建 Weight 实例
func NewWeightFromTenths(t int64) Weight {
	return Weight{tenthsMeasurement: newTenthsMeasurementFromTenths(t)}
}

// JSON 反序列化，接受 number（例如 170.5）
func (w *Weight) UnmarshalJSON(b []byte) error {
	return w.UnmarshalJSONWithKind("weight", b)
}

// Scan 从数据库读取值
func (w *Weight) Scan(src any) error {
	return w.ScanWithKind("weight", src)
}
