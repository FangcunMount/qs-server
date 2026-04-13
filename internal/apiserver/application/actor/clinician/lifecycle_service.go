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

func (s *lifecycleService) Update(ctx context.Context, dto UpdateClinicianDTO) (*ClinicianResult, error) {
	var result *domainClinician.Clinician

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.repo.FindByID(txCtx, domainClinician.ID(dto.ClinicianID))
		if err != nil {
			return errors.Wrap(err, "failed to find clinician")
		}

		if err := s.validator.ValidateName(dto.Name); err != nil {
			return err
		}
		if err := s.validator.ValidateDepartment(dto.Department); err != nil {
			return err
		}
		if err := s.validator.ValidateTitle(dto.Title); err != nil {
			return err
		}
		if err := s.validator.ValidateType(domainClinician.Type(dto.ClinicianType)); err != nil {
			return err
		}
		if err := s.validator.ValidateEmployeeCode(dto.EmployeeCode); err != nil {
			return err
		}

		item.UpdateProfile(
			dto.Name,
			dto.Department,
			dto.Title,
			domainClinician.Type(dto.ClinicianType),
			dto.EmployeeCode,
		)
		if err := s.repo.Update(txCtx, item); err != nil {
			return errors.Wrap(err, "failed to update clinician")
		}
		result = item
		return nil
	})
	if err != nil {
		return nil, err
	}

	return toClinicianResult(result), nil
}

func (s *lifecycleService) Activate(ctx context.Context, clinicianID uint64) (*ClinicianResult, error) {
	return s.setActive(ctx, clinicianID, true)
}

func (s *lifecycleService) Deactivate(ctx context.Context, clinicianID uint64) (*ClinicianResult, error) {
	return s.setActive(ctx, clinicianID, false)
}

func (s *lifecycleService) BindOperator(ctx context.Context, dto BindClinicianOperatorDTO) (*ClinicianResult, error) {
	var result *domainClinician.Clinician

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.repo.FindByID(txCtx, domainClinician.ID(dto.ClinicianID))
		if err != nil {
			return errors.Wrap(err, "failed to find clinician")
		}
		operatorItem, err := s.operatorRepo.FindByID(txCtx, domainOperator.ID(dto.OperatorID))
		if err != nil {
			return errors.Wrap(err, "failed to find operator")
		}
		if operatorItem.OrgID() != item.OrgID() {
			return errors.WithCode(code.ErrInvalidArgument, "operator does not belong to clinician organization")
		}

		existing, err := s.repo.FindByOperator(txCtx, item.OrgID(), dto.OperatorID)
		if err == nil && existing.ID() != item.ID() && existing.IsActive() {
			return errors.WithCode(code.ErrUserAlreadyExists, "operator already bound to another active clinician")
		}
		if err != nil && !errors.IsCode(err, code.ErrUserNotFound) {
			return errors.Wrap(err, "failed to validate clinician operator binding")
		}

		item.BindOperator(dto.OperatorID)
		if err := s.repo.Update(txCtx, item); err != nil {
			return errors.Wrap(err, "failed to bind operator")
		}
		result = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return toClinicianResult(result), nil
}

func (s *lifecycleService) UnbindOperator(ctx context.Context, clinicianID uint64) (*ClinicianResult, error) {
	var result *domainClinician.Clinician

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.repo.FindByID(txCtx, domainClinician.ID(clinicianID))
		if err != nil {
			return errors.Wrap(err, "failed to find clinician")
		}
		item.UnbindOperator()
		if err := s.repo.Update(txCtx, item); err != nil {
			return errors.Wrap(err, "failed to unbind operator")
		}
		result = item
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

func (s *lifecycleService) setActive(ctx context.Context, clinicianID uint64, active bool) (*ClinicianResult, error) {
	var result *domainClinician.Clinician

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.repo.FindByID(txCtx, domainClinician.ID(clinicianID))
		if err != nil {
			return errors.Wrap(err, "failed to find clinician")
		}
		if active {
			item.Activate()
		} else {
			item.Deactivate()
		}
		if err := s.repo.Update(txCtx, item); err != nil {
			return errors.Wrap(err, "failed to update clinician status")
		}
		result = item
		return nil
	})
	if err != nil {
		return nil, err
	}

	return toClinicianResult(result), nil
}
