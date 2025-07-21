package rules

// CalculationRule 计算规则
type CalculationRule struct {
	Name    string                 `json:"name"`    // 规则名称（如：sum, average, max等）
	Params  map[string]interface{} `json:"params"`  // 规则参数
	Config  CalculationConfig      `json:"config"`  // 计算配置
	Message string                 `json:"message"` // 自定义消息
}

// CalculationConfig 计算配置
type CalculationConfig struct {
	Precision       int                    `json:"precision"`        // 精度
	RoundingMode    string                 `json:"rounding_mode"`    // 舍入模式：round, ceil, floor
	MinOperands     int                    `json:"min_operands"`     // 最小操作数
	MaxOperands     int                    `json:"max_operands"`     // 最大操作数
	Weights         []float64              `json:"weights"`          // 权重配置
	ValidationRules []ValidationRule       `json:"validation_rules"` // 验证规则
	CustomParams    map[string]interface{} `json:"custom_params"`    // 自定义参数
}

// ValidationRule 验证规则
type ValidationRule struct {
	Type    string      `json:"type"`    // 规则类型
	Value   interface{} `json:"value"`   // 规则值
	Message string      `json:"message"` // 错误消息
}

// NewCalculationRule 创建计算规则
func NewCalculationRule(name string) *CalculationRule {
	return &CalculationRule{
		Name:   name,
		Params: make(map[string]interface{}),
		Config: DefaultCalculationConfig(),
	}
}

// DefaultCalculationConfig 默认配置
func DefaultCalculationConfig() CalculationConfig {
	return CalculationConfig{
		Precision:       2,
		RoundingMode:    "round",
		MinOperands:     0,
		MaxOperands:     0, // 0表示无限制
		Weights:         []float64{},
		ValidationRules: []ValidationRule{},
		CustomParams:    make(map[string]interface{}),
	}
}

// AddParam 添加参数
func (r *CalculationRule) AddParam(key string, value interface{}) *CalculationRule {
	if r.Params == nil {
		r.Params = make(map[string]interface{})
	}
	r.Params[key] = value
	return r
}

// SetPrecision 设置精度
func (r *CalculationRule) SetPrecision(precision int) *CalculationRule {
	r.Config.Precision = precision
	return r
}

// SetRoundingMode 设置舍入模式
func (r *CalculationRule) SetRoundingMode(mode string) *CalculationRule {
	r.Config.RoundingMode = mode
	return r
}

// SetOperandLimits 设置操作数限制
func (r *CalculationRule) SetOperandLimits(min, max int) *CalculationRule {
	r.Config.MinOperands = min
	r.Config.MaxOperands = max
	return r
}

// SetWeights 设置权重
func (r *CalculationRule) SetWeights(weights []float64) *CalculationRule {
	r.Config.Weights = weights
	return r
}

// AddValidationRule 添加验证规则
func (r *CalculationRule) AddValidationRule(ruleType string, value interface{}, message string) *CalculationRule {
	r.Config.ValidationRules = append(r.Config.ValidationRules, ValidationRule{
		Type:    ruleType,
		Value:   value,
		Message: message,
	})
	return r
}

// SetMessage 设置消息
func (r *CalculationRule) SetMessage(message string) *CalculationRule {
	r.Message = message
	return r
}
