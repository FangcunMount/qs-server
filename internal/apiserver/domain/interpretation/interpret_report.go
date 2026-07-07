package interpretation

import "time"

// InterpretReport 解读报告聚合根。
// 与 Assessment 关系：1:1，ID 与 AssessmentID 一致。
type InterpretReport struct {
	id           ID
	model        ModelIdentity
	primaryScore *ScoreValue
	level        *ResultLevel
	modelName    string
	modelCode    string
	totalScore   float64
	riskLevel    RiskLevel
	conclusion   string
	dimensions   []DimensionInterpret
	suggestions  []Suggestion
	modelExtra   *ModelExtra
	createdAt    time.Time
	updatedAt    *time.Time
}

// NewInterpretReport 创建解读报告。
func NewInterpretReport(
	id ID,
	modelName string,
	modelCode string,
	totalScore float64,
	riskLevel RiskLevel,
	conclusion string,
	dimensions []DimensionInterpret,
	suggestions []Suggestion,
	modelExtra *ModelExtra,
) *InterpretReport {
	r := &InterpretReport{
		id:          id,
		modelName:   modelName,
		modelCode:   modelCode,
		totalScore:  totalScore,
		riskLevel:   riskLevel,
		conclusion:  conclusion,
		dimensions:  dimensions,
		suggestions: suggestions,
		modelExtra:  modelExtra,
		createdAt:   time.Now(),
	}
	FinalizeInterpretReport(r)
	return r
}

// ReconstructInterpretReport 重建解读报告（仅供仓储层使用）。
func ReconstructInterpretReport(
	id ID,
	modelName string,
	modelCode string,
	totalScore float64,
	riskLevel RiskLevel,
	conclusion string,
	dimensions []DimensionInterpret,
	suggestions []Suggestion,
	modelExtra *ModelExtra,
	createdAt time.Time,
	updatedAt *time.Time,
) *InterpretReport {
	r := &InterpretReport{
		id:          id,
		modelName:   modelName,
		modelCode:   modelCode,
		totalScore:  totalScore,
		riskLevel:   riskLevel,
		conclusion:  conclusion,
		dimensions:  dimensions,
		suggestions: suggestions,
		modelExtra:  modelExtra,
		createdAt:   createdAt,
		updatedAt:   updatedAt,
	}
	FinalizeInterpretReport(r)
	return r
}

// UpdateSuggestions 更新建议列表。
func (r *InterpretReport) UpdateSuggestions(suggestions []Suggestion) {
	r.suggestions = suggestions
	now := time.Now()
	r.updatedAt = &now
}

// AppendSuggestion 追加建议。
func (r *InterpretReport) AppendSuggestion(suggestion Suggestion) {
	if suggestion.Content != "" {
		r.suggestions = append(r.suggestions, suggestion)
		now := time.Now()
		r.updatedAt = &now
	}
}

func (r *InterpretReport) ID() ID { return r.id }

func (r *InterpretReport) Model() ModelIdentity { return r.model }

func (r *InterpretReport) PrimaryScore() *ScoreValue { return r.primaryScore }

func (r *InterpretReport) Level() *ResultLevel { return r.level }

func (r *InterpretReport) ModelName() string { return r.modelName }

func (r *InterpretReport) ModelCode() string { return r.modelCode }

func (r *InterpretReport) TotalScore() float64 { return r.totalScore }

func (r *InterpretReport) RiskLevel() RiskLevel { return r.riskLevel }

func (r *InterpretReport) Conclusion() string { return r.conclusion }

func (r *InterpretReport) Dimensions() []DimensionInterpret { return r.dimensions }

func (r *InterpretReport) Suggestions() []Suggestion { return r.suggestions }

func (r *InterpretReport) ModelExtra() *ModelExtra { return r.modelExtra }

func (r *InterpretReport) CreatedAt() time.Time { return r.createdAt }

func (r *InterpretReport) UpdatedAt() *time.Time { return r.updatedAt }

func (r *InterpretReport) IsHighRisk() bool {
	if r.level != nil && r.level.Severity == "high" {
		return true
	}
	return IsHighRisk(r.riskLevel)
}

func (r *InterpretReport) HasDimensions() bool { return len(r.dimensions) > 0 }

func (r *InterpretReport) FindDimensionByCode(code DimensionCode) (*DimensionInterpret, bool) {
	for i := range r.dimensions {
		if r.dimensions[i].Code() == code {
			return &r.dimensions[i], true
		}
	}
	return nil, false
}

// FindDimension 是deprecated; 使用 Find维度By编码。
func (r *InterpretReport) FindDimension(factorCode FactorCode) (*DimensionInterpret, bool) {
	return r.FindDimensionByCode(NewDimensionCode(factorCode.String()))
}

func (r *InterpretReport) GetHighSeverityDimensions() []DimensionInterpret {
	var result []DimensionInterpret
	for _, d := range r.dimensions {
		if d.IsHighSeverity() {
			result = append(result, d)
		}
	}
	return result
}

// GetHighRiskDimensions 是deprecated; 使用 GetHighSeverity维度。
func (r *InterpretReport) GetHighRiskDimensions() []DimensionInterpret {
	return r.GetHighSeverityDimensions()
}

func (r *InterpretReport) HasSuggestions() bool { return len(r.suggestions) > 0 }
