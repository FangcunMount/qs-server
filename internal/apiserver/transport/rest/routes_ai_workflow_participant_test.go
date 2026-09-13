package rest

import (
	"context"
	"net/http/httptest"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

type participantRouteGateway struct {
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
