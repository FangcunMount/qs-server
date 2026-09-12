package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	middleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

type managementGateway struct {
	reopen   app.EvaluationReopen
	finalize app.EvaluationFinalize
	review   app.EvaluationReview
	calls    int
	scope    app.EvaluationScope
}

func (g *managementGateway) GetEvaluation(_ context.Context, s app.EvaluationScope) (app.EvaluationState, error) {
	g.calls++
	g.scope = s
	return app.EvaluationState{RunID: s.RunID, Version: 7, Status: "collecting", Resolutions: []byte("[]")}, nil
}
func (g *managementGateway) ResolveUnknown(ctx context.Context, s app.EvaluationScope, _ app.UnknownResolution) (app.EvaluationState, error) {
	return g.GetEvaluation(ctx, s)
}
func TestAIWorkflowManagementUsesProtectedIdentityNotBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &managementGateway{}
	h := NewAIWorkflowManagementHandler(&app.EvaluationAdministration{Gateway: gateway})
	body := `{"organization_id":999,"operator_user_id":999,"expected_version":6,"execution_id":"execution:1","decision":"cancel_run","reason":"确认取消","confirm":true,"acknowledged_duplicate_call_and_cost_risk":true}`
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "run_id", Value: "00000000-0000-4000-8000-000000000001"}}
	ctx.Set(middleware.OrgIDKey, uint64(12))
	ctx.Set(middleware.UserIDKey, uint64(34))
	snapshot := &authz.Snapshot{EffectiveRoles: []string{"qs:admin"}, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}}
	ctx.Request = ctx.Request.WithContext(authz.WithSnapshot(ctx.Request.Context(), snapshot))
	h.ResolveUnknown(ctx)
	if w.Code != 200 || gateway.calls != 1 || gateway.scope.OrganizationID != 12 || gateway.scope.OperatorUserID != 34 {
		t.Fatalf("status=%d calls=%d scope=%+v", w.Code, gateway.calls, gateway.scope)
	}
}
func TestAIWorkflowManagementRejectsMissingIdentity(t *testing.T) {
	gateway := &managementGateway{}
	h := NewAIWorkflowManagementHandler(&app.EvaluationAdministration{Gateway: gateway})
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest("GET", "/", nil)
	h.Get(ctx)
	if w.Code == 200 || gateway.calls != 0 {
		t.Fatalf("status=%d calls=%d", w.Code, gateway.calls)
	}
}

func (g *managementGateway) StartEvaluation(ctx context.Context, s app.EvaluationScope, _ app.EvaluationStart) (app.EvaluationState, error) {
	return g.GetEvaluation(ctx, s)
}

func TestAIWorkflowStartUsesProtectedIdentityNotBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &managementGateway{}
	h := NewAIWorkflowManagementHandler(&app.EvaluationAdministration{Gateway: gateway})
	body := `{"organization_id":999,"operator_user_id":999,"expected_version":6,"execution_id":"execution:1","decision":"cancel_run","reason":"确认取消","confirm":true,"acknowledged_duplicate_call_and_cost_risk":true}`
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "run_id", Value: "00000000-0000-4000-8000-000000000001"}}
	ctx.Set(middleware.OrgIDKey, uint64(12))
	ctx.Set(middleware.UserIDKey, uint64(34))
	snapshot := &authz.Snapshot{EffectiveRoles: []string{"qs:admin"}, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}}
	ctx.Request = ctx.Request.WithContext(authz.WithSnapshot(ctx.Request.Context(), snapshot))
	h.Start(ctx)
	if w.Code != 200 || gateway.calls != 1 || gateway.scope.OrganizationID != 12 || gateway.scope.OperatorUserID != 34 {
		t.Fatalf("status=%d calls=%d scope=%+v", w.Code, gateway.calls, gateway.scope)
	}
}

func (g *managementGateway) CreateEvaluation(ctx context.Context, s app.EvaluationScope, _ app.EvaluationCreate) (app.EvaluationState, error) {
	return g.GetEvaluation(ctx, s)
}

func TestAIWorkflowCreateUsesProtectedIdentityNotBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &managementGateway{}
	h := NewAIWorkflowManagementHandler(&app.EvaluationAdministration{Gateway: gateway})
	refs := map[string]any{}
	for _, name := range []string{"suite", "profile", "prompt", "input_schema", "output_schema", "generation_route", "semantic_prompt", "semantic_output_schema", "semantic_route", "execution_policy", "gate_policy"} {
		refs[name] = map[string]string{"id": name, "version": "v1", "fingerprint": "sha256:" + strings.Repeat("a", 64)}
	}
	raw, _ := json.Marshal(map[string]any{"organization_id": 999, "operator_user_id": 999, "release": refs, "reason": "创建评测", "confirm": true})
	body := string(raw)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "run_id", Value: "00000000-0000-4000-8000-000000000001"}}
	ctx.Set(middleware.OrgIDKey, uint64(12))
	ctx.Set(middleware.UserIDKey, uint64(34))
	snapshot := &authz.Snapshot{EffectiveRoles: []string{"qs:admin"}, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}}
	ctx.Request = ctx.Request.WithContext(authz.WithSnapshot(ctx.Request.Context(), snapshot))
	h.Create(ctx)
	if w.Code != 200 || gateway.calls != 1 || gateway.scope.OrganizationID != 12 || gateway.scope.OperatorUserID != 34 {
		t.Fatalf("status=%d calls=%d scope=%+v", w.Code, gateway.calls, gateway.scope)
	}
}

func (g *managementGateway) ReviewEvaluation(ctx context.Context, s app.EvaluationScope, value app.EvaluationReview) (app.EvaluationState, error) {
	g.review = value
	return g.GetEvaluation(ctx, s)
}

func TestAIWorkflowReviewUsesProtectedScopeAndDoesNotAcceptClientAuditIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &managementGateway{}
	h := NewAIWorkflowManagementHandler(&app.EvaluationAdministration{Gateway: gateway})
	body := `{"organization_id":999,"operator_user_id":999,"expected_version":7,"role":"assessment_semantics","reviews":[{"candidate_id":"candidate:1","decision":"approve","reason":"核对事实","reviewer":"user:999","reviewed_at":"2000-01-01"}]}`
	for _, authorized := range []bool{true, false} {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "run_id", Value: "00000000-0000-4000-8000-000000000001"}}
		ctx.Set(middleware.OrgIDKey, uint64(12))
		ctx.Set(middleware.UserIDKey, uint64(34))
		if authorized {
			snapshot := &authz.Snapshot{EffectiveRoles: []string{"qs:admin"}, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}}
			ctx.Request = ctx.Request.WithContext(authz.WithSnapshot(ctx.Request.Context(), snapshot))
		}
		h.Review(ctx)
		if authorized != (w.Code == 200) {
			t.Fatalf("authorization=%v status=%d", authorized, w.Code)
		}
	}
	if gateway.calls != 1 || gateway.scope.OrganizationID != 12 || gateway.scope.OperatorUserID != 34 {
		t.Fatal("review identity or revocation bypassed")
	}
	raw, err := json.Marshal(gateway.review)
	if err != nil || strings.Contains(string(raw), "user:999") || strings.Contains(string(raw), "2000-01-01") {
		t.Fatal("client audit identity forwarded")
	}
}
