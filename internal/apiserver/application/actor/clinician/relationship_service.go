package clinician

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type relationshipService struct {
	relationRepo   domainRelation.Repository
	clinicianRepo  domainClinician.Repository
	testeeRepo     domainTestee.Repository
	relationReader actorreadmodel.RelationReader
	entryReader    actorreadmodel.AssessmentEntryReader
	assignmentRule domainRelation.AssignmentPolicy
	behaviorEvents BehaviorEventStager
	uow            apptransaction.Runner
}

type relationAssignmentInput struct {
	orgID        int64
	clinicianID  domainClinician.ID
	testeeID     domainTestee.ID
	relationType domainRelation.RelationType
	sourceType   domainRelation.SourceType
	sourceID     *uint64
	now          time.Time
}

func (i *relationAssignmentInput) toAssignmentRequest() domainRelation.AssignmentRequest {
	return domainRelation.AssignmentRequest{
		OrgID:        i.orgID,
		ClinicianID:  i.clinicianID,
		TesteeID:     i.testeeID,
		RelationType: i.relationType,
		SourceType:   i.sourceType,
		SourceID:     i.sourceID,
		Now:          i.now,
	}
}

// NewRelationshipService 创建从业者关系服务。
func NewRelationshipService(
	relationRepo domainRelation.Repository,
	clinicianRepo domainClinician.Repository,
	testeeRepo domainTestee.Repository,
	behaviorEvents BehaviorEventStager,
	uow apptransaction.Runner,
	readModels ...actorreadmodel.ReadModel,
) ClinicianRelationshipService {
	var readModel actorreadmodel.ReadModel
	if len(readModels) > 0 {
		readModel = readModels[0]
	}
	return &relationshipService{
		relationRepo:   relationRepo,
		clinicianRepo:  clinicianRepo,
		testeeRepo:     testeeRepo,
		relationReader: readModel,
		entryReader:    readModel,
		assignmentRule: domainRelation.NewAssignmentPolicy(),
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
	testeeID, err := testeeIDFromUint64("testee_id", dto.TesteeID)
	if err != nil {
		return nil, err
	}
	var result *domainRelation.ClinicianTesteeRelation
	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		var fromClinicianID uint64
		existingPrimary, err := s.relationRepo.FindActivePrimaryByTestee(txCtx, dto.OrgID, testeeID)
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
	input, err := s.prepareRelationAssignment(ctx, dto)
	if err != nil {
		return nil, err
	}

	activePrimary, err := s.loadActivePrimaryForAssignment(ctx, input)
	if err != nil {
		return nil, err
	}
	if activePrimary != nil && activePrimary.ClinicianID() == input.clinicianID {
		plan, err := s.assignmentPolicyForUse().PlanAssignment(input.toAssignmentRequest(), activePrimary, nil)
		if err != nil {
			return nil, err
		}
		return s.applyAssignmentPlan(ctx, plan)
	}

	activeAccessRelation, err := s.loadActiveAccessRelation(ctx, input)
	if err != nil {
		return nil, err
	}
	plan, err := s.assignmentPolicyForUse().PlanAssignment(input.toAssignmentRequest(), activePrimary, activeAccessRelation)
	if err != nil {
		return nil, err
	}
	return s.applyAssignmentPlan(ctx, plan)
}

func (s *relationshipService) UnbindRelation(ctx context.Context, relationID uint64) (*RelationResult, error) {
	var result *domainRelation.ClinicianTesteeRelation
	targetRelationID, err := relationIDFromUint64("relation_id", relationID)
	if err != nil {
		return nil, err
	}

	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.relationRepo.FindByID(txCtx, targetRelationID)
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
	clinicianID, err := clinicianIDFromUint64("clinician_id", dto.ClinicianID)
	if err != nil {
		return nil, err
	}
	if s.relationReader == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "relation reader is not configured")
	}
	rows, totalCount, err := s.relationReader.ListAssignedTestees(ctx, actorreadmodel.RelationFilter{
		OrgID:         dto.OrgID,
		ClinicianID:   clinicianID.Uint64(),
		RelationTypes: relationTypesToStrings(domainRelation.AccessGrantRelationTypes()),
		ActiveOnly:    true,
		Offset:        dto.Offset,
		Limit:         dto.Limit,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list assigned testees")
	}
	items := make([]*AssignedTesteeResult, 0, len(rows))
	for i := range rows {
		items = append(items, toAssignedTesteeResultFromRow(&rows[i]))
	}
	return &AssignedTesteeListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}

func (s *relationshipService) ListAssignedTesteeIDs(ctx context.Context, orgID int64, clinicianID uint64) ([]uint64, error) {
	targetClinicianID, err := clinicianIDFromUint64("clinician_id", clinicianID)
	if err != nil {
		return nil, err
	}
	if s.relationReader == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "relation reader is not configured")
	}
	ids, err := s.relationReader.ListActiveTesteeIDsByClinician(
		ctx,
		orgID,
		targetClinicianID.Uint64(),
		relationTypesToStrings(domainRelation.AccessGrantRelationTypes()),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list assigned testee ids")
	}

	seen := make(map[uint64]struct{}, len(ids))
	result := make([]uint64, 0, len(ids))
	for _, id := range ids {
		rawID := id
		if _, ok := seen[rawID]; ok {
			continue
		}
		seen[rawID] = struct{}{}
		result = append(result, rawID)
	}
	return result, nil
}

func (s *relationshipService) ListTesteeRelations(ctx context.Context, dto ListTesteeRelationDTO) (*TesteeRelationListResult, error) {
	testeeID, err := testeeIDFromUint64("testee_id", dto.TesteeID)
	if err != nil {
		return nil, err
	}
	if s.relationReader == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "relation reader is not configured")
	}
	rows, err := s.relationReader.ListTesteeRelations(ctx, actorreadmodel.RelationFilter{
		OrgID:      dto.OrgID,
		TesteeID:   testeeID.Uint64(),
		ActiveOnly: dto.ActiveOnly,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list testee relations")
	}
	items := make([]*TesteeRelationResult, 0, len(rows))
	for i := range rows {
		items = append(items, &TesteeRelationResult{
			Relation:  toRelationResultFromRow(&rows[i].Relation),
			Clinician: toClinicianResultFromRow(&rows[i].Clinician),
		})
	}

	return &TesteeRelationListResult{Items: items}, nil
}

func (s *relationshipService) ListClinicianRelations(ctx context.Context, dto ListClinicianRelationDTO) (*ClinicianRelationListResult, error) {
	clinicianID, err := clinicianIDFromUint64("clinician_id", dto.ClinicianID)
	if err != nil {
		return nil, err
	}
	if s.relationReader == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "relation reader is not configured")
	}
	rows, totalCount, err := s.relationReader.ListClinicianRelations(ctx, actorreadmodel.RelationFilter{
		OrgID:       dto.OrgID,
		ClinicianID: clinicianID.Uint64(),
		ActiveOnly:  dto.ActiveOnly,
		Offset:      dto.Offset,
		Limit:       dto.Limit,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list clinician relations")
	}
	items := make([]*ClinicianRelationResult, 0, len(rows))
	for i := range rows {
		items = append(items, &ClinicianRelationResult{
			Relation: toRelationResultFromRow(&rows[i].Relation),
			Testee:   toAssignedTesteeResultFromRow(&rows[i].Testee),
		})
	}

	return &ClinicianRelationListResult{
		Items:      items,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}

func (s *relationshipService) GetTesteeCareContext(ctx context.Context, orgID int64, testeeID uint64) (*TesteeCareContextResult, error) {
	relations, err := s.ListTesteeRelations(ctx, ListTesteeRelationDTO{
		OrgID:      orgID,
		TesteeID:   testeeID,
		ActiveOnly: true,
	})
	if err != nil {
		return nil, err
	}
	item := pickPreferredCareContext(relations)
	if item == nil || item.Relation == nil || item.Clinician == nil {
		return nil, nil
	}
	result := &TesteeCareContextResult{
		ClinicianName: item.Clinician.Name,
		ClinicianRole: resolveClinicianRole(item.Clinician),
		RelationType:  item.Relation.RelationType,
	}
	if item.Relation.SourceType != "" {
		result.EntrySourceType = item.Relation.SourceType
	}
	if item.Relation.SourceType == string(domainRelation.SourceTypeAssessmentEntry) && item.Relation.SourceID != nil && s.entryReader != nil {
		title, err := s.entryReader.GetAssessmentEntryTitle(ctx, *item.Relation.SourceID)
		if err == nil {
			result.EntryTitle = title
		}
	}
	return result, nil
}

func normalizeAssignmentRelationType(raw string) (domainRelation.RelationType, error) {
	relationType := domainRelation.NormalizeAssignableRelationType(domainRelation.RelationType(raw))
	if !domainRelation.IsSupportedAssignmentRelationType(relationType) {
		return "", errors.WithCode(code.ErrInvalidArgument, "unsupported clinician relation type")
	}
	return relationType, nil
}

func (s *relationshipService) prepareRelationAssignment(ctx context.Context, dto AssignTesteeDTO) (*relationAssignmentInput, error) {
	relationType, err := normalizeAssignmentRelationType(dto.RelationType)
	if err != nil {
		return nil, err
	}

	sourceType := domainRelation.SourceType(dto.SourceType)
	if sourceType == "" {
		sourceType = domainRelation.SourceTypeManual
	}

	clinicianID, err := clinicianIDFromUint64("clinician_id", dto.ClinicianID)
	if err != nil {
		return nil, err
	}
	testeeID, err := testeeIDFromUint64("testee_id", dto.TesteeID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureAssignmentActorsInOrg(ctx, dto.OrgID, clinicianID, testeeID); err != nil {
		return nil, err
	}

	return &relationAssignmentInput{
		orgID:        dto.OrgID,
		clinicianID:  clinicianID,
		testeeID:     testeeID,
		relationType: relationType,
		sourceType:   sourceType,
		sourceID:     dto.SourceID,
		now:          time.Now(),
	}, nil
}

func (s *relationshipService) ensureAssignmentActorsInOrg(
	ctx context.Context,
	orgID int64,
	clinicianID domainClinician.ID,
	testeeID domainTestee.ID,
) error {
	clinicianItem, err := s.clinicianRepo.FindByID(ctx, clinicianID)
	if err != nil {
		return errors.Wrap(err, "failed to find clinician")
	}
	if clinicianItem.OrgID() != orgID {
		return errors.WithCode(code.ErrInvalidArgument, "clinician does not belong to the requested organization")
	}

	testeeItem, err := s.testeeRepo.FindByID(ctx, testeeID)
	if err != nil {
		return errors.Wrap(err, "failed to find testee")
	}
	if testeeItem.OrgID() != orgID {
		return errors.WithCode(code.ErrInvalidArgument, "testee does not belong to the requested organization")
	}

	return nil
}

func (s *relationshipService) loadActivePrimaryForAssignment(
	ctx context.Context,
	input *relationAssignmentInput,
) (*domainRelation.ClinicianTesteeRelation, error) {
	if input.relationType != domainRelation.RelationTypePrimary {
		return nil, nil
	}

	existingPrimaryRelation, err := s.relationRepo.FindActivePrimaryByTestee(ctx, input.orgID, input.testeeID)
	if err != nil && !errors.IsCode(err, code.ErrUserNotFound) {
		return nil, errors.Wrap(err, "failed to find active primary relation")
	}
	if err != nil {
		return nil, nil
	}
	return existingPrimaryRelation, nil
}

func (s *relationshipService) loadActiveAccessRelation(
	ctx context.Context,
	input *relationAssignmentInput,
) (*domainRelation.ClinicianTesteeRelation, error) {
	existingRelation, err := s.relationRepo.FindActiveByTypes(
		ctx,
		input.orgID,
		input.clinicianID,
		input.testeeID,
		domainRelation.AccessGrantRelationTypes(),
	)
	if err != nil && !errors.IsCode(err, code.ErrUserNotFound) {
		return nil, errors.Wrap(err, "failed to find existing access relation")
	}
	if err != nil {
		return nil, nil
	}
	return existingRelation, nil
}

func (s *relationshipService) applyAssignmentPlan(ctx context.Context, plan *domainRelation.AssignmentPlan) (*domainRelation.ClinicianTesteeRelation, error) {
	if plan == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "relation assignment plan is nil")
	}
	if plan.ReuseRelation != nil {
		return plan.ReuseRelation, nil
	}
	for _, item := range plan.Unbind {
		if item == nil {
			continue
		}
		if err := s.relationRepo.Update(ctx, item); err != nil {
			return nil, errors.Wrap(err, assignmentUnbindErrorMessage(item))
		}
	}
	if plan.Create == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "relation assignment plan missing create relation")
	}
	if err := s.relationRepo.Save(ctx, plan.Create); err != nil {
		return nil, errors.Wrap(err, "failed to save relation")
	}
	return plan.Create, nil
}

func assignmentUnbindErrorMessage(item *domainRelation.ClinicianTesteeRelation) string {
	if item.RelationType() == domainRelation.RelationTypePrimary {
		return "failed to unbind existing primary relation"
	}
	return "failed to replace existing access relation"
}

func (s *relationshipService) assignmentPolicyForUse() domainRelation.AssignmentPolicy {
	if s.assignmentRule != nil {
		return s.assignmentRule
	}
	return domainRelation.NewAssignmentPolicy()
}

func pickPreferredCareContext(result *TesteeRelationListResult) *TesteeRelationResult {
	if result == nil || len(result.Items) == 0 {
		return nil
	}
	var selected *TesteeRelationResult
	bestPriority := 1 << 30
	for _, item := range result.Items {
		if item == nil || item.Relation == nil || item.Clinician == nil {
			continue
		}
		priority := relationTypePriority(item.Relation.RelationType)
		if priority < bestPriority {
			selected = item
			bestPriority = priority
		}
	}
	return selected
}

func relationTypePriority(raw string) int {
	switch domainRelation.RelationType(raw) {
	case domainRelation.RelationTypePrimary:
		return 0
	case domainRelation.RelationTypeAttending:
		return 1
	case domainRelation.RelationTypeCollaborator:
		return 2
	case domainRelation.RelationTypeAssigned:
		return 3
	case domainRelation.RelationTypeCreator:
		return 4
	default:
		return 100
	}
}

func resolveClinicianRole(item *ClinicianResult) string {
	if item == nil {
		return ""
	}
	if item.Title != "" {
		return item.Title
	}
	return item.ClinicianType
}
