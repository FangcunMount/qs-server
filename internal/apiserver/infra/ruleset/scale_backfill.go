package ruleset

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/definition"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	mongoScale "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
)

// PublishedScaleRuleSetSnapshots 从 Mongo 已发布量表快照构建规则集（backfill 用）。
func PublishedScaleRuleSetSnapshots(ctx context.Context, repo *mongoScale.Repository) ([]*domain.RuleSetSnapshot, error) {
	if repo == nil {
		return nil, nil
	}
	scales, err := repo.ListActivePublishedSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	snapshots := make([]*domain.RuleSetSnapshot, 0, len(scales))
	for _, scale := range scales {
		snapshot, err := ScaleRuleSetSnapshot(evaluationinputInfra.MedicalScaleToSnapshot(scale))
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

// ScaleRuleSetSnapshotsFromMedicalScales 将领域量表列表转为规则集快照。
func ScaleRuleSetSnapshotsFromMedicalScales(scales []*scaledefinition.MedicalScale) ([]*domain.RuleSetSnapshot, error) {
	snapshots := make([]*domain.RuleSetSnapshot, 0, len(scales))
	for _, scale := range scales {
		if scale == nil {
			continue
		}
		snapshot, err := ScaleRuleSetSnapshot(evaluationinputInfra.MedicalScaleToSnapshot(scale))
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}
