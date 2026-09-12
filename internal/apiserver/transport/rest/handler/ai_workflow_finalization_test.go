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

func (g *managementGateway) FinalizeEvaluation(ctx context.Context, s app.EvaluationScope, command app.EvaluationFinalize) (app.EvaluationState, error) {
	g.finalize = command
	return g.GetEvaluation(ctx, s)
}

func TestFinalizeUsesProtectedIdentityAndRequiresExplicitFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, outcome := range []string{`,"expected_passed":false`, ``, `,"expected_passed":null`} {
		gateway := &managementGateway{}
		h := NewAIWorkflowManagementHandler(&app.EvaluationAdministration{Gateway: gateway})
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		body := `{"organization_id":999,"operator_user_id":999,"expected_version":7,"reason":"核对后拒绝","confirm":true` + outcome + `}`
		ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "run_id", Value: "00000000-0000-4000-8000-000000000001"}}
		ctx.Set(middleware.OrgIDKey, uint64(12))
		ctx.Set(middleware.UserIDKey, uint64(34))
		snapshot := &authz.Snapshot{EffectiveRoles: []string{"qs:admin"}, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}}
		ctx.Request = ctx.Request.WithContext(authz.WithSnapshot(ctx.Request.Context(), snapshot))
		h.Finalize(ctx)
		if outcome == `,"expected_passed":false` {
			if w.Code != 200 || gateway.calls != 1 || gateway.scope.OrganizationID != 12 || gateway.scope.OperatorUserID != 34 || gateway.finalize.ExpectedPassed == nil || *gateway.finalize.ExpectedPassed {
				t.Fatalf("status=%d scope=%+v", w.Code, gateway.scope)
			}
		} else if w.Code != 400 || gateway.calls != 0 {
			t.Fatal("omitted/null expected outcome accepted", w.Code)
		}
	}
}
