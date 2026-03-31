package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/gin-gonic/gin"
)

func TestRequireCapabilityMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	planManagerSnap := &authzapp.Snapshot{
		Roles: []string{"qs:evaluation_plan_manager"},
		Permissions: []authzapp.Permission{
			{Resource: "qs:evaluation_plans", Action: "create|read|list|update|pause|resume|cancel|enroll|terminate|statistics"},
			{Resource: "qs:evaluation_plan_tasks", Action: "schedule|read|list|open|complete|expire|cancel"},
		},
	}
	evaluatorSnap := &authzapp.Snapshot{
		Roles: []string{"qs:evaluator"},
		Permissions: []authzapp.Permission{
			{Resource: "qs:assessments", Action: "read|list|retry|batch_evaluate|statistics"},
		},
	}
	adminSnap := &authzapp.Snapshot{
		Roles: []string{"qs:admin"},
		Permissions: []authzapp.Permission{
			{Resource: "qs:*", Action: ".*"},
		},
	}

	tests := []struct {
		name        string
		capability  Capability
		snapshot    *authzapp.Snapshot
		wantStatus  int
		wantNextRun bool
	}{
		{
			name:        "plan manager can pass",
			capability:  CapabilityManageEvaluationPlans,
			snapshot:    planManagerSnap,
			wantStatus:  http.StatusOK,
			wantNextRun: true,
		},
		{
			name:        "evaluator cannot pass plan manager capability",
			capability:  CapabilityManageEvaluationPlans,
			snapshot:    evaluatorSnap,
			wantStatus:  http.StatusForbidden,
			wantNextRun: false,
		},
		{
			name:        "admin can pass org admin capability",
			capability:  CapabilityOrgAdmin,
			snapshot:    adminSnap,
			wantStatus:  http.StatusOK,
			wantNextRun: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set(AuthzSnapshotKey, tt.snapshot)

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
