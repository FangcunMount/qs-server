package ruleset

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	evaluationinputPort "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// DefaultEmbeddedRuleSets 从内置 SBTI/MBTI seed 构建 RuleSetSnapshot 列表。
func DefaultEmbeddedRuleSets(ctx context.Context) ([]*domain.RuleSetSnapshot, error) {
	return defaultEmbeddedSnapshots(ctx)
}

// DefaultEmbeddedSnapshots 从内置 SBTI/MBTI seed 构建 RuleSetSnapshot 列表。
func DefaultEmbeddedSnapshots(ctx context.Context) ([]*domain.RuleSetSnapshot, error) {
	return defaultEmbeddedSnapshots(ctx)
}

func defaultEmbeddedSnapshots(ctx context.Context) ([]*domain.RuleSetSnapshot, error) {
	sbtiCatalog, err := evaluationinputInfra.NewDefaultSBTIModelCatalog()
	if err != nil {
		return nil, err
	}
	mbtiCatalog, err := evaluationinputInfra.NewDefaultMBTIModelCatalog()
	if err != nil {
		return nil, err
	}
	sbtiModel, err := sbtiCatalog.GetSBTIModelByRef(ctx, evaluationinputPort.ModelRef{
		Code: evaluationinputPort.DefaultSBTIModelCode,
	})
	if err != nil {
		return nil, err
	}
	mbtiModel, err := mbtiCatalog.GetMBTIModelByRef(ctx, evaluationinputPort.ModelRef{
		Code: evaluationinputPort.DefaultMBTIModelCode,
	})
	if err != nil {
		return nil, err
	}
	sbtiSnapshot, err := SBTIRuleSetSnapshot(sbtiModel)
	if err != nil {
		return nil, err
	}
	mbtiSnapshot, err := MBTIRuleSetSnapshot(mbtiModel)
	if err != nil {
		return nil, err
	}
	return []*domain.RuleSetSnapshot{sbtiSnapshot, mbtiSnapshot}, nil
}
