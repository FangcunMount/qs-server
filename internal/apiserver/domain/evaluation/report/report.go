package report

import "time"

// ==================== InterpretReport 聚合根 ====================

// InterpretReport 解读报告聚合根
// 职责：
// - 记录测评的解读报告
// - 包含维度解读和建议列表
// - 支持多种导出格式
//
// 存储：MongoDB（灵活的文档结构）
// 与 Assessment 关系：1:1，ID 与 AssessmentID 一致
type InterpretReport struct {
	// 身份标识（与 AssessmentID 一致）
	id ID

	// 量表信息
	scaleName string
	scaleCode string

	// 评估结果汇总
	totalScore float64
	riskLevel  RiskLevel
	conclusion string

	// 维度解读列表
	dimensions []DimensionInterpret

	// 建议列表
	suggestions []Suggestion

	// 时间戳
	createdAt time.Time
	updatedAt *time.Time
}

// NewInterpretReport 创建解读报告
func NewInterpretReport(
	id ID,
	scaleName string,
	scaleCode string,
	totalScore float64,
	riskLevel RiskLevel,
	conclusion string,
	dimensions []DimensionInterpret,
	suggestions []Suggestion,
) *InterpretReport {
	return &InterpretReport{
		id:          id,
		scaleName:   scaleName,
		scaleCode:   scaleCode,
		totalScore:  totalScore,
		riskLevel:   riskLevel,
		conclusion:  conclusion,
		dimensions:  dimensions,
		suggestions: suggestions,
		createdAt:   time.Now(),
	}
}

// ReconstructInterpretReport 重建解读报告（仅供仓储层使用）
func ReconstructInterpretReport(
	id ID,
	scaleName string,
	scaleCode string,
	totalScore float64,
	riskLevel RiskLevel,
	conclusion string,
	dimensions []DimensionInterpret,
	suggestions []Suggestion,
	createdAt time.Time,
	updatedAt *time.Time,
) *InterpretReport {
	return &InterpretReport{
		id:          id,
		scaleName:   scaleName,
		scaleCode:   scaleCode,
		totalScore:  totalScore,
		riskLevel:   riskLevel,
		conclusion:  conclusion,
		dimensions:  dimensions,
		suggestions: suggestions,
		createdAt:   createdAt,
		updatedAt:   updatedAt,
	}
}

// ==================== 报告更新方法 ====================

// UpdateSuggestions 更新建议列表
func (r *InterpretReport) UpdateSuggestions(suggestions []Suggestion) {
	r.suggestions = suggestions
	now := time.Now()
	r.updatedAt = &now
}

// AppendSuggestion 追加建议
func (r *InterpretReport) AppendSuggestion(suggestion Suggestion) {
	if suggestion.Content != "" {
		r.suggestions = append(r.suggestions, suggestion)
		now := time.Now()
		r.updatedAt = &now
	}
}

// ==================== InterpretReport 查询方法 ====================

// ID 获取报告ID（与 AssessmentID 一致）
func (r *InterpretReport) ID() ID {
	return r.id
}

// ScaleName 获取量表名称
func (r *InterpretReport) ScaleName() string {
	return r.scaleName
}

// ScaleCode 获取量表编码
func (r *InterpretReport) ScaleCode() string {
	return r.scaleCode
}

// TotalScore 获取总分
func (r *InterpretReport) TotalScore() float64 {
	return r.totalScore
}

// RiskLevel 获取风险等级
func (r *InterpretReport) RiskLevel() RiskLevel {
	return r.riskLevel
}

// Conclusion 获取结论
func (r *InterpretReport) Conclusion() string {
	return r.conclusion
}

// Dimensions 获取维度解读列表
func (r *InterpretReport) Dimensions() []DimensionInterpret {
	return r.dimensions
}

// Suggestions 获取建议列表
func (r *InterpretReport) Suggestions() []Suggestion {
	return r.suggestions
}

// CreatedAt 获取创建时间
func (r *InterpretReport) CreatedAt() time.Time {
	return r.createdAt
}

// UpdatedAt 获取更新时间
func (r *InterpretReport) UpdatedAt() *time.Time {
	return r.updatedAt
}

// ==================== 业务查询方法 ====================

// IsHighRisk 是否高风险
func (r *InterpretReport) IsHighRisk() bool {
	return IsHighRisk(r.riskLevel)
}

// HasDimensions 是否有维度解读
func (r *InterpretReport) HasDimensions() bool {
	return len(r.dimensions) > 0
}

// FindDimension 查找指定因子的维度解读
func (r *InterpretReport) FindDimension(factorCode FactorCode) (*DimensionInterpret, bool) {
	for i := range r.dimensions {
		if r.dimensions[i].FactorCode() == factorCode {
			return &r.dimensions[i], true
		}
	}
	return nil, false
}

// GetHighRiskDimensions 获取高风险维度
func (r *InterpretReport) GetHighRiskDimensions() []DimensionInterpret {
	var result []DimensionInterpret
	for _, d := range r.dimensions {
		if d.IsHighRisk() {
			result = append(result, d)
		}
	}
	return result
}

// HasSuggestions 是否有建议
func (r *InterpretReport) HasSuggestions() bool {
	return len(r.suggestions) > 0
}
