package handler

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

func assessmentModelActorContext(c *gin.Context) (assessmentModelApp.ActorContext, error) {
	principal, ok := restmiddleware.GetPrincipal(c)
	if !ok {
		return assessmentModelApp.ActorContext{}, errors.WithCode(code.ErrPermissionDenied, "authenticated actor is required")
	}
	scope, ok := restmiddleware.GetOrgScope(c)
	if !ok {
		return assessmentModelApp.ActorContext{}, errors.WithCode(code.ErrPermissionDenied, "resolved organization scope is required")
	}
	return assessmentModelApp.ActorContext{Principal: principal, Scope: scope}, nil
}
