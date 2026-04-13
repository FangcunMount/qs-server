package clinician

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	assessmentEntryDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
)

type queryService struct {
	repo                domainClinician.Repository
	relationRepo        domainRelation.Repository
	assessmentEntryRepo assessmentEntryDomain.Repository
}

// NewQueryService 创建从业者查询服务。
func NewQueryService(
	repo domainClinician.Repository,
	relationRepo domainRelation.Repository,
	assessmentEntryRepo assessmentEntryDomain.Repository,
) ClinicianQueryService {
	return &queryService{
		repo:                repo,
		relationRepo:        relationRepo,
		assessmentEntryRepo: assessmentEntryRepo,
	}
}

func (s *queryService) GetByID(ctx context.Context, clinicianID uint64) (*ClinicianResult, error) {
	item, err := s.repo.FindByID(ctx, domainClinician.ID(clinicianID))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find clinician")
	}
	return s.enrichCounts(ctx, toClinicianResult(item))
}

func (s *queryService) GetByOperator(ctx context.Context, orgID int64, operatorID uint64) (*ClinicianResult, error) {
	item, err := s.repo.FindByOperator(ctx, orgID, operatorID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find clinician by operator")
	}
	return s.enrichCounts(ctx, toClinicianResult(item))
}

func (s *queryService) ListClinicians(ctx context.Context, dto ListClinicianDTO) (*ClinicianListResult, error) {
	items, err := s.repo.ListByOrg(ctx, dto.OrgID, dto.Offset, dto.Limit)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list clinicians")
	}

	totalCount, err := s.repo.Count(ctx, dto.OrgID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count clinicians")
	}

	results := make([]*ClinicianResult, 0, len(items))
	for _, item := range items {
		enriched, err := s.enrichCounts(ctx, toClinicianResult(item))
		if err != nil {
			return nil, err
		}
		results = append(results, enriched)
	}

	return &ClinicianListResult{
		Items:      results,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}

func (s *queryService) enrichCounts(ctx context.Context, item *ClinicianResult) (*ClinicianResult, error) {
	if item == nil {
		return nil, nil
	}
	if s.relationRepo != nil {
		count, err := s.relationRepo.CountActiveByClinician(
			ctx,
			item.OrgID,
			domainClinician.ID(item.ID),
			[]domainRelation.RelationType{domainRelation.RelationTypeAssigned},
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to count assigned testees")
		}
		item.AssignedTesteeCount = count
	}
	if s.assessmentEntryRepo != nil {
		count, err := s.assessmentEntryRepo.CountByClinician(ctx, item.OrgID, domainClinician.ID(item.ID))
		if err != nil {
			return nil, errors.Wrap(err, "failed to count assessment entries")
		}
		item.AssessmentEntryCount = count
	}
	return item, nil
}
