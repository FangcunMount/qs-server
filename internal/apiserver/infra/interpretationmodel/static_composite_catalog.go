package interpretationmodel

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretationmodel"
	evaluationinputPort "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationmodel"
)

type StaticCompositeCatalog struct {
	sbti evaluationinputPort.SBTIModelCatalog
	mbti evaluationinputPort.MBTIModelCatalog
}

var _ port.ModelCatalog = (*StaticCompositeCatalog)(nil)

func NewStaticCompositeCatalog(
	sbti evaluationinputPort.SBTIModelCatalog,
	mbti evaluationinputPort.MBTIModelCatalog,
) *StaticCompositeCatalog {
	return &StaticCompositeCatalog{sbti: sbti, mbti: mbti}
}

func (c *StaticCompositeCatalog) ResolveByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (port.ModelRef, bool, error) {
	if c == nil {
		return port.ModelRef{}, false, nil
	}
	if c.sbti != nil {
		if model, err := c.sbti.FindSBTIModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion); err == nil && model != nil {
			return sbtiModelRef(model), true, nil
		}
	}
	if c.mbti != nil {
		if model, err := c.mbti.FindMBTIModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion); err == nil && model != nil {
			return mbtiModelRef(model), true, nil
		}
	}
	return port.ModelRef{}, false, nil
}

func (c *StaticCompositeCatalog) GetPublishedByRef(ctx context.Context, ref port.ModelRef) (*domain.RuleSetSnapshot, error) {
	if c == nil {
		return nil, fmt.Errorf("interpretation model catalog is not configured")
	}
	switch ref.Kind {
	case domain.ModelKindSBTI:
		if c.sbti == nil {
			return nil, fmt.Errorf("sbti model catalog is not configured")
		}
		model, err := c.sbti.GetSBTIModelByRef(ctx, evaluationinputPort.ModelRef{
			Kind:    evaluationinputPort.EvaluationModelKindSBTI,
			Code:    ref.Code,
			Version: ref.Version,
		})
		if err != nil {
			return nil, err
		}
		return sbtiRuleSetSnapshot(model)
	case domain.ModelKindMBTI:
		if c.mbti == nil {
			return nil, fmt.Errorf("mbti model catalog is not configured")
		}
		model, err := c.mbti.GetMBTIModelByRef(ctx, evaluationinputPort.ModelRef{
			Kind:    evaluationinputPort.EvaluationModelKindMBTI,
			Code:    ref.Code,
			Version: ref.Version,
		})
		if err != nil {
			return nil, err
		}
		return mbtiRuleSetSnapshot(model)
	default:
		return nil, fmt.Errorf("unsupported interpretation model kind: %s", ref.Kind)
	}
}

func (c *StaticCompositeCatalog) FindPublishedByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (*domain.RuleSetSnapshot, error) {
	ref, ok, err := c.ResolveByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrNotFound
	}
	return c.GetPublishedByRef(ctx, ref)
}

func sbtiRuleSetSnapshot(model *evaluationinputPort.SBTIModelSnapshot) (*domain.RuleSetSnapshot, error) {
	return SBTIRuleSetSnapshot(model)
}

func mbtiRuleSetSnapshot(model *evaluationinputPort.MBTIModelSnapshot) (*domain.RuleSetSnapshot, error) {
	return MBTIRuleSetSnapshot(model)
}
