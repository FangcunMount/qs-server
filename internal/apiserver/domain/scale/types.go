package scale

// ===================== 量表状态 =================

// Status 量表状态
type Status uint8

const (
	StatusDraft     Status = 0 // 草稿
	StatusPublished Status = 1 // 已发布
	StatusArchived  Status = 2 // 已归档
)

// Value 获取状态值
func (s Status) Value() uint8 {
	return uint8(s)
}

// String 获取状态字符串
func (s Status) String() string {
	statusMap := map[uint8]string{
		0: "草稿",
		1: "已发布",
		2: "已归档",
	}
	return statusMap[s.Value()]
}

// IsDraft 是否草稿状态
func (s Status) IsDraft() bool {
	return s == StatusDraft
}

// IsPublished 是否已发布状态
func (s Status) IsPublished() bool {
	return s == StatusPublished
}

// IsArchived 是否已归档状态
func (s Status) IsArchived() bool {
	return s == StatusArchived
}

// ===================== 因子编码 =================

// FactorCode 因子编码
type FactorCode string

// NewFactorCode 创建因子编码
func NewFactorCode(value string) FactorCode {
	return FactorCode(value)
}

// Value 获取编码值
func (c FactorCode) Value() string {
	return string(c)
}

// String 获取编码字符串
func (c FactorCode) String() string {
	return string(c)
}

// IsEmpty 判断编码是否为空
func (c FactorCode) IsEmpty() bool {
	return c == ""
}

// Equals 判断编码是否相等
func (c FactorCode) Equals(other FactorCode) bool {
	return c == other
}

// ===================== 因子类型 =================

// FactorType 因子类型
type FactorType string

const (
	// FactorTypePrimary 一级因子
	FactorTypePrimary FactorType = "primary"
	// FactorTypeMultilevel 多级因子
	FactorTypeMultilevel FactorType = "multilevel"
)

// String 返回因子类型的字符串表示
func (ft FactorType) String() string {
	return string(ft)
}

// IsValid 检查因子类型是否有效
func (ft FactorType) IsValid() bool {
	return ft == FactorTypePrimary || ft == FactorTypeMultilevel
}

// ===================== 计分策略 =================

// ScoringStrategyCode 因子计分策略编码
type ScoringStrategyCode string

const (
	// ScoringStrategySum 求和策略
	ScoringStrategySum ScoringStrategyCode = "sum"
	// ScoringStrategyAvg 平均策略
	ScoringStrategyAvg ScoringStrategyCode = "avg"
	// ScoringStrategyCnt 计数策略（统计匹配特定选项的题目数量）
	ScoringStrategyCnt ScoringStrategyCode = "cnt"
)

// String 返回计分策略的字符串表示
func (s ScoringStrategyCode) String() string {
	return string(s)
}

// IsValid 检查计分策略是否有效
func (s ScoringStrategyCode) IsValid() bool {
	return s == ScoringStrategySum || s == ScoringStrategyAvg || s == ScoringStrategyCnt
}

// ===================== 风险等级 =================

// RiskLevel 风险等级
type RiskLevel string

const (
	// RiskLevelNone 无风险
	RiskLevelNone RiskLevel = "none"
	// RiskLevelLow 低风险
	RiskLevelLow RiskLevel = "low"
	// RiskLevelMedium 中风险
	RiskLevelMedium RiskLevel = "medium"
	// RiskLevelHigh 高风险
	RiskLevelHigh RiskLevel = "high"
	// RiskLevelSevere 严重风险
	RiskLevelSevere RiskLevel = "severe"
)

// String 返回风险等级的字符串表示
func (r RiskLevel) String() string {
	return string(r)
}

// IsValid 检查风险等级是否有效
func (r RiskLevel) IsValid() bool {
	switch r {
	case RiskLevelNone, RiskLevelLow, RiskLevelMedium, RiskLevelHigh, RiskLevelSevere:
		return true
	default:
		return false
	}
}

// ===================== 分数区间 =================

// ScoreRange 分数区间 [Min, Max]
// 注意：医学量表的解读规则通常使用闭区间，即包含边界值
// 例如：[0, 10] 表示 0 到 10 分（包括 10 分）都适用该规则
type ScoreRange struct {
	min float64
	max float64
}

// NewScoreRange 创建分数区间
func NewScoreRange(min, max float64) ScoreRange {
	return ScoreRange{min: min, max: max}
}

// Min 获取最小值
func (r ScoreRange) Min() float64 {
	return r.min
}

// Max 获取最大值
func (r ScoreRange) Max() float64 {
	return r.max
}

// Contains 判断分数是否在区间内 [min, max]
// 使用左闭右闭区间，符合医学量表的实际使用场景
// 例如：ScoreRange{0, 10}.Contains(10) 返回 true
func (r ScoreRange) Contains(score float64) bool {
	return score >= r.min && score <= r.max
}

// IsValid 检查区间是否有效
func (r ScoreRange) IsValid() bool {
	return r.min <= r.max
}

// ===================== 计分参数 =================

// ScoringParams 计分参数值对象
// 根据不同的计分策略，使用不同的字段
type ScoringParams struct {
	// 计数策略（cnt）专用参数
	CntOptionContents []string
}

// NewScoringParams 创建计分参数
func NewScoringParams() *ScoringParams {
	return &ScoringParams{
		CntOptionContents: make([]string, 0),
	}
}

// WithCntOptionContents 设置计数策略的选项内容列表
func (p *ScoringParams) WithCntOptionContents(contents []string) *ScoringParams {
	if contents == nil {
		p.CntOptionContents = make([]string, 0)
	} else {
		p.CntOptionContents = contents
	}
	return p
}

// GetCntOptionContents 获取计数策略的选项内容列表
func (p *ScoringParams) GetCntOptionContents() []string {
	if p == nil {
		return nil
	}
	return p.CntOptionContents
}

// ToMap 转换为 map[string]interface{}（用于持久化）
// 根据计分策略构建相应的参数结构
func (p *ScoringParams) ToMap(strategy ScoringStrategyCode) map[string]interface{} {
	result := make(map[string]interface{})

	if p == nil {
		return result
	}

	// 根据策略类型处理参数
	switch strategy {
	case ScoringStrategyCnt:
		// 计数策略：构建 raw_calc_rule 结构化对象
		if len(p.CntOptionContents) > 0 {
			rawCalcRule := map[string]interface{}{
				"formula": "cnt",
				"AppendParams": map[string]interface{}{
					"cnt_option_contents": p.CntOptionContents,
				},
			}
			result["raw_calc_rule"] = rawCalcRule
		}

	case ScoringStrategySum, ScoringStrategyAvg:
		// 求和和平均策略：当前不需要额外参数
		// 如果需要扩展，可以在这里添加

	default:
		// 其他策略：当前不需要额外参数
	}

	return result
}

// FromMap 从 map[string]interface{} 创建（用于从持久化层恢复）
func ScoringParamsFromMap(params map[string]interface{}, strategy ScoringStrategyCode) *ScoringParams {
	if params == nil {
		return NewScoringParams()
	}

	result := NewScoringParams()

	// 根据策略类型解析参数
	switch strategy {
	case ScoringStrategyCnt:
		// 从 raw_calc_rule 中提取 cnt_option_contents
		if rawRule, exists := params["raw_calc_rule"]; exists && rawRule != nil {
			if ruleMap, ok := rawRule.(map[string]interface{}); ok {
				if appendParams, ok := ruleMap["AppendParams"].(map[string]interface{}); ok {
					if contents, ok := appendParams["cnt_option_contents"]; ok {
						if contentsArray, ok := contents.([]interface{}); ok {
							result.CntOptionContents = make([]string, 0, len(contentsArray))
							for _, item := range contentsArray {
								if str, ok := item.(string); ok {
									result.CntOptionContents = append(result.CntOptionContents, str)
								}
							}
						} else if contentsArray, ok := contents.([]string); ok {
							result.CntOptionContents = contentsArray
						}
					}
				}
			}
		}

	case ScoringStrategySum, ScoringStrategyAvg:
		// 求和和平均策略：当前不需要额外参数

	default:
		// 其他策略：当前不需要额外参数
	}

	return result
}
