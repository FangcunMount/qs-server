package query

import (
	"context"

	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
)

// PersonalityModelQueryService exposes C-side personality model catalog reads.
type PersonalityModelQueryService interface {
	GetPublishedByCode(ctx context.Context, code string) (*shared.PersonalityModelResult, error)
	ListPublished(ctx context.Context, dto shared.ListPersonalityModelsDTO) (*shared.PersonalityModelSummaryListResult, error)
	GetCategories(ctx context.Context) (*shared.PersonalityModelCategoriesResult, error)
}

type queryService struct {
	lister          port.PublishedLister
	algorithmLister port.PublishedAlgorithmLister
}

func NewQueryService(lister port.PublishedLister) PersonalityModelQueryService {
	return &queryService{lister: lister}
}

func NewQueryServiceWithAlgorithmLister(
	lister port.PublishedLister,
	algorithmLister port.PublishedAlgorithmLister,
) PersonalityModelQueryService {
	return &queryService{lister: lister, algorithmLister: algorithmLister}
}

func (s *queryService) GetPublishedByCode(ctx context.Context, code string) (*shared.PersonalityModelResult, error) {
	if s == nil || s.lister == nil {
		return nil, domain.ErrNotFound
	}
	snapshot, err := s.lister.FindPublishedByModelCode(ctx, domain.KindPersonality, code)
	if err != nil {
		return nil, err
	}
	return shared.DetailFromSnapshot(snapshot)
}

func (s *queryService) ListPublished(ctx context.Context, dto shared.ListPersonalityModelsDTO) (*shared.PersonalityModelSummaryListResult, error) {
	if s == nil || s.lister == nil {
		return &shared.PersonalityModelSummaryListResult{}, nil
	}
	page := dto.Page
	if page <= 0 {
		page = 1
	}
	pageSize := dto.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	filter := port.ListPublishedFilter{
		Kind:     domain.KindPersonality,
		Page:     page,
		PageSize: pageSize,
	}
	if dto.Algorithm != "" {
		filter.Algorithm = domain.Algorithm(dto.Algorithm)
	}
	snapshots, total, err := s.lister.ListPublished(ctx, filter)
	if err != nil {
		return nil, err
	}
	items := make([]shared.PersonalityModelSummaryResult, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		summary, err := shared.SummaryFromSnapshotOnly(snapshot)
		if err != nil {
			continue
		}
		items = append(items, summary)
	}
	totalPages := 0
	if pageSize > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &shared.PersonalityModelSummaryListResult{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *queryService) GetCategories(ctx context.Context) (*shared.PersonalityModelCategoriesResult, error) {
	algorithms, err := s.listPublishedAlgorithms(ctx)
	if err != nil {
		return nil, err
	}
	categories := make([]shared.PersonalityModelCategoryResult, 0, len(algorithms))
	for _, algorithm := range algorithms {
		categories = append(categories, shared.PersonalityModelCategoryResult{
			Value: string(algorithm),
			Label: algorithmCategoryLabel(algorithm),
		})
	}
	return &shared.PersonalityModelCategoriesResult{Categories: categories}, nil
}

func (s *queryService) listPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error) {
	if s != nil && s.algorithmLister != nil {
		algorithms, err := s.algorithmLister.ListPublishedAlgorithms(ctx)
		if err != nil {
			return nil, err
		}
		if len(algorithms) > 0 {
			return algorithms, nil
		}
	}
	return typologyeval.DefaultAlgorithms(), nil
}

func algorithmCategoryLabel(algorithm domain.Algorithm) string {
	return typologyeval.CategoryLabelFor(algorithm)
}
