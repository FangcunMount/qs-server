package assessmententry

import (
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Validator 测评入口验证器。
type Validator interface {
	ValidateOrgID(orgID int64) error
	ValidateClinicianID(clinicianID uint64) error
	ValidateToken(token string) error
	ValidateTargetType(targetType TargetType) error
	ValidateTargetCode(targetCode string) error
	ValidateTargetVersion(targetVersion string) error
	ValidateForCreation(
		orgID int64,
		clinicianID uint64,
		token string,
		targetType TargetType,
		targetCode, targetVersion string,
	) error
}

type validator struct{}

// NewValidator 创建测评入口验证器。
func NewValidator() Validator {
	return &validator{}
}

func (v *validator) ValidateOrgID(orgID int64) error {
	if orgID <= 0 {
		return errors.WithCode(code.ErrValidation, "orgID must be positive")
	}
	return nil
}

func (v *validator) ValidateClinicianID(clinicianID uint64) error {
	if clinicianID == 0 {
		return errors.WithCode(code.ErrValidation, "clinicianID must be positive")
	}
	return nil
}

func (v *validator) ValidateToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.WithCode(code.ErrValidation, "token cannot be empty")
	}
	if len(token) > 32 {
		return errors.WithCode(code.ErrValidation, "token too long (max 32 characters)")
	}
	return nil
}

func (v *validator) ValidateTargetType(targetType TargetType) error {
	switch targetType {
	case TargetTypeQuestionnaire, TargetTypeScale:
		return nil
	default:
		return errors.WithCode(code.ErrValidation, "invalid target type")
	}
}

func (v *validator) ValidateTargetCode(targetCode string) error {
	targetCode = strings.TrimSpace(targetCode)
	if targetCode == "" {
		return errors.WithCode(code.ErrValidation, "targetCode cannot be empty")
	}
	if len(targetCode) > 100 {
		return errors.WithCode(code.ErrValidation, "targetCode too long (max 100 characters)")
	}
	return nil
}

func (v *validator) ValidateTargetVersion(targetVersion string) error {
	if len(strings.TrimSpace(targetVersion)) > 50 {
		return errors.WithCode(code.ErrValidation, "targetVersion too long (max 50 characters)")
	}
	return nil
}

func (v *validator) ValidateForCreation(
	orgID int64,
	clinicianID uint64,
	token string,
	targetType TargetType,
	targetCode, targetVersion string,
) error {
	if err := v.ValidateOrgID(orgID); err != nil {
		return err
	}
	if err := v.ValidateClinicianID(clinicianID); err != nil {
		return err
	}
	if err := v.ValidateToken(token); err != nil {
		return err
	}
	if err := v.ValidateTargetType(targetType); err != nil {
		return err
	}
	if err := v.ValidateTargetCode(targetCode); err != nil {
		return err
	}
	if err := v.ValidateTargetVersion(targetVersion); err != nil {
		return err
	}
	return nil
}
