package query

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology/consumer/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// TypologyModelQueryService 暴露 C 端 typology 模型目录 reads。
type TypologyModelQueryService interface {
	GetPublishedByCode(ctx context.Context, code string) (*shared.TypologyModelResult, error)
	ListPublished(ctx context.Context, dto shared.ListTypologyModelsDTO) (*shared.TypologyModelSummaryListResult, error)
	GetCategories(ctx context.Context) (*shared.TypologyModelCategoriesResult, error)
}

type queryService struct {
	lister          port.PublishedModelLister
	algorithmLister port.PublishedAlgorithmLister
}

func NewQueryService(lister port.PublishedModelLister) TypologyModelQueryService {
	return &queryService{lister: lister}
}

func NewQueryServiceWithAlgorithmLister(
	lister port.PublishedModelLister,
	algorithmLister port.PublishedAlgorithmLister,
) TypologyModelQueryService {
	return &queryService{lister: lister, algorithmLister: algorithmLister}
}

func (s *queryService) GetPublishedByCode(ctx context.Context, code string) (*shared.TypologyModelResult, error) {
	if s == nil || s.lister == nil {
		return nil, domain.ErrNotFound
	}
	snapshot, err := s.lister.FindPublishedModelByCode(ctx, domain.KindPersonality, code)
	if err != nil {
		return nil, err
	}
	return shared.DetailFromPublishedModel(snapshot)
}

func (s *queryService) ListPublished(ctx context.Context, dto shared.ListTypologyModelsDTO) (*shared.TypologyModelSummaryListResult, error) {
	if s == nil || s.lister == nil {
		return &shared.TypologyModelSummaryListResult{}, nil
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
	snapshots, total, err := s.lister.ListPublishedModels(ctx, filter)
	if err != nil {
		return nil, err
	}
	items := make([]shared.TypologyModelSummaryResult, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		summary, err := shared.SummaryFromPublishedModel(snapshot)
		if err != nil {
			continue
		}
		items = append(items, summary)
	}
	totalPages := 0
	if pageSize > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &shared.TypologyModelSummaryListResult{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *queryService) GetCategories(ctx context.Context) (*shared.TypologyModelCategoriesResult, error) {
	algorithms, err := s.listPublishedAlgorithms(ctx)
	if err != nil {
		return nil, err
	}
	categories := make([]shared.TypologyModelCategoryResult, 0, len(algorithms))
	for _, algorithm := range algorithms {
		categories = append(categories, shared.TypologyModelCategoryResult{
			Value: string(algorithm),
			Label: algorithmCategoryLabel(algorithm),
		})
	}
	return &shared.TypologyModelCategoriesResult{Categories: categories}, nil
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
	return defaultPublishedAlgorithms(), nil
}

func algorithmCategoryLabel(algorithm domain.Algorithm) string {
	return modeltypology.LegacyReportSpecFromAlgorithm(algorithm).CategoryLabel
}

func defaultPublishedAlgorithms() []domain.Algorithm {
	return []domain.Algorithm{
		domain.AlgorithmMBTI,
		domain.AlgorithmSBTI,
		domain.AlgorithmBigFive,
	}
}
