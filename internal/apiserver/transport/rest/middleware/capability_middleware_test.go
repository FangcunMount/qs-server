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
			{Resource: "qs:answersheets", Action: "read|list|statistics"},
		},
	}
	contentManagerSnap := &authzapp.Snapshot{
		Roles: []string{"qs:content_manager"},
		Permissions: []authzapp.Permission{
			{Resource: "qs:questionnaires", Action: "create|read|list|update|delete|publish|unpublish|archive|statistics"},
			{Resource: "qs:scales", Action: "create|read|list|update|delete|publish|unpublish|archive"},
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
		{
			name:        "content manager can manage questionnaires",
			capability:  CapabilityManageQuestionnaires,
			snapshot:    contentManagerSnap,
			wantStatus:  http.StatusOK,
			wantNextRun: true,
		},
		{
			name:        "content manager can read scales",
			capability:  CapabilityReadScales,
			snapshot:    contentManagerSnap,
			wantStatus:  http.StatusOK,
			wantNextRun: true,
		},
		{
			name:        "evaluator can read answersheets",
			capability:  CapabilityReadAnswersheets,
			snapshot:    evaluatorSnap,
			wantStatus:  http.StatusOK,
			wantNextRun: true,
		},
		{
			name:        "content manager cannot read answersheets",
			capability:  CapabilityReadAnswersheets,
			snapshot:    contentManagerSnap,
			wantStatus:  http.StatusForbidden,
			wantNextRun: false,
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
