package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	middleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func (g *managementGateway) ReopenEvaluationReview(ctx context.Context, s app.EvaluationScope, command app.EvaluationReopen) (app.EvaluationState, error) {
	g.reopen = command
	return g.GetEvaluation(ctx, s)
}

func TestReopenUsesProtectedIdentityInsteadOfBody(t *testing.T) {
	gateway := &managementGateway{}
	h := NewAIWorkflowManagementHandler(&app.EvaluationAdministration{Gateway: gateway})
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"organization_id":999,"operator_user_id":999,"expected_version":8,"reason":"复核","confirm":true}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "run_id", Value: "00000000-0000-4000-8000-000000000001"}}
	ctx.Set(middleware.OrgIDKey, uint64(12))
	ctx.Set(middleware.UserIDKey, uint64(34))
	snapshot := &authz.Snapshot{EffectiveRoles: []string{"qs:admin"}, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}}
	ctx.Request = ctx.Request.WithContext(authz.WithSnapshot(ctx.Request.Context(), snapshot))
	h.ReopenReview(ctx)
	if w.Code != 200 || gateway.calls != 1 || gateway.scope.OrganizationID != 12 || gateway.scope.OperatorUserID != 34 || gateway.reopen.ExpectedVersion != 8 {
		t.Fatalf("status %d scope %+v", w.Code, gateway.scope)
	}
}
