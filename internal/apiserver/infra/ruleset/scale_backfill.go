package ruleset

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	mongoScale "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
)

// PublishedScaleSnapshots builds v2 published snapshots from active Mongo scale rows (oneoff seed).
func PublishedScaleSnapshots(ctx context.Context, repo *mongoScale.Repository) ([]*domain.PublishedModelSnapshot, error) {
	if repo == nil {
		return nil, nil
	}
	scales, err := repo.ListActivePublishedSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	return ScaleSnapshotsFromMedicalScales(scales)
}

// ScaleSnapshotsFromMedicalScales converts published medical scales to v2 snapshots.
func ScaleSnapshotsFromMedicalScales(scales []*scaledefinition.MedicalScale) ([]*domain.PublishedModelSnapshot, error) {
	snapshots := make([]*domain.PublishedModelSnapshot, 0, len(scales))
	for _, scale := range scales {
		if scale == nil {
			continue
		}
		snapshot, err := aminfra.BuildScalePublishedSnapshot(evaluationinputInfra.MedicalScaleToSnapshot(scale))
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

// PublishedScaleRuleSetSnapshots is deprecated; use PublishedScaleSnapshots.
func PublishedScaleRuleSetSnapshots(ctx context.Context, repo *mongoScale.Repository) ([]*domain.PublishedModelSnapshot, error) {
	return PublishedScaleSnapshots(ctx, repo)
}
