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
	entry, clinicianItem, err := s.resolveEntry(ctx, token)
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
		entry         *domainAssessmentEntry.AssessmentEntry
		clinicianItem *domainClinician.Clinician
		testeeItem    *domainTestee.Testee
		relationItem  *domainRelation.ClinicianTesteeRelation
		assignment    *RelationSummaryResult
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

		assignedRelation, err := s.relationRepo.FindActive(
			txCtx,
			entry.OrgID(),
			entry.ClinicianID(),
			testeeItem.ID(),
			domainRelation.RelationTypeAssigned,
		)
		if err != nil {
			if !errors.IsCode(err, code.ErrUserNotFound) {
				return errors.Wrap(err, "failed to find assigned relation")
			}

			assignedRelation = domainRelation.NewClinicianTesteeRelation(
				entry.OrgID(),
				entry.ClinicianID(),
				testeeItem.ID(),
				domainRelation.RelationTypeAssigned,
				domainRelation.SourceTypeAssessmentEntry,
				&entryID,
				true,
				time.Now(),
				nil,
			)
			if err := s.relationRepo.Save(txCtx, assignedRelation); err != nil {
				return errors.Wrap(err, "failed to save assigned relation")
			}
		}

		assignment = toRelationSummaryResult(assignedRelation)

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
