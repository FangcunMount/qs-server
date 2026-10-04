package rest

import (
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

func TestMQWorkflowCancelRouteRequiresOrgAdminAndGovernanceDependency(t *testing.T) {
	commandID := "00000000-0000-4000-8000-000000000002"
	for _, admin := range []bool{false, true} {
		for _, state := range []string{"disabled", "missing_messages", "enabled"} {
			gateway := &candidateRouteGateway{}
			messages := &mqRouteSubmitter{}
			deps := Deps{}
			if state != "disabled" {
				deps.Interpretation.AIWorkflowManagement = &app.EvaluationAdministration{Gateway: gateway}
				if state == "enabled" {
					deps.Interpretation.AIWorkflowManagement.Messages = messages
				}
			}
			router := newRouterWithBudgets(deps)
			engine := gin.New()
			engine.Use(aiRouteSnapshotMiddleware(admin))
			router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
			request := httptest.NewRequest("POST", "/internal/v2/interpretation/ai-workflow/evaluations/00000000-0000-4000-8000-000000000001/cancel?organization_id=99&operator_user_id=99", strings.NewReader(`{"command_id":"`+commandID+`","expected_version":8,"reason":"停止","confirm":true,"discard":false,"organization_id":99,"operator_user_id":99}`))
			request.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, request)
			want := 404
			if state != "disabled" {
				want = 403
				if admin {
					want = 501
					if state == "enabled" {
						want = 202
					}
				}
			}
			if w.Code != want || gateway.writes != 0 || (messages.calls == 1) != (state == "enabled" && admin) {
				t.Fatal("unexpected cancellation route access", state, admin, w.Code, gateway.writes, messages.calls)
			}
			if messages.calls == 1 {
				if messages.evaluationScope.OrganizationID != 12 || messages.evaluationScope.OperatorUserID != 34 || messages.evaluationScope.RunID != "00000000-0000-4000-8000-000000000001" || messages.commandID != commandID || messages.cancel.ExpectedVersion != 8 || messages.cancel.Reason != "停止" || !messages.cancel.Confirm || messages.cancel.Discard == nil || *messages.cancel.Discard {
					t.Fatal("untrusted cancellation scope or intent", messages)
				}
				assertMQRouteSubmitted(t, w, commandID)
			}
		}
	}
}
