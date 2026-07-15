package ruleset

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// PublishedScaleSnapshots builds v2 published snapshots from active scale snapshots (oneoff seed).
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
