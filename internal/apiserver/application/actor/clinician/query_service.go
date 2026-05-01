package clinician

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
)

type queryService struct {
	clinicianReader       actorreadmodel.ClinicianReader
	relationReader        actorreadmodel.RelationReader
	assessmentEntryReader actorreadmodel.AssessmentEntryReader
}

// NewQueryService 创建从业者查询服务。
func NewQueryService(
	clinicianReader actorreadmodel.ClinicianReader,
	relationReader actorreadmodel.RelationReader,
	assessmentEntryReader actorreadmodel.AssessmentEntryReader,
) ClinicianQueryService {
	return &queryService{
		clinicianReader:       clinicianReader,
		relationReader:        relationReader,
		assessmentEntryReader: assessmentEntryReader,
	}
}

func (s *queryService) GetByID(ctx context.Context, clinicianID uint64) (*ClinicianResult, error) {
	targetClinicianID, err := clinicianIDFromUint64("clinician_id", clinicianID)
	if err != nil {
		return nil, err
	}
	item, err := s.clinicianReader.GetClinician(ctx, targetClinicianID.Uint64())
	if err != nil {
		return nil, errors.Wrap(err, "failed to find clinician")
	}
	return s.enrichCounts(ctx, toClinicianResultFromRow(item))
}

func (s *queryService) GetByOperator(ctx context.Context, orgID int64, operatorID uint64) (*ClinicianResult, error) {
	item, err := s.clinicianReader.FindClinicianByOperator(ctx, orgID, operatorID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find clinician by operator")
	}
	return s.enrichCounts(ctx, toClinicianResultFromRow(item))
}

func (s *queryService) ListClinicians(ctx context.Context, dto ListClinicianDTO) (*ClinicianListResult, error) {
	items, err := s.clinicianReader.ListClinicians(ctx, actorreadmodel.ClinicianFilter{
		OrgID:  dto.OrgID,
		Offset: dto.Offset,
		Limit:  dto.Limit,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list clinicians")
	}

	totalCount, err := s.clinicianReader.CountClinicians(ctx, dto.OrgID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count clinicians")
	}

	results := make([]*ClinicianResult, 0, len(items))
	for i := range items {
		enriched, err := s.enrichCounts(ctx, toClinicianResultFromRow(&items[i]))
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
	if s.relationReader != nil {
		ids, err := s.relationReader.ListActiveTesteeIDsByClinician(
			ctx,
			item.OrgID,
			item.ID,
			relationTypesToStrings(domainRelation.AccessGrantRelationTypes()),
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to count accessible testees")
		}
		seen := make(map[uint64]struct{}, len(ids))
		for _, id := range ids {
			seen[id] = struct{}{}
		}
		item.AssignedTesteeCount = int64(len(seen))
	}
	if s.assessmentEntryReader != nil {
		count, err := s.assessmentEntryReader.CountAssessmentEntriesByClinician(ctx, item.OrgID, item.ID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to count assessment entries")
		}
		item.AssessmentEntryCount = count
	}
	return item, nil
}
