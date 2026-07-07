package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/patterns"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

type outcomeAdapterFunc func(assessment.EvaluationModelRef, evaluationtypology.ScoringResult) (*assessment.AssessmentOutcome, error)

// OutcomeAdapterRegistry 解析测评结果 assemblers 按 detail adapter 键。
type OutcomeAdapterRegistry struct {
	adapters map[modeltypology.DetailAdapterKey]outcomeAdapterFunc
}

// 默认OutcomeAdapterRegistry 返回内置 通用 和 旧版 结果 adapters。
func DefaultOutcomeAdapterRegistry() OutcomeAdapterRegistry {
	return NewOutcomeAdapterRegistry()
}

// NewOutcomeAdapterRegistry 返回内置 通用 和 旧版 结果 adapters。
func NewOutcomeAdapterRegistry() OutcomeAdapterRegistry {
	return OutcomeAdapterRegistry{
		adapters: map[modeltypology.DetailAdapterKey]outcomeAdapterFunc{
			modeltypology.DetailAdapterPersonalityType: assembleGenericPersonalityTypeOutcome,
			modeltypology.DetailAdapterTraitProfile:    assembleGenericTraitProfileOutcome,
		},
	}
}

// Register 返回注册表副本 使用 额外 或 覆盖 结果 adapter。
func (r OutcomeAdapterRegistry) Register(key modeltypology.DetailAdapterKey, adapter outcomeAdapterFunc) OutcomeAdapterRegistry {
	next := OutcomeAdapterRegistry{adapters: make(map[modeltypology.DetailAdapterKey]outcomeAdapterFunc, len(r.adapters)+1)}
	for k, v := range r.adapters {
		next.adapters[k] = v
	}
	next.adapters[key] = adapter
	return next
}

func (r OutcomeAdapterRegistry) Assemble(
	key modeltypology.DetailAdapterKey,
	modelRef assessment.EvaluationModelRef,
	result evaluationtypology.ScoringResult,
) (*assessment.AssessmentOutcome, error) {
	if key == "" {
		return nil, fmt.Errorf("detail adapter key is required")
	}
	adapter, ok := r.adapters[key]
	if !ok {
		return nil, fmt.Errorf("unsupported detail adapter key: %s", key)
	}
	return adapter(modelRef, result)
}

func (r OutcomeAdapterRegistry) Len() int {
	return len(r.adapters)
}
