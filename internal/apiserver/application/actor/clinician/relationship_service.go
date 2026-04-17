package clinician

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

type relationshipService struct {
	relationRepo   domainRelation.Repository
	clinicianRepo  domainClinician.Repository
	testeeRepo     domainTestee.Repository
	behaviorEvents BehaviorEventStager
	uow            *mysql.UnitOfWork
}

// NewRelationshipService 创建从业者关系服务。
func NewRelationshipService(
	relationRepo domainRelation.Repository,
	clinicianRepo domainClinician.Repository,
	testeeRepo domainTestee.Repository,
	behaviorEvents BehaviorEventStager,
	uow *mysql.UnitOfWork,
) ClinicianRelationshipService {
	return &relationshipService{
		relationRepo:   relationRepo,
		clinicianRepo:  clinicianRepo,
		testeeRepo:     testeeRepo,
		behaviorEvents: behaviorEvents,
		uow:            uow,
	}
}

func (s *relationshipService) AssignTestee(ctx context.Context, dto AssignTesteeDTO) (*RelationResult, error) {
	normalizedType, err := normalizeAssignmentRelationType(dto.RelationType)
	if err != nil {
		return nil, err
	}
	dto.RelationType = string(normalizedType)
	return s.assignRelation(ctx, dto)
}

func (s *relationshipService) AssignPrimary(ctx context.Context, dto AssignTesteeDTO) (*RelationResult, error) {
	dto.RelationType = string(domainRelation.RelationTypePrimary)
	return s.assignRelation(ctx, dto)
}

func (s *relationshipService) AssignAttending(ctx context.Context, dto AssignTesteeDTO) (*RelationResult, error) {
	dto.RelationType = string(domainRelation.RelationTypeAttending)
	return s.assignRelation(ctx, dto)
}

func (s *relationshipService) AssignCollaborator(ctx context.Context, dto AssignTesteeDTO) (*RelationResult, error) {
	dto.RelationType = string(domainRelation.RelationTypeCollaborator)
	return s.assignRelation(ctx, dto)
}

func (s *relationshipService) TransferPrimary(ctx context.Context, dto TransferPrimaryDTO) (*RelationResult, error) {
	sourceType := dto.SourceType
	if sourceType == "" {
		sourceType = string(domainRelation.SourceTypeTransfer)
	}
	var result *domainRelation.ClinicianTesteeRelation
	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		var fromClinicianID uint64
		existingPrimary, err := s.relationRepo.FindActivePrimaryByTestee(txCtx, dto.OrgID, domainTestee.ID(dto.TesteeID))
		if err != nil && !errors.IsCode(err, code.ErrUserNotFound) {
			return errors.Wrap(err, "failed to find active primary relation before transfer")
		}
		if err == nil && existingPrimary != nil {
			fromClinicianID = existingPrimary.ClinicianID().Uint64()
		}
		result, err = s.assignRelationTx(txCtx, AssignTesteeDTO{
			OrgID:        dto.OrgID,
			ClinicianID:  dto.ToClinicianID,
			TesteeID:     dto.TesteeID,
			RelationType: string(domainRelation.RelationTypePrimary),
			SourceType:   sourceType,
			SourceID:     dto.SourceID,
		})
		if err != nil {
			return err
		}
		if s.behaviorEvents != nil {
			if err := s.behaviorEvents.StageCareRelationshipTransferred(txCtx, dto.OrgID, fromClinicianID, dto.ToClinicianID, dto.TesteeID, time.Now()); err != nil {
				return errors.Wrap(err, "failed to stage care relationship transferred event")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return toRelationResult(result), nil
}

func (s *relationshipService) assignRelation(ctx context.Context, dto AssignTesteeDTO) (*RelationResult, error) {
	var result *domainRelation.ClinicianTesteeRelation
	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.assignRelationTx(txCtx, dto)
		if err != nil {
			return err
		}
		result = item
		return nil
	})
	if err != nil {
		return nil, err
	}

	return toRelationResult(result), nil
}

func (s *relationshipService) assignRelationTx(ctx context.Context, dto AssignTesteeDTO) (*domainRelation.ClinicianTesteeRelation, error) {
	relationshipType, err := normalizeAssignmentRelationType(dto.RelationType)
	if err != nil {
		return nil, err
	}

	sourceType := domainRelation.SourceType(dto.SourceType)
	if sourceType == "" {
		sourceType = domainRelation.SourceTypeManual
	}

	clinicianItem, err := s.clinicianRepo.FindByID(ctx, domainClinician.ID(dto.ClinicianID))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find clinician")
	}
	if clinicianItem.OrgID() != dto.OrgID {
		return nil, errors.WithCode(code.ErrInvalidArgument, "clinician does not belong to the requested organization")
	}

	testeeItem, err := s.testeeRepo.FindByID(ctx, domainTestee.ID(dto.TesteeID))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find testee")
	}
	if testeeItem.OrgID() != dto.OrgID {
		return nil, errors.WithCode(code.ErrInvalidArgument, "testee does not belong to the requested organization")
	}

	now := time.Now()
	if relationshipType == domainRelation.RelationTypePrimary {
		existingPrimaryRelation, err := s.relationRepo.FindActivePrimaryByTestee(
			ctx,
			dto.OrgID,
			domainTestee.ID(dto.TesteeID),
		)
		if err != nil && !errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.Wrap(err, "failed to find active primary relation")
		}
		if err == nil && existingPrimaryRelation != nil {
			if existingPrimaryRelation.ClinicianID() == domainClinician.ID(dto.ClinicianID) {
				return existingPrimaryRelation, nil
			}
			existingPrimaryRelation.Unbind(now)
			if err := s.relationRepo.Update(ctx, existingPrimaryRelation); err != nil {
				return nil, errors.Wrap(err, "failed to unbind existing primary relation")
			}
		}
	}

	existingRelation, err := s.relationRepo.FindActiveByTypes(
		ctx,
		dto.OrgID,
		domainClinician.ID(dto.ClinicianID),
		domainTestee.ID(dto.TesteeID),
		domainRelation.AccessGrantRelationTypes(),
	)
	if err != nil && !errors.IsCode(err, code.ErrUserNotFound) {
		return nil, errors.Wrap(err, "failed to find existing access relation")
	}
	if err == nil && existingRelation != nil {
		if existingRelation.RelationType() == relationshipType {
			return existingRelation, nil
		}
		existingRelation.Unbind(now)
		if err := s.relationRepo.Update(ctx, existingRelation); err != nil {
			return nil, errors.Wrap(err, "failed to replace existing access relation")
		}
	}

	result := domainRelation.NewClinicianTesteeRelation(
		dto.OrgID,
		domainClinician.ID(dto.ClinicianID),
		domainTestee.ID(dto.TesteeID),
		relationshipType,
		sourceType,
		dto.SourceID,
		true,
		now,
		nil,
	)
	if err := s.relationRepo.Save(ctx, result); err != nil {
		return nil, errors.Wrap(err, "failed to save relation")
	}
	return result, nil
}

func (s *relationshipService) UnbindRelation(ctx context.Context, relationID uint64) (*RelationResult, error) {
	var result *domainRelation.ClinicianTesteeRelation

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.relationRepo.FindByID(txCtx, domainRelation.ID(relationID))
		if err != nil {
			return errors.Wrap(err, "failed to find relation")
		}
		if !item.IsActive() {
			result = item
			return nil
		}
		item.Unbind(time.Now())
		if err := s.relationRepo.Update(txCtx, item); err != nil {
			return errors.Wrap(err, "failed to unbind relation")
		}
		result = item
		return nil
	})
	if err != nil {
		return nil, err
	}

	return toRelationResult(result), nil
}

func (s *relationshipService) ListAssignedTestees(ctx context.Context, dto ListAssignedTesteeDTO) (*AssignedTesteeListResult, error) {
	relations, err := s.relationRepo.ListActiveByClinician(
		ctx,
		dto.OrgID,
		domainClinician.ID(dto.ClinicianID),
		domainRelation.AccessGrantRelationTypes(),
		dto.Offset,
		dto.Limit,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list relations")
	}

	totalCount, err := s.relationRepo.CountActiveByClinician(
		ctx,
		dto.OrgID,
		domainClinician.ID(dto.ClinicianID),
		domainRelation.AccessGrantRelationTypes(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count relations")
	}

	testeesByID, err := s.loadTesteesByID(ctx, extractRelationTesteeIDs(relations))
	if err != nil {
		return nil, errors.Wrap(err, "failed to batch load assigned testees")
	}

	items := make([]*AssignedTesteeResult, 0, len(relations))
	for _, item := range relations {
		testeeItem := testeesByID[item.TesteeID()]
		if testeeItem == nil {
			continue
		}
		items = append(items, toAssignedTesteeResult(testeeItem))
	}

	return &AssignedTesteeListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}

func (s *relationshipService) ListAssignedTesteeIDs(ctx context.Context, orgID int64, clinicianID uint64) ([]uint64, error) {
	ids, err := s.relationRepo.ListActiveTesteeIDsByClinician(
		ctx,
		orgID,
		domainClinician.ID(clinicianID),
		domainRelation.AccessGrantRelationTypes(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list assigned testee ids")
	}

	seen := make(map[uint64]struct{}, len(ids))
	result := make([]uint64, 0, len(ids))
	for _, id := range ids {
		rawID := id.Uint64()
		if _, ok := seen[rawID]; ok {
			continue
		}
		seen[rawID] = struct{}{}
		result = append(result, rawID)
	}
	return result, nil
}

func (s *relationshipService) ListTesteeRelations(ctx context.Context, dto ListTesteeRelationDTO) (*TesteeRelationListResult, error) {
	var (
		relations []*domainRelation.ClinicianTesteeRelation
		err       error
	)

	if dto.ActiveOnly {
		relations, err = s.relationRepo.ListActiveByTestee(ctx, dto.OrgID, domainTestee.ID(dto.TesteeID), nil)
	} else {
		relations, err = s.relationRepo.ListHistoryByTestee(ctx, dto.OrgID, domainTestee.ID(dto.TesteeID))
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to list testee relations")
	}

	items := make([]*TesteeRelationResult, 0, len(relations))
	for _, relationItem := range relations {
		clinicianItem, err := s.clinicianRepo.FindByID(ctx, relationItem.ClinicianID())
		if err != nil {
			if errors.IsCode(err, code.ErrUserNotFound) {
				continue
			}
			return nil, errors.Wrap(err, "failed to find clinician")
		}
		items = append(items, &TesteeRelationResult{
			Relation:  toRelationResult(relationItem),
			Clinician: toClinicianResult(clinicianItem),
		})
	}

	return &TesteeRelationListResult{Items: items}, nil
}

func (s *relationshipService) ListClinicianRelations(ctx context.Context, dto ListClinicianRelationDTO) (*ClinicianRelationListResult, error) {
	var (
		relations []*domainRelation.ClinicianTesteeRelation
		err       error
	)

	if dto.ActiveOnly {
		relations, err = s.relationRepo.ListActiveByClinician(
			ctx,
			dto.OrgID,
			domainClinician.ID(dto.ClinicianID),
			nil,
			dto.Offset,
			dto.Limit,
		)
	} else {
		relations, err = s.relationRepo.ListHistoryByClinician(ctx, dto.OrgID, domainClinician.ID(dto.ClinicianID))
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to list clinician relations")
	}

	totalCount := int64(len(relations))
	if dto.ActiveOnly {
		totalCount, err = s.relationRepo.CountActiveByClinician(ctx, dto.OrgID, domainClinician.ID(dto.ClinicianID), nil)
		if err != nil {
			return nil, errors.Wrap(err, "failed to count clinician relations")
		}
	}

	testeesByID, err := s.loadTesteesByID(ctx, extractRelationTesteeIDs(relations))
	if err != nil {
		return nil, errors.Wrap(err, "failed to batch load clinician relation testees")
	}

	items := make([]*ClinicianRelationResult, 0, len(relations))
	for _, relationItem := range relations {
		testeeItem := testeesByID[relationItem.TesteeID()]
		if testeeItem == nil {
			continue
		}
		items = append(items, &ClinicianRelationResult{
			Relation: toRelationResult(relationItem),
			Testee:   toAssignedTesteeResult(testeeItem),
		})
	}

	return &ClinicianRelationListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}

func normalizeAssignmentRelationType(raw string) (domainRelation.RelationType, error) {
	relationType := domainRelation.NormalizeAssignableRelationType(domainRelation.RelationType(raw))
	if !domainRelation.IsSupportedAssignmentRelationType(relationType) {
		return "", errors.WithCode(code.ErrInvalidArgument, "unsupported clinician relation type")
	}
	return relationType, nil
}

func extractRelationTesteeIDs(relations []*domainRelation.ClinicianTesteeRelation) []domainTestee.ID {
	ids := make([]domainTestee.ID, 0, len(relations))
	seen := make(map[domainTestee.ID]struct{}, len(relations))
	for _, relationItem := range relations {
		if relationItem == nil {
			continue
		}
		testeeID := relationItem.TesteeID()
		if _, ok := seen[testeeID]; ok {
			continue
		}
		seen[testeeID] = struct{}{}
		ids = append(ids, testeeID)
	}
	return ids
}

func (s *relationshipService) loadTesteesByID(ctx context.Context, ids []domainTestee.ID) (map[domainTestee.ID]*domainTestee.Testee, error) {
	if len(ids) == 0 {
		return map[domainTestee.ID]*domainTestee.Testee{}, nil
	}

	items, err := s.testeeRepo.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	result := make(map[domainTestee.ID]*domainTestee.Testee, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result[item.ID()] = item
	}
	return result, nil
}
