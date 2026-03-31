package middleware

import (
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// Capability 表示后台动作级权限能力。
type Capability string

const (
	CapabilityOrgAdmin              Capability = "org_admin"
	CapabilityManageEvaluationPlans Capability = "manage_evaluation_plans"
	CapabilityEvaluateAssessments   Capability = "evaluate_assessments"
)

func (c Capability) String() string {
	return string(c)
}

func (c Capability) roles() []operator.Role {
	switch c {
	case CapabilityOrgAdmin:
		return []operator.Role{operator.RoleQSAdmin}
	case CapabilityManageEvaluationPlans:
		return []operator.Role{operator.RoleQSAdmin, operator.RoleEvaluationPlanManager}
	case CapabilityEvaluateAssessments:
		return []operator.Role{operator.RoleQSAdmin, operator.RoleEvaluatorQS}
	default:
		return nil
	}
}

func (c Capability) roleStrings() []string {
	roles := c.roles()
	if len(roles) == 0 {
		return nil
	}

	results := make([]string, 0, len(roles))
	for _, role := range roles {
		results = append(results, role.String())
	}
	return results
}

// RequireCapabilityMiddleware 要求当前请求具备指定能力。
func RequireCapabilityMiddleware(capability Capability) gin.HandlerFunc {
	return func(c *gin.Context) {
		requiredRoles := capability.roleStrings()
		if len(requiredRoles) == 0 {
			abortPermissionDenied(c, errors.WithCode(code.ErrPermissionDenied, "capability not configured: %s", capability))
			return
		}
		if hasAnyRole(GetRoles(c), requiredRoles...) {
			c.Next()
			return
		}

		abortPermissionDenied(c, errors.WithCode(
			code.ErrPermissionDenied,
			"capability %s requires one of roles: %s",
			capability,
			strings.Join(requiredRoles, ", "),
		))
	}
}

func hasAnyRole(currentRoles []string, requiredRoles ...string) bool {
	for _, currentRole := range currentRoles {
		for _, requiredRole := range requiredRoles {
			if currentRole == requiredRole {
				return true
			}
		}
	}
	return false
}

func abortPermissionDenied(c *gin.Context, err error) {
	core.WriteResponse(c, err, nil)
	c.Abort()
}
