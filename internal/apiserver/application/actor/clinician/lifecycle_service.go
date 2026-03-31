package clinician

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainOperator "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

type lifecycleService struct {
	repo         domainClinician.Repository
	operatorRepo domainOperator.Repository
	validator    domainClinician.Validator
	uow          *mysql.UnitOfWork
}

// NewLifecycleService 创建从业者生命周期服务。
func NewLifecycleService(
	repo domainClinician.Repository,
	operatorRepo domainOperator.Repository,
	validator domainClinician.Validator,
	uow *mysql.UnitOfWork,
) ClinicianLifecycleService {
	return &lifecycleService{
		repo:         repo,
		operatorRepo: operatorRepo,
		validator:    validator,
		uow:          uow,
	}
}

func (s *lifecycleService) Register(ctx context.Context, dto RegisterClinicianDTO) (*ClinicianResult, error) {
	var result *domainClinician.Clinician

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.validator.ValidateForCreation(
			dto.OrgID,
			dto.OperatorID,
			dto.Name,
			dto.Department,
			dto.Title,
			domainClinician.Type(dto.ClinicianType),
			dto.EmployeeCode,
		); err != nil {
			return err
		}

		if dto.OperatorID != nil {
			if _, err := s.operatorRepo.FindByID(txCtx, domainOperator.ID(*dto.OperatorID)); err != nil {
				return errors.Wrap(err, "failed to find operator")
			}

			if _, err := s.repo.FindByOperator(txCtx, dto.OrgID, *dto.OperatorID); err == nil {
				return errors.WithCode(code.ErrUserAlreadyExists, "clinician with this operator already exists")
			} else if !errors.IsCode(err, code.ErrUserNotFound) {
				return errors.Wrap(err, "failed to find clinician by operator")
			}
		}

		result = domainClinician.NewClinician(
			dto.OrgID,
			dto.OperatorID,
			dto.Name,
			dto.Department,
			dto.Title,
			domainClinician.Type(dto.ClinicianType),
			dto.EmployeeCode,
			dto.IsActive,
		)

		if err := s.repo.Save(txCtx, result); err != nil {
			return errors.Wrap(err, "failed to save clinician")
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return toClinicianResult(result), nil
}

func (s *lifecycleService) Delete(ctx context.Context, clinicianID uint64) error {
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.Delete(txCtx, domainClinician.ID(clinicianID)); err != nil {
			return errors.Wrap(err, "failed to delete clinician")
		}
		return nil
	})
}
