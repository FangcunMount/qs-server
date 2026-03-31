package clinician

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
)

type queryService struct {
	repo domainClinician.Repository
}

// NewQueryService 创建从业者查询服务。
func NewQueryService(repo domainClinician.Repository) ClinicianQueryService {
	return &queryService{repo: repo}
}

func (s *queryService) GetByID(ctx context.Context, clinicianID uint64) (*ClinicianResult, error) {
	item, err := s.repo.FindByID(ctx, domainClinician.ID(clinicianID))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find clinician")
	}
	return toClinicianResult(item), nil
}

func (s *queryService) GetByOperator(ctx context.Context, orgID int64, operatorID uint64) (*ClinicianResult, error) {
	item, err := s.repo.FindByOperator(ctx, orgID, operatorID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find clinician by operator")
	}
	return toClinicianResult(item), nil
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
		results = append(results, toClinicianResult(item))
	}

	return &ClinicianListResult{
		Items:      results,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}
