package assessmentmodel

import (
	"context"
	"sort"

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
	_ port.PublishedReader          = (*DualStore)(nil)
	_ port.PublishedLister          = (*DualStore)(nil)
	_ port.PublishedWriter          = (*DualStore)(nil)
	_ port.PublishedAlgorithmLister = (*DualStore)(nil)
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

func (s *DualStore) FindPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.Snapshot, error) {
	if s == nil {
		return nil, domain.ErrNotFound
	}
	if s.v2 != nil {
		snapshot, err := s.v2.FindPublishedByModelCode(ctx, kind, code)
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
	snapshots, err := s.legacy.ListPublished(ctx)
	if err != nil {
		return nil, err
	}
	for _, snapshot := range snapshots {
		if snapshot == nil || snapshot.Definition.Code != code {
			continue
		}
		if kind == domain.KindPersonality {
			switch snapshot.Definition.Kind {
			case domain.KindPersonality, domain.KindMBTIMigration, domain.KindSBTIMigration:
				return snapshot, nil
			}
			continue
		}
		if snapshot.Definition.Kind == kind {
			return snapshot, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (s *DualStore) ListPublished(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.Snapshot, int64, error) {
	if s == nil {
		return nil, 0, domain.ErrNotFound
	}
	if s.v2 != nil {
		snapshots, total, err := s.v2.ListPublished(ctx, filter)
		if err != nil {
			return nil, 0, err
		}
		if total > 0 || s.legacy == nil {
			return snapshots, total, nil
		}
	}
	if s.legacy == nil {
		return nil, 0, nil
	}
	all, err := s.legacy.ListPublished(ctx)
	if err != nil {
		return nil, 0, err
	}
	filtered := filterLegacySnapshots(all, filter)
	total := int64(len(filtered))
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	start := (page - 1) * pageSize
	if start >= len(filtered) {
		return []*domain.Snapshot{}, total, nil
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func filterLegacySnapshots(all []*domain.Snapshot, filter port.ListPublishedFilter) []*domain.Snapshot {
	out := make([]*domain.Snapshot, 0, len(all))
	for _, snapshot := range all {
		if snapshot == nil {
			continue
		}
		if filter.Kind == domain.KindPersonality {
			switch snapshot.Definition.Kind {
			case domain.KindPersonality, domain.KindMBTIMigration, domain.KindSBTIMigration:
			default:
				continue
			}
		} else if filter.Kind != "" && snapshot.Definition.Kind != filter.Kind {
			continue
		}
		if filter.Algorithm != "" {
			algorithm, err := resolveLegacyAlgorithm(snapshot)
			if err != nil || algorithm != filter.Algorithm {
				continue
			}
		}
		out = append(out, snapshot)
	}
	return out
}

func resolveLegacyAlgorithm(snapshot *domain.Snapshot) (domain.Algorithm, error) {
	if snapshot == nil {
		return "", domain.ErrNotFound
	}
	if domain.IsPersonalityTypologyPayloadFormat(snapshot.PayloadFormat) {
		return domain.AlgorithmFromTypologyPayload(snapshot.Payload)
	}
	switch snapshot.Definition.Kind {
	case domain.KindMBTIMigration:
		return domain.AlgorithmMBTI, nil
	case domain.KindSBTIMigration:
		return domain.AlgorithmSBTI, nil
	default:
		return "", nil
	}
}

func (s *DualStore) ListPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error) {
	if s == nil {
		return nil, domain.ErrNotFound
	}
	parts := make([][]domain.Algorithm, 0, 2)
	if s.v2 != nil {
		algorithms, err := s.v2.ListPublishedAlgorithms(ctx)
		if err != nil {
			return nil, err
		}
		parts = append(parts, algorithms)
	}
	if s.legacy != nil {
		all, err := s.legacy.ListPublished(ctx)
		if err != nil {
			return nil, err
		}
		legacyAlgorithms := make([]domain.Algorithm, 0)
		for _, snapshot := range all {
			algorithm, err := resolveLegacyAlgorithm(snapshot)
			if err != nil {
				continue
			}
			if algorithm != "" {
				legacyAlgorithms = append(legacyAlgorithms, algorithm)
			}
		}
		parts = append(parts, legacyAlgorithms)
	}
	return mergeAlgorithmSets(parts...), nil
}

func mergeAlgorithmSets(parts ...[]domain.Algorithm) []domain.Algorithm {
	seen := make(map[domain.Algorithm]struct{})
	for _, algorithms := range parts {
		for _, algorithm := range algorithms {
			if algorithm == "" {
				continue
			}
			seen[algorithm] = struct{}{}
		}
	}
	out := make([]domain.Algorithm, 0, len(seen))
	for algorithm := range seen {
		out = append(out, algorithm)
	}
	sortAlgorithms(out)
	return out
}

func sortAlgorithms(algorithms []domain.Algorithm) {
	order := map[domain.Algorithm]int{
		domain.AlgorithmMBTI:    0,
		domain.AlgorithmSBTI:    1,
		domain.AlgorithmBigFive: 2,
	}
	sort.Slice(algorithms, func(i, j int) bool {
		left, okLeft := order[algorithms[i]]
		right, okRight := order[algorithms[j]]
		switch {
		case okLeft && okRight:
			return left < right
		case okLeft:
			return true
		case okRight:
			return false
		default:
			return algorithms[i] < algorithms[j]
		}
	})
}
