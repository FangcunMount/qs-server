package modelcatalog

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	mongoruleset "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset"
)

// BackfillFromLegacy copies published rows from evaluation_rule_sets into published_assessment_models.
func BackfillFromLegacy(ctx context.Context, legacy *mongoruleset.Repository, target *Repository) (int, error) {
	if legacy == nil || target == nil {
		return 0, fmt.Errorf("legacy and target repositories are required")
	}
	rows, err := legacy.ListPublished(ctx)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, snapshot := range rows {
		if snapshot == nil {
			continue
		}
		published := domain.PublishedFromLegacy(snapshot)
		if published == nil {
			continue
		}
		if err := target.UpsertPublishedModel(ctx, published); err != nil {
			return written, fmt.Errorf("upsert %s@%s: %w", published.Model.Code, published.Model.Version, err)
		}
		written++
	}
	return written, nil
}
