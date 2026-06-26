package assessmentmodel

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	mongoassessmentmodel "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/assessmentmodel"
	mongoruleset "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
)

// DualStore writes v2 published_assessment_models and reads v2 first, then legacy evaluation_rule_sets.
type DualStore struct {
	v2     *mongoassessmentmodel.Repository
	legacy *mongoruleset.Repository
}

var (
	_ port.PublishedReader = (*DualStore)(nil)
	_ port.PublishedWriter = (*DualStore)(nil)
)

func NewDualStore(v2 *mongoassessmentmodel.Repository, legacy *mongoruleset.Repository) *DualStore {
	return &DualStore{v2: v2, legacy: legacy}
}

func (s *DualStore) UpsertPublished(ctx context.Context, snapshot *domain.Snapshot) error {
	if s == nil || s.v2 == nil {
		return domain.ErrNotFound
	}
	return s.v2.UpsertPublished(ctx, snapshot)
}

func (s *DualStore) GetPublishedByRef(ctx context.Context, ref port.Ref) (*domain.Snapshot, error) {
	if s == nil {
		return nil, domain.ErrNotFound
	}
	if s.v2 != nil {
		snapshot, err := s.v2.GetPublishedByRef(ctx, ref)
		if err == nil {
			return snapshot, nil
		}
		if !domain.IsNotFound(err) {
			return nil, err
		}
	}
	if s.legacy == nil {
		return nil, domain.ErrNotFound
	}
	return s.legacy.GetPublishedByRef(ctx, ref)
}

func (s *DualStore) FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.Snapshot, error) {
	if s == nil {
		return nil, domain.ErrNotFound
	}
	if s.v2 != nil {
		snapshot, err := s.v2.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
		if err == nil {
			return snapshot, nil
		}
		if !domain.IsNotFound(err) {
			return nil, err
		}
	}
	if s.legacy == nil {
		return nil, domain.ErrNotFound
	}
	return s.legacy.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
}
