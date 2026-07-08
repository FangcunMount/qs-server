package ruleset

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
)

// DefaultEmbeddedRuleSets builds v2 published snapshots from embedded SBTI/MBTI seed.
func DefaultEmbeddedRuleSets(ctx context.Context) ([]*domain.PublishedModelSnapshot, error) {
	return defaultEmbeddedSnapshots(ctx)
}

// DefaultEmbeddedSnapshots builds v2 published snapshots from embedded SBTI/MBTI seed.
func DefaultEmbeddedSnapshots(ctx context.Context) ([]*domain.PublishedModelSnapshot, error) {
	return defaultEmbeddedSnapshots(ctx)
}

func defaultEmbeddedSnapshots(_ context.Context) ([]*domain.PublishedModelSnapshot, error) {
	sbtiModel, err := LoadDefaultSBTILegacyModel()
	if err != nil {
		return nil, err
	}
	mbtiModel, err := LoadDefaultMBTILegacyModel()
	if err != nil {
		return nil, err
	}
	sbtiSnapshot, err := aminfra.BuildSBTIPublishedSnapshot(sbtiModel)
	if err != nil {
		return nil, err
	}
	mbtiSnapshot, err := aminfra.BuildMBTIPublishedSnapshot(mbtiModel)
	if err != nil {
		return nil, err
	}
	return []*domain.PublishedModelSnapshot{sbtiSnapshot, mbtiSnapshot}, nil
}
