package ruleset

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// PublishedScaleSnapshots builds v2 published snapshots from published_assessment_models scale rows (oneoff seed).
func PublishedScaleSnapshots(ctx context.Context, repo *mongomodelcatalog.Repository) ([]*port.PublishedModel, error) {
	if repo == nil {
		return nil, nil
	}
	rows, _, err := repo.ListPublishedModels(ctx, port.ListPublishedFilter{Kind: domain.KindScale})
	if err != nil {
		return nil, err
	}
	return append([]*port.PublishedModel(nil), rows...), nil
}

// ScaleSnapshotsFromMedicalScales converts published medical scales to v2 snapshots.
func ScaleSnapshotsFromMedicalScales(scales []*scaledefinition.MedicalScale) ([]*port.PublishedModel, error) {
	snapshots := make([]*port.PublishedModel, 0, len(scales))
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
func PublishedScaleRuleSetSnapshots(ctx context.Context, repo *mongomodelcatalog.Repository) ([]*port.PublishedModel, error) {
	return PublishedScaleSnapshots(ctx, repo)
}
