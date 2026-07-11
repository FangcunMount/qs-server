package report

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// InterpretReport 解读报告聚合根。
// 与 Assessment 关系：1:1，ID 与 AssessmentID 一致。
type InterpretReport struct {
	id            ID
	outcomeID     meta.ID
	status        Status
	attempt       uint
	failureReason string
	generatingAt  *time.Time
	generatedAt   *time.Time
	failedAt      *time.Time
	model         ModelIdentity
	primaryScore  *ScoreValue
	level         *ResultLevel
	modelName     string
	modelCode     string
	totalScore    float64
	riskLevel     RiskLevel
	conclusion    string
	dimensions    []DimensionInterpret
	suggestions   []Suggestion
	modelExtra    *ModelExtra
	createdAt     time.Time
	updatedAt     *time.Time
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
		status:      StatusGenerated,
		attempt:     1,
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
	r.generatedAt = timePtr(r.createdAt)
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
		status:      StatusGenerated,
		attempt:     1,
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

// NewPendingInterpretReport starts an independent report lifecycle for one
// durable EvaluationOutcome.
func NewPendingInterpretReport(id ID, outcomeID meta.ID, at time.Time) (*InterpretReport, error) {
	if id.IsZero() || outcomeID.IsZero() {
		return nil, fmt.Errorf("report id and evaluation outcome id are required")
	}
	if at.IsZero() {
		at = time.Now()
	}
	return &InterpretReport{id: id, outcomeID: outcomeID, status: StatusPending, createdAt: at}, nil
}

// RestoreLifecycle applies persisted lifecycle metadata. Empty status denotes
// a legacy generated report.
func (r *InterpretReport) RestoreLifecycle(outcomeID meta.ID, status Status, attempt uint, failureReason string, generatingAt, generatedAt, failedAt *time.Time) {
	if status == "" {
		status = StatusGenerated
	}
	r.outcomeID = outcomeID
	r.status = status
	r.attempt = attempt
	if r.attempt == 0 && status == StatusGenerated {
		r.attempt = 1
	}
	r.failureReason = failureReason
	r.generatingAt = copyTimePtr(generatingAt)
	r.generatedAt = copyTimePtr(generatedAt)
	r.failedAt = copyTimePtr(failedAt)
}

// ResetForOutcome starts a new report lifecycle when an explicitly new
// EvaluationOutcome supersedes an older report projection.
func (r *InterpretReport) ResetForOutcome(outcomeID meta.ID, at time.Time) error {
	if outcomeID.IsZero() {
		return fmt.Errorf("evaluation outcome id is required")
	}
	r.outcomeID = outcomeID
	r.status = StatusPending
	r.attempt = 0
	r.failureReason = ""
	r.generatingAt = nil
	r.generatedAt = nil
	r.failedAt = nil
	if !at.IsZero() {
		r.updatedAt = timePtr(at)
	}
	return nil
}

func (r *InterpretReport) BeginGenerating(at time.Time) error {
	if r.status != StatusPending && r.status != StatusFailed && r.status != StatusGenerating {
		return fmt.Errorf("report cannot begin generating from status %s", r.status)
	}
	if at.IsZero() {
		at = time.Now()
	}
	r.status = StatusGenerating
	r.attempt++
	r.failureReason = ""
	r.generatingAt = timePtr(at)
	r.failedAt = nil
	r.updatedAt = timePtr(at)
	return nil
}

func (r *InterpretReport) CompleteFrom(generated *InterpretReport, at time.Time) error {
	if r.status != StatusGenerating {
		return fmt.Errorf("report cannot complete from status %s", r.status)
	}
	if generated == nil {
		return fmt.Errorf("generated report is required")
	}
	if at.IsZero() {
		at = time.Now()
	}
	r.model = generated.model
	r.primaryScore = generated.primaryScore
	r.level = generated.level
	r.modelName = generated.modelName
	r.modelCode = generated.modelCode
	r.totalScore = generated.totalScore
	r.riskLevel = generated.riskLevel
	r.conclusion = generated.conclusion
	r.dimensions = generated.dimensions
	r.suggestions = generated.suggestions
	r.modelExtra = generated.modelExtra
	r.status = StatusGenerated
	r.failureReason = ""
	r.generatedAt = timePtr(at)
	r.failedAt = nil
	r.updatedAt = timePtr(at)
	return nil
}

func (r *InterpretReport) Fail(reason string, at time.Time) error {
	if r.status != StatusGenerating && r.status != StatusGenerated {
		return fmt.Errorf("report cannot fail from status %s", r.status)
	}
	if reason == "" {
		return fmt.Errorf("report failure reason is required")
	}
	if at.IsZero() {
		at = time.Now()
	}
	r.status = StatusFailed
	r.failureReason = reason
	r.generatedAt = nil
	r.failedAt = timePtr(at)
	r.updatedAt = timePtr(at)
	return nil
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

func (r *InterpretReport) OutcomeID() meta.ID { return r.outcomeID }

func (r *InterpretReport) Status() Status { return r.status }

func (r *InterpretReport) Attempt() uint { return r.attempt }

func (r *InterpretReport) FailureReason() string { return r.failureReason }

func (r *InterpretReport) GeneratingAt() *time.Time { return copyTimePtr(r.generatingAt) }

func (r *InterpretReport) GeneratedAt() *time.Time { return copyTimePtr(r.generatedAt) }

func (r *InterpretReport) FailedAt() *time.Time { return copyTimePtr(r.failedAt) }

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

// FindDimension 是 deprecated; 使用 FindDimensionByCode。
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

// GetHighRiskDimensions 是 deprecated; 使用 GetHighSeverityDimensions。
func (r *InterpretReport) GetHighRiskDimensions() []DimensionInterpret {
	return r.GetHighSeverityDimensions()
}

func (r *InterpretReport) HasSuggestions() bool { return len(r.suggestions) > 0 }

func timePtr(value time.Time) *time.Time { return &value }

func copyTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
