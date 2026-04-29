package assessmententry

import (
	"context"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainAssessmentEntry "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type service struct {
	repo            domainAssessmentEntry.Repository
	clinicianRepo   domainClinician.Repository
	relationRepo    domainRelation.Repository
	testeeRepo      domainTestee.Repository
	testeeFactory   domainTestee.Factory
	validator       domainAssessmentEntry.Validator
	guardianshipSvc iambridge.GuardianshipReader
	resolveLog      ResolveLogWriter
	intakeLog       IntakeLogWriter
	behaviorEvents  BehaviorEventStager
	uow             apptransaction.Runner
}

type intakeState struct {
	entry             *domainAssessmentEntry.AssessmentEntry
	clinician         *domainClinician.Clinician
	testee            *domainTestee.Testee
	relation          *domainRelation.ClinicianTesteeRelation
	assignment        *RelationSummaryResult
	testeeCreated     bool
	assignmentCreated bool
	intakeAt          time.Time
}

// NewService 创建测评入口服务。
func NewService(
	repo domainAssessmentEntry.Repository,
	clinicianRepo domainClinician.Repository,
	relationRepo domainRelation.Repository,
	testeeRepo domainTestee.Repository,
	testeeFactory domainTestee.Factory,
	validator domainAssessmentEntry.Validator,
	guardianshipSvc iambridge.GuardianshipReader,
	resolveLog ResolveLogWriter,
	intakeLog IntakeLogWriter,
	behaviorEvents BehaviorEventStager,
	uow apptransaction.Runner,
) AssessmentEntryService {
	return &service{
		repo:            repo,
		clinicianRepo:   clinicianRepo,
		relationRepo:    relationRepo,
		testeeRepo:      testeeRepo,
		testeeFactory:   testeeFactory,
		validator:       validator,
		guardianshipSvc: guardianshipSvc,
		resolveLog:      resolveLog,
		intakeLog:       intakeLog,
		behaviorEvents:  behaviorEvents,
		uow:             uow,
	}
}

func (s *service) Create(ctx context.Context, dto CreateAssessmentEntryDTO) (*AssessmentEntryResult, error) {
	var result *domainAssessmentEntry.AssessmentEntry
	clinicianID, err := clinicianIDFromUint64("clinician_id", dto.ClinicianID)
	if err != nil {
		return nil, err
	}

	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		clinicianItem, err := s.clinicianRepo.FindByID(txCtx, clinicianID)
		if err != nil {
			return errors.Wrap(err, "failed to find clinician")
		}
		if clinicianItem.OrgID() != dto.OrgID {
			return errors.WithCode(code.ErrInvalidArgument, "clinician does not belong to the requested organization")
		}
		if !clinicianItem.IsActive() {
			return errors.WithCode(code.ErrPermissionDenied, "clinician is inactive")
		}

		tokenCode, err := meta.GenerateCodeWithPrefix("ae_")
		if err != nil {
			return errors.WithCode(code.ErrTokenGeneration, "failed to generate assessment entry token")
		}

		if err := s.validator.ValidateForCreation(
			dto.OrgID,
			dto.ClinicianID,
			tokenCode.String(),
			domainAssessmentEntry.TargetType(dto.TargetType),
			dto.TargetCode,
			dto.TargetVersion,
		); err != nil {
			return err
		}

		result = domainAssessmentEntry.NewAssessmentEntry(
			dto.OrgID,
			clinicianID,
			tokenCode.String(),
			domainAssessmentEntry.TargetType(dto.TargetType),
			dto.TargetCode,
			dto.TargetVersion,
			true,
			dto.ExpiresAt,
		)

		if err := s.repo.Save(txCtx, result); err != nil {
			return errors.Wrap(err, "failed to save assessment entry")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return toAssessmentEntryResult(result), nil
}

func (s *service) GetByID(ctx context.Context, entryID uint64) (*AssessmentEntryResult, error) {
	targetEntryID, err := assessmentEntryIDFromUint64("entry_id", entryID)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.FindByID(ctx, targetEntryID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find assessment entry")
	}
	return toAssessmentEntryResult(item), nil
}

func (s *service) Deactivate(ctx context.Context, entryID uint64) (*AssessmentEntryResult, error) {
	return s.setActive(ctx, entryID, false)
}

func (s *service) Reactivate(ctx context.Context, entryID uint64) (*AssessmentEntryResult, error) {
	return s.setActive(ctx, entryID, true)
}

func (s *service) ListByClinician(ctx context.Context, dto ListAssessmentEntryDTO) (*AssessmentEntryListResult, error) {
	clinicianID, err := clinicianIDFromUint64("clinician_id", dto.ClinicianID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListByClinician(
		ctx,
		dto.OrgID,
		clinicianID,
		dto.Offset,
		dto.Limit,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list assessment entries")
	}

	totalCount, err := s.repo.CountByClinician(ctx, dto.OrgID, clinicianID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to count assessment entries")
	}

	results := make([]*AssessmentEntryResult, 0, len(items))
	for _, item := range items {
		results = append(results, toAssessmentEntryResult(item))
	}

	return &AssessmentEntryListResult{
		Items:      results,
		TotalCount: totalCount,
		Offset:     dto.Offset,
		Limit:      dto.Limit,
	}, nil
}

func (s *service) Resolve(ctx context.Context, token string) (*ResolvedAssessmentEntryResult, error) {
	var (
		entry         *domainAssessmentEntry.AssessmentEntry
		clinicianItem *domainClinician.Clinician
	)
	resolvedAt := time.Now()
	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		entry, clinicianItem, err = s.resolveEntry(txCtx, token)
		if err != nil {
			return err
		}
		if s.behaviorEvents != nil {
			if err := s.behaviorEvents.StageEntryOpened(txCtx, entry.OrgID(), entry.ClinicianID().Uint64(), entry.ID().Uint64(), resolvedAt); err != nil {
				return errors.Wrap(err, "failed to stage assessment entry opened behavior event")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ResolvedAssessmentEntryResult{
		Entry:     toAssessmentEntryResult(entry),
		Clinician: toClinicianSummaryResult(clinicianItem),
	}, nil
}

func (s *service) Intake(ctx context.Context, token string, dto IntakeByAssessmentEntryDTO) (*AssessmentEntryIntakeResult, error) {
	state := &intakeState{intakeAt: time.Now()}

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		state.entry, state.clinician, err = s.resolveEntry(txCtx, token)
		if err != nil {
			return err
		}

		if err := s.validateIntakeProfile(txCtx, dto); err != nil {
			return err
		}

		state.testee, state.testeeCreated, err = s.resolveIntakeTestee(txCtx, state.entry, dto)
		if err != nil {
			return err
		}

		state.relation, err = s.ensureCreatorRelation(txCtx, state.entry, state.testee)
		if err != nil {
			return err
		}

		assignedRelation, assignmentCreated, err := s.ensureAssignmentRelation(txCtx, state.entry, state.testee)
		if err != nil {
			return err
		}
		state.assignmentCreated = assignmentCreated
		state.assignment = toRelationSummaryResult(assignedRelation)
		return s.stageIntakeBehaviorEvents(txCtx, state)
	})
	if err != nil {
		return nil, err
	}

	return &AssessmentEntryIntakeResult{
		Entry:      toAssessmentEntryResult(state.entry),
		Clinician:  toClinicianSummaryResult(state.clinician),
		Testee:     toTesteeSummaryResult(state.testee),
		Relation:   toRelationSummaryResult(state.relation),
		Assignment: state.assignment,
	}, nil
}

func (s *service) validateIntakeProfile(ctx context.Context, dto IntakeByAssessmentEntryDTO) error {
	if dto.ProfileID == nil || *dto.ProfileID == 0 || s.guardianshipSvc == nil || !s.guardianshipSvc.IsEnabled() {
		return nil
	}
	if err := s.guardianshipSvc.ValidateChildExists(ctx, strconv.FormatUint(*dto.ProfileID, 10)); err != nil {
		return errors.WithCode(code.ErrInvalidArgument, "child profile does not exist in IAM system")
	}
	return nil
}

func (s *service) resolveIntakeTestee(
	ctx context.Context,
	entry *domainAssessmentEntry.AssessmentEntry,
	dto IntakeByAssessmentEntryDTO,
) (*domainTestee.Testee, bool, error) {
	if dto.ProfileID == nil || *dto.ProfileID == 0 {
		testeeItem, err := s.testeeFactory.CreateTemporary(
			ctx,
			entry.OrgID(),
			dto.Name,
			dto.Gender,
			dto.Birthday,
			string(domainRelation.SourceTypeAssessmentEntry),
		)
		if err != nil {
			return nil, false, errors.Wrap(err, "failed to create temporary testee")
		}
		return testeeItem, true, nil
	}

	testeeCreated, err := s.isNewProfileTestee(ctx, entry.OrgID(), *dto.ProfileID)
	if err != nil {
		return nil, false, err
	}

	testeeItem, err := s.testeeFactory.GetOrCreateByProfile(
		ctx,
		entry.OrgID(),
		*dto.ProfileID,
		dto.Name,
		dto.Gender,
		dto.Birthday,
	)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to get or create testee by profile")
	}
	return testeeItem, testeeCreated, nil
}

func (s *service) isNewProfileTestee(ctx context.Context, orgID int64, profileID uint64) (bool, error) {
	_, err := s.testeeRepo.FindByProfile(ctx, orgID, profileID)
	switch {
	case err == nil:
		return false, nil
	case errors.IsCode(err, code.ErrUserNotFound):
		return true, nil
	default:
		return false, errors.Wrap(err, "failed to check existing testee by profile")
	}
}

func (s *service) ensureCreatorRelation(
	ctx context.Context,
	entry *domainAssessmentEntry.AssessmentEntry,
	testeeItem *domainTestee.Testee,
) (*domainRelation.ClinicianTesteeRelation, error) {
	relationItem, err := s.relationRepo.FindActive(
		ctx,
		entry.OrgID(),
		entry.ClinicianID(),
		testeeItem.ID(),
		domainRelation.RelationTypeCreator,
	)
	if err == nil {
		return relationItem, nil
	}
	if !errors.IsCode(err, code.ErrUserNotFound) {
		return nil, errors.Wrap(err, "failed to find relation")
	}

	relationItem = newAssessmentEntryRelation(entry, testeeItem, domainRelation.RelationTypeCreator)
	if err := s.relationRepo.Save(ctx, relationItem); err != nil {
		return nil, errors.Wrap(err, "failed to save relation")
	}
	return relationItem, nil
}

func (s *service) ensureAssignmentRelation(
	ctx context.Context,
	entry *domainAssessmentEntry.AssessmentEntry,
	testeeItem *domainTestee.Testee,
) (*domainRelation.ClinicianTesteeRelation, bool, error) {
	relationItem, err := s.relationRepo.FindActiveByTypes(
		ctx,
		entry.OrgID(),
		entry.ClinicianID(),
		testeeItem.ID(),
		domainRelation.AccessGrantRelationTypes(),
	)
	if err == nil {
		return relationItem, false, nil
	}
	if !errors.IsCode(err, code.ErrUserNotFound) {
		return nil, false, errors.Wrap(err, "failed to find access relation")
	}

	relationItem = newAssessmentEntryRelation(entry, testeeItem, domainRelation.RelationTypeAttending)
	if err := s.relationRepo.Save(ctx, relationItem); err != nil {
		return nil, false, errors.Wrap(err, "failed to save attending relation")
	}
	return relationItem, true, nil
}

func newAssessmentEntryRelation(
	entry *domainAssessmentEntry.AssessmentEntry,
	testeeItem *domainTestee.Testee,
	relationType domainRelation.RelationType,
) *domainRelation.ClinicianTesteeRelation {
	entryID := entry.ID().Uint64()
	return domainRelation.NewClinicianTesteeRelation(
		entry.OrgID(),
		entry.ClinicianID(),
		testeeItem.ID(),
		relationType,
		domainRelation.SourceTypeAssessmentEntry,
		&entryID,
		true,
		time.Now(),
		nil,
	)
}

func (s *service) stageIntakeBehaviorEvents(ctx context.Context, state *intakeState) error {
	if s.behaviorEvents == nil {
		return nil
	}

	orgID := state.entry.OrgID()
	clinicianID := state.entry.ClinicianID().Uint64()
	entryID := state.entry.ID().Uint64()
	testeeID := state.testee.ID().Uint64()

	if err := s.behaviorEvents.StageIntakeConfirmed(ctx, orgID, clinicianID, entryID, testeeID, state.intakeAt); err != nil {
		return errors.Wrap(err, "failed to stage intake confirmed behavior event")
	}
	if state.testeeCreated {
		if err := s.behaviorEvents.StageTesteeProfileCreated(ctx, orgID, clinicianID, entryID, testeeID, state.intakeAt); err != nil {
			return errors.Wrap(err, "failed to stage testee profile created behavior event")
		}
	}
	if state.assignmentCreated {
		if err := s.behaviorEvents.StageCareRelationshipEstablished(ctx, orgID, clinicianID, entryID, testeeID, state.intakeAt); err != nil {
			return errors.Wrap(err, "failed to stage care relationship established behavior event")
		}
	}
	return nil
}

func (s *service) resolveEntry(
	ctx context.Context,
	token string,
) (*domainAssessmentEntry.AssessmentEntry, *domainClinician.Clinician, error) {
	entry, err := s.repo.FindByToken(ctx, token)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to find assessment entry by token")
	}
	if !entry.CanResolve(time.Now()) {
		return nil, nil, errors.WithCode(code.ErrInvalidArgument, "assessment entry is inactive or expired")
	}

	clinicianItem, err := s.clinicianRepo.FindByID(ctx, entry.ClinicianID())
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to find clinician")
	}
	if clinicianItem.OrgID() != entry.OrgID() {
		return nil, nil, errors.WithCode(code.ErrPermissionDenied, "clinician does not belong to assessment entry organization")
	}
	if !clinicianItem.IsActive() {
		return nil, nil, errors.WithCode(code.ErrPermissionDenied, "clinician is inactive")
	}

	return entry, clinicianItem, nil
}

func (s *service) setActive(ctx context.Context, entryID uint64, active bool) (*AssessmentEntryResult, error) {
	var result *domainAssessmentEntry.AssessmentEntry
	targetEntryID, err := assessmentEntryIDFromUint64("entry_id", entryID)
	if err != nil {
		return nil, err
	}

	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.repo.FindByID(txCtx, targetEntryID)
		if err != nil {
			return errors.Wrap(err, "failed to find assessment entry")
		}
		if active {
			item.Reactivate()
		} else {
			item.Deactivate()
		}
		if err := s.repo.Update(txCtx, item); err != nil {
			return errors.Wrap(err, "failed to update assessment entry")
		}
		result = item
		return nil
	})
	if err != nil {
		return nil, err
	}

	return toAssessmentEntryResult(result), nil
}
