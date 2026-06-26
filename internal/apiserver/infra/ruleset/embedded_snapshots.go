package ruleset

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
)

// DefaultEmbeddedRuleSets 从内置 SBTI/MBTI seed 构建 RuleSetSnapshot 列表。
func DefaultEmbeddedRuleSets(ctx context.Context) ([]*domain.RuleSetSnapshot, error) {
	return defaultEmbeddedSnapshots(ctx)
}

// DefaultEmbeddedSnapshots 从内置 SBTI/MBTI seed 构建 RuleSetSnapshot 列表。
func DefaultEmbeddedSnapshots(ctx context.Context) ([]*domain.RuleSetSnapshot, error) {
	return defaultEmbeddedSnapshots(ctx)
}

func defaultEmbeddedSnapshots(_ context.Context) ([]*domain.RuleSetSnapshot, error) {
	sbtiModel, err := LoadDefaultSBTILegacyModel()
	if err != nil {
		return nil, err
	}
	mbtiModel, err := LoadDefaultMBTILegacyModel()
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
