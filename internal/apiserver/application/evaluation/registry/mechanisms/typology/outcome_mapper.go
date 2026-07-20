package typology

import (
	"fmt"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// OutcomeAssembler 映射计分结果 到 测评结果 using 结果 mapping spec。
type OutcomeAssembler struct {
	registry OutcomeAdapterRegistry
}

// NewOutcomeAssembler 返回默认 类型学 结果组装器。
func NewOutcomeAssembler() OutcomeAssembler {
	return NewOutcomeAssemblerWithRegistry(DefaultOutcomeAdapterRegistry())
}

// NewOutcomeAssemblerWithRegistry 返回结果组装器 bound 到 特定 adapter 注册表。
func NewOutcomeAssemblerWithRegistry(registry OutcomeAdapterRegistry) OutcomeAssembler {
	return OutcomeAssembler{registry: registry}
}

// Assemble converts a scoring result to canonical Execution.
func (a OutcomeAssembler) Assemble(
	modelRef assessment.EvaluationModelRef,
	result outcometypology.ScoringResult,
	mapping modeltypology.OutcomeMappingSpec,
) (*domainoutcome.Execution, error) {
	adapterKey := mapping.ResolvedDetailAdapterKey(decisionKindFromResult(result))
	return a.registry.Assemble(adapterKey, modelRef, result)
}

func decisionKindFromResult(result outcometypology.ScoringResult) modelcatalog.DecisionKind {
	if result.Runtime != nil {
		return result.Runtime.Decision.Kind
	}
	return ""
}

func assembleGenericTraitProfileOutcome(
	modelRef assessment.EvaluationModelRef,
	result outcometypology.ScoringResult,
) (*domainoutcome.Execution, error) {
	detail, err := outcometypology.TraitProfileDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return executionFromTraitProfile(modelRef, detail), nil
}

func assembleGenericPersonalityTypeOutcome(
	modelRef assessment.EvaluationModelRef,
	result outcometypology.ScoringResult,
) (*domainoutcome.Execution, error) {
	detail, err := outcometypology.PersonalityTypeDetailFromPayload(result.Detail)
	if err != nil {
		return nil, err
	}
	return executionFromPersonalityType(modelRef, detail), nil
}

// AssembleFromPayload 推导mapping 从 载荷 和 assembles 结果。
func (a OutcomeAssembler) AssembleFromPayload(
	modelRef assessment.EvaluationModelRef,
	payload *modeltypology.Payload,
	result outcometypology.ScoringResult,
) (*domainoutcome.Execution, error) {
	if payload == nil {
		return nil, fmt.Errorf("typology payload is required")
	}
	spec, err := modeltypology.ResolveRuntimeSpec(nil, payload)
	if err != nil {
		return nil, err
	}
	return a.Assemble(modelRef, result, spec.OutcomeMapping)
}
