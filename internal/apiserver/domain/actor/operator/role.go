package operator

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// IsSupportedRole 判断角色是否为当前 QS 支持的角色。
func IsSupportedRole(role Role) bool {
	switch role {
	case RoleQSAdmin, RoleContentManager, RoleEvaluatorQS, RoleOperator,
		RoleEvaluationPlanManager:
		return true
	default:
		return false
	}
}

func invalidRoleError() error {
	return errors.WithCode(code.ErrValidation, "invalid role")
}
