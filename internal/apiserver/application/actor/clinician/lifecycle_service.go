package clinician

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
)

type lifecycleService struct {
	repo      domainClinician.Repository
	validator domainClinician.Validator
	uow       apptransaction.Runner
}

// NewLifecycleService 创建从业者生命周期服务。
func NewLifecycleService(
	repo domainClinician.Repository,
	validator domainClinician.Validator,
	uow apptransaction.Runner,
) ClinicianLifecycleService {
	return &lifecycleService{
		repo:      repo,
		validator: validator,
		uow:       uow,
	}
}

func (s *lifecycleService) Register(ctx context.Context, dto RegisterClinicianDTO) (*ClinicianResult, error) {
	var result *domainClinician.Clinician

	err := s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.validator.ValidateForCreation(
			dto.OrgID,
			dto.Name,
			dto.Department,
			dto.Title,
			domainClinician.Type(dto.ClinicianType),
			dto.EmployeeCode,
		); err != nil {
			return err
		}

		result = domainClinician.NewClinician(
			dto.OrgID,
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
	clinicianID, err := clinicianIDFromUint64("clinician_id", dto.ClinicianID)
	if err != nil {
		return nil, err
	}

	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.repo.FindByID(txCtx, clinicianID)
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

func (s *lifecycleService) Delete(ctx context.Context, clinicianID uint64) error {
	targetClinicianID, err := clinicianIDFromUint64("clinician_id", clinicianID)
	if err != nil {
		return err
	}
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.Delete(txCtx, targetClinicianID); err != nil {
			return errors.Wrap(err, "failed to delete clinician")
		}
		return nil
	})
}

func (s *lifecycleService) setActive(ctx context.Context, clinicianID uint64, active bool) (*ClinicianResult, error) {
	var result *domainClinician.Clinician
	targetClinicianID, err := clinicianIDFromUint64("clinician_id", clinicianID)
	if err != nil {
		return nil, err
	}

	err = s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		item, err := s.repo.FindByID(txCtx, targetClinicianID)
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
