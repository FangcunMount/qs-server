package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/gin-gonic/gin"
)

func TestRequireCapabilityMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		capability  Capability
		roles       []string
		wantStatus  int
		wantNextRun bool
	}{
		{
			name:        "plan manager can pass",
			capability:  CapabilityManageEvaluationPlans,
			roles:       []string{operator.RoleEvaluationPlanManager.String()},
			wantStatus:  http.StatusOK,
			wantNextRun: true,
		},
		{
			name:        "evaluator cannot pass plan manager capability",
			capability:  CapabilityManageEvaluationPlans,
			roles:       []string{operator.RoleEvaluatorQS.String()},
			wantStatus:  http.StatusForbidden,
			wantNextRun: false,
		},
		{
			name:        "admin can pass all admin capability",
			capability:  CapabilityOrgAdmin,
			roles:       []string{operator.RoleQSAdmin.String()},
			wantStatus:  http.StatusOK,
			wantNextRun: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set(RolesKey, tt.roles)

			nextRun := false
			mw := RequireCapabilityMiddleware(tt.capability)
			mw(c)
			if !c.IsAborted() {
				nextRun = true
				c.Status(http.StatusOK)
			}

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if nextRun != tt.wantNextRun {
				t.Fatalf("nextRun = %v, want %v", nextRun, tt.wantNextRun)
			}
		})
	}
}
