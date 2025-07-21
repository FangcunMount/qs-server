package calculation

// AdapterType 适配器类型
type AdapterType string

const (
	// SerialAdapter 串行适配器
	SerialAdapter AdapterType = "serial"
	// ConcurrentAdapter 并发适配器
	ConcurrentAdapter AdapterType = "concurrent"
)

// AdapterFactory 适配器工厂
type AdapterFactory struct{}

// NewAdapterFactory 创建适配器工厂
func NewAdapterFactory() *AdapterFactory {
	return &AdapterFactory{}
}

// CreateCalculationPort 创建计算端口适配器
func (f *AdapterFactory) CreateCalculationPort(adapterType AdapterType, maxConcurrency ...int) CalculationPort {
	switch adapterType {
	case SerialAdapter:
		return NewSerialCalculationAdapter()
	case ConcurrentAdapter:
		concurrency := 10 // 默认并发数
		if len(maxConcurrency) > 0 && maxConcurrency[0] > 0 {
			concurrency = maxConcurrency[0]
		}
		return NewConcurrentCalculationAdapter(concurrency)
	default:
		// 默认返回串行适配器
		return NewSerialCalculationAdapter()
	}
}

// GetGlobalAdapterFactory 获取全局适配器工厂
func GetGlobalAdapterFactory() *AdapterFactory {
	return NewAdapterFactory()
}

// ============ 便捷创建函数 ============

// GetSerialCalculationPort 获取串行计算端口
func GetSerialCalculationPort() CalculationPort {
	return NewSerialCalculationAdapter()
}

// GetConcurrentCalculationPort 获取并发计算端口
func GetConcurrentCalculationPort(maxConcurrency int) CalculationPort {
	return NewConcurrentCalculationAdapter(maxConcurrency)
}
