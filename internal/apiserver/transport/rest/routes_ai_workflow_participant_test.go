package rest

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

type participantRouteGateway struct {
	app.ParticipantManagementGateway
	calls int
	scope app.DraftScope
	query app.ParticipantCapacityQuery
}

func (g *participantRouteGateway) GetParticipantCapacity(_ context.Context, scope app.DraftScope, query app.ParticipantCapacityQuery) (app.ParticipantCapacity, error) {
	g.calls++
	g.scope, g.query = scope, query
	return app.ParticipantCapacity{OrganizationID: scope.OrganizationID}, nil
}
func TestParticipantCapacityRouteProtectsScopeAndFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, query   string
		admin         bool
		status, calls int
	}{
		{"admin", "subject_id=user%3A42&assessment_id=42&organization_id=99&operator_user_id=99", true, 200, 1},
		{"revoked", "subject_id=user%3A42&assessment_id=42", false, 403, 0},
		{"duplicate", "subject_id=one&subject_id=two", true, 400, 0},
		{"zero", "assessment_id=0", true, 400, 0},
		{"control", "subject_id=one%0Atwo", true, 400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &participantRouteGateway{}
			engine := gin.New()
			engine.Use(aiRouteSnapshotMiddleware(tc.admin))
			router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowParticipants: &app.ParticipantAdministration{Gateway: gateway}}})
			router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest("GET", "/internal/v2/interpretation/ai-workflow/participant-capacity?"+tc.query, nil))
			if w.Code != tc.status || gateway.calls != tc.calls {
				t.Fatal(w.Code, w.Body.String(), gateway)
			}
			if tc.calls > 0 && (gateway.scope.OrganizationID != 12 || gateway.scope.OperatorUserID != 34 || gateway.query.SubjectID != "user:42" || gateway.query.AssessmentID != "42") {
				t.Fatal(gateway)
			}
		})
	}
}

func (g *participantRouteGateway) GetParticipantExecution(_ context.Context, scope app.DraftScope, sessionID string) (app.ParticipantExecution, error) {
	g.calls++
	g.scope = scope
	return app.ParticipantExecution{OrganizationID: scope.OrganizationID, SessionID: sessionID}, nil
}
func (g *participantRouteGateway) RetryParticipant(_ context.Context, scope app.DraftScope, sessionID string, _ app.ParticipantRetry) (app.Receipt, error) {
	g.calls++
	g.scope = scope
	return app.Receipt{SessionID: sessionID}, nil
}
func (g *participantRouteGateway) GetParticipantRetryReceipt(_ context.Context, scope app.DraftScope, _ string) (app.Receipt, error) {
	g.calls++
	g.scope = scope
	return app.Receipt{}, nil
}
func TestParticipantRetryRoutesRequireCurrentAdminAndTrustedScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sessionID := "00000000-0000-4000-8000-000000000001"
	body := `{"command_id":"00000000-0000-4000-8000-000000000002","expected_run_id":"00000000-0000-4000-8000-000000000003","expected_version":4,"reason":"重试","confirm":true,"expected_provider_invocations":1,"accept_result_unknown_risk":true,"organization_id":99,"operator_user_id":99}`
	for _, path := range []string{sessionID, sessionID + "/retry", "retry-commands/" + sessionID} {
		for _, admin := range []bool{false, true} {
			gateway := &participantRouteGateway{}
			engine := gin.New()
			engine.Use(aiRouteSnapshotMiddleware(admin))
			router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowParticipants: &app.ParticipantAdministration{Gateway: gateway}}})
			router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
			method := "GET"
			if path == sessionID+"/retry" {
				method = "POST"
			}
			req := httptest.NewRequest(method, "/internal/v2/interpretation/ai-workflow/participants/"+path+"?organization_id=99", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if admin && (w.Code != 200 || gateway.calls != 1 || gateway.scope.OrganizationID != 12 || gateway.scope.OperatorUserID != 34) {
				t.Fatal(w.Code, w.Body.String(), gateway)
			}
			if !admin && (w.Code != 403 || gateway.calls != 0) {
				t.Fatal(w.Code, gateway)
			}
		}
	}
}
