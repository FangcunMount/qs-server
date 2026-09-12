package handler

import (
	"context"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	middleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func (g *managementGateway) PreviewEvaluationGates(ctx context.Context, s app.EvaluationScope, version int64) (app.EvaluationGatePreview, error) {
	_, err := g.GetEvaluation(ctx, s)
	return app.EvaluationGatePreview{RunID: s.RunID, Version: version, GateResult: []byte(`{}`)}, err
}
func TestGateHandlerUsesProtectedIdentityAndOneExplicitVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &managementGateway{}
	h := NewAIWorkflowManagementHandler(&app.EvaluationAdministration{Gateway: gateway})
	for _, suffix := range []string{"?expected_version=7&organization_id=999&operator_user_id=999", "", "?expected_version=7&expected_version=8", "?expected_version=0", "?expected_version=x", "?expected_version=9223372036854775808"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/"+suffix, nil)
		c.Params = gin.Params{{Key: "run_id", Value: "00000000-0000-4000-8000-000000000001"}}
		c.Set(middleware.OrgIDKey, uint64(12))
		c.Set(middleware.UserIDKey, uint64(34))
		snapshot := &authz.Snapshot{Permissions: []authz.Permission{{Resource: "qs:evaluation:collection:reports", Action: "audit", Mode: authz.AuthorizationModeUnconditional}}}
		c.Request = c.Request.WithContext(authz.WithSnapshot(c.Request.Context(), snapshot))
		h.PreviewGates(c)
		if suffix == "?expected_version=7&organization_id=999&operator_user_id=999" {
			if w.Code != 200 || gateway.scope.OrganizationID != 12 || gateway.scope.OperatorUserID != 34 {
				t.Fatal(w.Code, gateway.scope)
			}
		} else if w.Code == 200 {
			t.Fatal("invalid version accepted")
		}
	}
	if gateway.calls != 1 {
		t.Fatal("invalid version forwarded")
	}
}
