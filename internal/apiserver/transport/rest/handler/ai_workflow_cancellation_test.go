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

func (g *managementGateway) CancelEvaluation(ctx context.Context, scope app.EvaluationScope, command app.EvaluationCancel) (app.EvaluationState, error) {
	g.cancel = command
	return g.GetEvaluation(ctx, scope)
}

func TestCancelUsesProtectedIdentityAndRejectsMissingDiscard(t *testing.T) {
	for _, field := range []string{`,"discard":false`, `,"discard":true`, ``, `,"discard":null`} {
		gateway := &managementGateway{}
		commands := &mqHTTPCommands{}
		h := NewAIWorkflowManagementHandler(&app.EvaluationAdministration{Gateway: gateway, Messages: commands})
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"command_id":"00000000-0000-4000-8000-000000000005","organization_id":999,"operator_user_id":999,"expected_version":8,"reason":"停止","confirm":true`+field+`}`))
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "run_id", Value: "00000000-0000-4000-8000-000000000001"}}
		ctx.Set(middleware.OrgIDKey, uint64(12))
		ctx.Set(middleware.UserIDKey, uint64(34))
		snapshot := &authz.Snapshot{EffectiveRoles: []string{"qs:admin"}, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}}
		ctx.Request = ctx.Request.WithContext(authz.WithSnapshot(ctx.Request.Context(), snapshot))
		h.Cancel(ctx)
		valid := strings.Contains(field, "false") || strings.Contains(field, "true")
		if valid {
			if w.Code != 202 || gateway.calls != 0 || commands.calls != 1 || commands.org != 12 || commands.user != 34 || commands.cancel.Discard == nil || *commands.cancel.Discard != strings.Contains(field, "true") {
				t.Fatal("trusted identity or explicit decision lost", w.Code)
			}
		} else if w.Code != 400 || gateway.calls != 0 || commands.calls != 0 {
			t.Fatal("missing decision reached AI", w.Code)
		}
	}
}
