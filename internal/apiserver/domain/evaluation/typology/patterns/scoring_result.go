package patterns

import (
	calcclassification "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// ScoringResult 是领域-本地 output of 人格模型适配器。
type ScoringResult struct {
	Runtime         *modeltypology.RuntimeSpec
	Vector          calcclassification.ProfileVector
	Candidate       calcclassification.OutcomeCandidate
	SelectedOutcome SelectedOutcome
	SpecialMatch    *ScoringSpecialMatch
	Detail          any
}

// SelectedOutcome 记录选中 model 结果 在之前 明细组装。
type SelectedOutcome struct {
	Code       string
	Similarity float64
	Trigger    string
}

// ScoringSpecialMatch 记录special rule that altered 计分。
type ScoringSpecialMatch struct {
	OutcomeCode string
	Trigger     string
	SkipScoring bool
}

// LegacyDetail 返回typed 明细载荷 用于 backward-compatible callers。
func (r ScoringResult) LegacyDetail() any {
	return r.Detail
}
