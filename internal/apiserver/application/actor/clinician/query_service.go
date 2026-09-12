package clinician

import (
	"context"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	"github.com/FangcunMount/qs-server/internal/pkg/code"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
)

type queryService struct {
	operatorOnly          bool
	scope                 SummaryScope
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

func NewOperatorQueryService(reader actorreadmodel.ClinicianReader, relations actorreadmodel.RelationReader, entries actorreadmodel.AssessmentEntryReader, scope SummaryScope) ClinicianQueryService {
	s := NewQueryService(reader, relations, entries).(*queryService)
	s.operatorOnly = true
	s.scope = scope
	return s
}

func (s *queryService) GetByID(ctx context.Context, clinicianID uint64) (*ClinicianResult, error) {
	item, err := s.GetBasicByID(ctx, clinicianID)
	if err != nil {
		return nil, err
	}
	return s.enrichCounts(ctx, item)
}

func (s *queryService) GetBasicByID(ctx context.Context, clinicianID uint64) (*ClinicianResult, error) {
	if s.operatorOnly {
		if err := authorizeHeadquarters(ctx, s.scope, actorctx.OperatorOrgID(ctx), "read"); err != nil {
			return nil, err
		}
	}

	targetClinicianID, err := clinicianIDFromUint64("clinician_id", clinicianID)
	if err != nil {
		return nil, err
	}
	item, err := s.clinicianReader.GetClinician(ctx, targetClinicianID.Uint64())
	if err != nil {
		return nil, errors.Wrap(err, "failed to find clinician")
	}
	if s.operatorOnly && (item == nil || item.OrgID != actorctx.OperatorOrgID(ctx)) {
		return nil, errors.WithCode(code.ErrUserNotFound, "clinician not found in current company")
	}
	return toClinicianResultFromRow(item), nil
}

func (s *queryService) ListClinicians(ctx context.Context, dto ListClinicianDTO) (*ClinicianListResult, error) {
	if s.operatorOnly {
		if err := authorizeHeadquarters(ctx, s.scope, dto.OrgID, "list"); err != nil {
			return nil, err
		}
	}

	if dto.StoreID != nil && (*dto.StoreID == 0 || dto.Unconfigured) {
		return nil, errors.WithCode(code.ErrInvalidArgument, "store_id and unconfigured filters are mutually exclusive")
	}
	filter := actorreadmodel.ClinicianFilter{StoreID: dto.StoreID, Unconfigured: dto.Unconfigured,
		OrgID:  dto.OrgID,
		Offset: dto.Offset,
		Limit:  dto.Limit,
	}
	items, err := s.clinicianReader.ListClinicians(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list clinicians")
	}

	totalCount, err := s.clinicianReader.CountClinicians(ctx, filter)
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
		ids, err := s.assignedTesteeIDsForCount(ctx, item)

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

func (s *queryService) assignedTesteeIDsForCount(ctx context.Context, item *ClinicianResult) ([]uint64, error) {
	if !s.operatorOnly {
		return s.relationReader.ListActiveTesteeIDsByClinician(ctx, item.OrgID, item.ID, relationTypesToStrings(domainRelation.AccessGrantRelationTypes()))
	}
	guard := relationshipService{operatorOnly: true, operatorScope: s.scope}
	filter, err := guard.operatorRelationFilter(ctx, actorreadmodel.RelationFilter{OrgID: item.OrgID, ClinicianID: item.ID, ActiveOnly: true, RelationTypes: relationTypesToStrings(domainRelation.AccessGrantRelationTypes())}, "list")
	if err != nil {
		if errors.IsCode(err, code.ErrPermissionDenied) {
			return nil, nil
		}
		return nil, err
	}
	rows, _, err := s.relationReader.ListAssignedTestees(ctx, filter)
	if err != nil {
		return nil, err
	}
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids, nil
}
