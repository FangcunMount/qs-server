package clinician

import (
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Validator 从业者验证器。
type Validator interface {
	ValidateOrgID(orgID int64) error
	ValidateOperatorID(operatorID *uint64) error
	ValidateName(name string) error
	ValidateDepartment(department string) error
	ValidateTitle(title string) error
	ValidateEmployeeCode(employeeCode string) error
	ValidateType(clinicianType Type) error
	ValidateForCreation(
		orgID int64,
		operatorID *uint64,
		name, department, title string,
		clinicianType Type,
		employeeCode string,
	) error
}

type validator struct{}

// NewValidator 创建从业者验证器。
func NewValidator() Validator {
	return &validator{}
}

func (v *validator) ValidateOrgID(orgID int64) error {
	if orgID <= 0 {
		return errors.WithCode(code.ErrValidation, "orgID must be positive")
	}
	return nil
}

func (v *validator) ValidateOperatorID(operatorID *uint64) error {
	if operatorID != nil && *operatorID == 0 {
		return errors.WithCode(code.ErrValidation, "operatorID must be positive")
	}
	return nil
}

func (v *validator) ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.WithCode(code.ErrValidation, "name cannot be empty")
	}
	if len(name) > 100 {
		return errors.WithCode(code.ErrValidation, "name too long (max 100 characters)")
	}
	return nil
}

func (v *validator) ValidateDepartment(department string) error {
	if len(strings.TrimSpace(department)) > 100 {
		return errors.WithCode(code.ErrValidation, "department too long (max 100 characters)")
	}
	return nil
}

func (v *validator) ValidateTitle(title string) error {
	if len(strings.TrimSpace(title)) > 100 {
		return errors.WithCode(code.ErrValidation, "title too long (max 100 characters)")
	}
	return nil
}

func (v *validator) ValidateEmployeeCode(employeeCode string) error {
	if len(strings.TrimSpace(employeeCode)) > 50 {
		return errors.WithCode(code.ErrValidation, "employeeCode too long (max 50 characters)")
	}
	return nil
}

func (v *validator) ValidateType(clinicianType Type) error {
	switch clinicianType {
	case TypeDoctor, TypeCounselor, TypeTherapist, TypeOther:
		return nil
	default:
		return errors.WithCode(code.ErrValidation, "invalid clinician type")
	}
}

func (v *validator) ValidateForCreation(
	orgID int64,
	operatorID *uint64,
	name, department, title string,
	clinicianType Type,
	employeeCode string,
) error {
	if err := v.ValidateOrgID(orgID); err != nil {
		return err
	}
	if err := v.ValidateOperatorID(operatorID); err != nil {
		return err
	}
	if err := v.ValidateName(name); err != nil {
		return err
	}
	if err := v.ValidateDepartment(department); err != nil {
		return err
	}
	if err := v.ValidateTitle(title); err != nil {
		return err
	}
	if err := v.ValidateType(clinicianType); err != nil {
		return err
	}
	if err := v.ValidateEmployeeCode(employeeCode); err != nil {
		return err
	}
	return nil
}
