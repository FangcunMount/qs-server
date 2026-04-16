package assessmententry

import (
	"context"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainAssessmentEntry "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type service struct {
	repo            domainAssessmentEntry.Repository
	clinicianRepo   domainClinician.Repository
	relationRepo    domainRelation.Repository
	testeeRepo      domainTestee.Repository
	testeeFactory   domainTestee.Factory
	validator       domainAssessmentEntry.Validator
	guardianshipSvc *iam.GuardianshipService
	resolveLog      ResolveLogWriter
	intakeLog       IntakeLogWriter
	behaviorEvents  BehaviorEventStager
	uow             *mysql.UnitOfWork
}

// NewService 创建测评入口服务。
func NewService(
	repo domainAssessmentEntry.Repository,
	clinicianRepo domainClinician.Repository,
	relationRepo domainRelation.Repository,
	testeeRepo domainTestee.Repository,
	testeeFactory domainTestee.Factory,
	validator domainAssessmentEntry.Validator,
	guardianshipSvc *iam.GuardianshipService,
	resolveLog ResolveLogWriter,
	intakeLog IntakeLogWriter,
	behaviorEvents BehaviorEventStager,
	uow *mysql.UnitOfWork,
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

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		clinicianItem, err := s.clinicianRepo.FindByID(txCtx, domainClinician.ID(dto.ClinicianID))
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
			domainClinician.ID(dto.ClinicianID),
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
	item, err := s.repo.FindByID(ctx, domainAssessmentEntry.ID(entryID))
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
	items, err := s.repo.ListByClinician(
		ctx,
		dto.OrgID,
		domainClinician.ID(dto.ClinicianID),
		dto.Offset,
		dto.Limit,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list assessment entries")
	}

	totalCount, err := s.repo.CountByClinician(ctx, dto.OrgID, domainClinician.ID(dto.ClinicianID))
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
			if err := s.behaviorEvents.StageEntryOpened(txCtx, entry.OrgID(), uint64(entry.ClinicianID()), entry.ID().Uint64(), resolvedAt); err != nil {
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
	var (
		entry             *domainAssessmentEntry.AssessmentEntry
		clinicianItem     *domainClinician.Clinician
		testeeItem        *domainTestee.Testee
		relationItem      *domainRelation.ClinicianTesteeRelation
		assignment        *RelationSummaryResult
		testeeCreated     bool
		assignmentCreated bool
		intakeAt          = time.Now()
	)

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		entry, clinicianItem, err = s.resolveEntry(txCtx, token)
		if err != nil {
			return err
		}

		if dto.ProfileID != nil && *dto.ProfileID > 0 && s.guardianshipSvc != nil && s.guardianshipSvc.IsEnabled() {
			if err := s.guardianshipSvc.ValidateChildExists(txCtx, strconv.FormatUint(*dto.ProfileID, 10)); err != nil {
				return errors.WithCode(code.ErrInvalidArgument, "child profile does not exist in IAM system")
			}
		}

		if dto.ProfileID != nil && *dto.ProfileID > 0 {
			_, existingErr := s.testeeRepo.FindByProfile(txCtx, entry.OrgID(), *dto.ProfileID)
			switch {
			case existingErr == nil:
				testeeCreated = false
			case errors.IsCode(existingErr, code.ErrUserNotFound):
				testeeCreated = true
			default:
				return errors.Wrap(existingErr, "failed to check existing testee by profile")
			}

			testeeItem, err = s.testeeFactory.GetOrCreateByProfile(
				txCtx,
				entry.OrgID(),
				*dto.ProfileID,
				dto.Name,
				dto.Gender,
				dto.Birthday,
			)
			if err != nil {
				return errors.Wrap(err, "failed to get or create testee by profile")
			}
		} else {
			testeeCreated = true
			testeeItem, err = s.testeeFactory.CreateTemporary(
				txCtx,
				entry.OrgID(),
				dto.Name,
				dto.Gender,
				dto.Birthday,
				string(domainRelation.SourceTypeAssessmentEntry),
			)
			if err != nil {
				return errors.Wrap(err, "failed to create temporary testee")
			}
		}

		entryID := entry.ID().Uint64()
		relationItem, err = s.relationRepo.FindActive(
			txCtx,
			entry.OrgID(),
			entry.ClinicianID(),
			testeeItem.ID(),
			domainRelation.RelationTypeCreator,
		)
		if err != nil {
			if !errors.IsCode(err, code.ErrUserNotFound) {
				return errors.Wrap(err, "failed to find relation")
			}

			relationItem = domainRelation.NewClinicianTesteeRelation(
				entry.OrgID(),
				entry.ClinicianID(),
				testeeItem.ID(),
				domainRelation.RelationTypeCreator,
				domainRelation.SourceTypeAssessmentEntry,
				&entryID,
				true,
				time.Now(),
				nil,
			)
			if err := s.relationRepo.Save(txCtx, relationItem); err != nil {
				return errors.Wrap(err, "failed to save relation")
			}
		}

		assignedRelation, err := s.relationRepo.FindActiveByTypes(
			txCtx,
			entry.OrgID(),
			entry.ClinicianID(),
			testeeItem.ID(),
			domainRelation.AccessGrantRelationTypes(),
		)
		if err != nil {
			if !errors.IsCode(err, code.ErrUserNotFound) {
				return errors.Wrap(err, "failed to find access relation")
			}

			assignmentCreated = true
			assignedRelation = domainRelation.NewClinicianTesteeRelation(
				entry.OrgID(),
				entry.ClinicianID(),
				testeeItem.ID(),
				domainRelation.RelationTypeAttending,
				domainRelation.SourceTypeAssessmentEntry,
				&entryID,
				true,
				time.Now(),
				nil,
			)
			if err := s.relationRepo.Save(txCtx, assignedRelation); err != nil {
				return errors.Wrap(err, "failed to save attending relation")
			}
		}

		assignment = toRelationSummaryResult(assignedRelation)

		if s.behaviorEvents != nil {
			orgID := entry.OrgID()
			clinicianID := uint64(entry.ClinicianID())
			entryID := entry.ID().Uint64()
			testeeID := testeeItem.ID().Uint64()

			if err := s.behaviorEvents.StageIntakeConfirmed(txCtx, orgID, clinicianID, entryID, testeeID, intakeAt); err != nil {
				return errors.Wrap(err, "failed to stage intake confirmed behavior event")
			}
			if testeeCreated {
				if err := s.behaviorEvents.StageTesteeProfileCreated(txCtx, orgID, clinicianID, entryID, testeeID, intakeAt); err != nil {
					return errors.Wrap(err, "failed to stage testee profile created behavior event")
				}
			}
			if assignmentCreated {
				if err := s.behaviorEvents.StageCareRelationshipEstablished(txCtx, orgID, clinicianID, entryID, testeeID, intakeAt); err != nil {
					return errors.Wrap(err, "failed to stage care relationship established behavior event")
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &AssessmentEntryIntakeResult{
		Entry:      toAssessmentEntryResult(entry),
		Clinician:  toClinicianSummaryResult(clinicianItem),
		Testee:     toTesteeSummaryResult(testeeItem),
		Relation:   toRelationSummaryResult(relationItem),
		Assignment: assignment,
	}, nil
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

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.repo.FindByID(txCtx, domainAssessmentEntry.ID(entryID))
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
