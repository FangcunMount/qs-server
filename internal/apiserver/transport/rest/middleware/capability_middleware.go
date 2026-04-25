package middleware

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

// Capability 与 application/authz 对齐，供路由引用。
type Capability = authzapp.Capability

const (
	CapabilityOrgAdmin              = authzapp.CapabilityOrgAdmin
	CapabilityReadQuestionnaires    = authzapp.CapabilityReadQuestionnaires
	CapabilityManageQuestionnaires  = authzapp.CapabilityManageQuestionnaires
	CapabilityReadScales            = authzapp.CapabilityReadScales
	CapabilityManageScales          = authzapp.CapabilityManageScales
	CapabilityReadAnswersheets      = authzapp.CapabilityReadAnswersheets
	CapabilityManageEvaluationPlans = authzapp.CapabilityManageEvaluationPlans
	CapabilityEvaluateAssessments   = authzapp.CapabilityEvaluateAssessments
)

// RequireCapabilityMiddleware 要求当前请求具备指定能力（基于 IAM 授权快照的 resource/action，不信任 JWT roles）。
func RequireCapabilityMiddleware(capability Capability) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := GetAuthzSnapshot(c)
		if snap == nil {
			abortPermissionDenied(c, errors.WithCode(code.ErrPermissionDenied, "authorization snapshot required"))
			return
		}
		if decision := authzapp.DecideCapability(snap, capability); !decision.Allowed {
			abortPermissionDenied(c, errors.WithCode(
				code.ErrPermissionDenied,
				"capability %s denied by IAM authorization",
				capability,
			))
			return
		}
		c.Next()
	}
}

// RequireAnyCapabilityMiddleware 要求当前请求具备任一能力。
func RequireAnyCapabilityMiddleware(capabilities ...Capability) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := GetAuthzSnapshot(c)
		if snap == nil {
			abortPermissionDenied(c, errors.WithCode(code.ErrPermissionDenied, "authorization snapshot required"))
			return
		}
		if decision := authzapp.DecideAnyCapability(snap, capabilities...); decision.Allowed {
			c.Next()
			return
		}
		abortPermissionDenied(c, errors.WithCode(
			code.ErrPermissionDenied,
			"capabilities %v denied by IAM authorization",
			capabilities,
		))
	}
}

func abortPermissionDenied(c *gin.Context, err error) {
	core.WriteResponse(c, err, nil)
	c.Abort()
}
