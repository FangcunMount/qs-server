package factor

// FactorType 因子类型
type FactorType string

const (
	// PrimaryFactor 一级因子
	PrimaryFactor FactorType = "primary"
	// MultilevelFactor 多级因子
	MultilevelFactor FactorType = "multilevel"
)

// String 返回因子类型的字符串表示
func (ft FactorType) String() string {
	return string(ft)
}

// IsValid 检查因子类型是否有效
func (ft FactorType) IsValid() bool {
	return ft == PrimaryFactor || ft == MultilevelFactor
}
