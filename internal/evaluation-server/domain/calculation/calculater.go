package calculation

// Calculater 计算器接口
type Calculater interface {
	Calculate(operands []Operand) (CalcResult, error)
}

// CalculaterType 计算器类型
type CalculaterType string

const (
	CalculaterTheOption   CalculaterType = "the_option" // 选项
	CalculaterTypeSum     CalculaterType = "sum"        // 求和
	CalculaterTypeAverage CalculaterType = "average"    // 平均值
	CalculaterTypeMax     CalculaterType = "max"        // 最大值
	CalculaterTypeMin     CalculaterType = "min"        // 最小值
)

// Operand 操作数
type Operand float64

// Value 获取操作数值
func (o Operand) Value() float64 {
	return float64(o)
}

// 计算结果
type CalcResult float64

// Value 获取计算结果
func (r CalcResult) Value() float64 {
	return float64(r)
}
