package rest

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

type candidateRouteGateway struct {
	app.EvaluationGateway
	reads, writes int
}

func (g *candidateRouteGateway) ListEvaluationCandidates(_ context.Context, s app.EvaluationScope) (app.EvaluationCandidateIndex, error) {
	g.reads++
	return app.EvaluationCandidateIndex{RunID: s.RunID, Version: 1, Candidates: []app.EvaluationCandidateSummary{}}, nil
}
func (g *candidateRouteGateway) GetEvaluationCandidate(_ context.Context, s app.EvaluationScope, q app.CandidateQuery) (app.EvaluationCandidateEvidence, error) {
	g.reads++
	return app.EvaluationCandidateEvidence{RunID: s.RunID, Version: q.ExpectedVersion, CandidateID: q.CandidateID, Evidence: []byte(`{}`)}, nil
}
func (g *candidateRouteGateway) PreviewEvaluationGates(_ context.Context, s app.EvaluationScope, version int64) (app.EvaluationGatePreview, error) {
	g.reads++
	return app.EvaluationGatePreview{RunID: s.RunID, Version: version, GateResult: []byte(`{}`)}, nil
}
func (g *candidateRouteGateway) ReviewEvaluation(_ context.Context, s app.EvaluationScope, _ app.EvaluationReview) (app.EvaluationState, error) {
	g.writes++
	return app.EvaluationState{RunID: s.RunID}, nil
}

func (g *candidateRouteGateway) FinalizeEvaluation(_ context.Context, s app.EvaluationScope, _ app.EvaluationFinalize) (app.EvaluationState, error) {
	g.writes++
	return app.EvaluationState{RunID: s.RunID}, nil
}

func TestWorkflowFinalizationRouteRequiresCurrentOrgAdmin(t *testing.T) {
	for _, admin := range []bool{false, true} {
		gateway := &candidateRouteGateway{}
		router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowManagement: &app.EvaluationAdministration{Gateway: gateway}}})
		engine := gin.New()
		engine.Use(aiRouteSnapshotMiddleware(admin))
		router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
		request := httptest.NewRequest("POST", "/internal/v2/interpretation/ai-workflow/evaluations/00000000-0000-4000-8000-000000000001/finalize", strings.NewReader(`{"expected_version":7,"expected_passed":false,"reason":"核对后拒绝","confirm":true}`))
		request.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, request)
		if admin && (w.Code != 200 || gateway.writes != 1) {
			t.Fatal("authorized finalization not forwarded", w.Code)
		}
		if !admin && (w.Code != 403 || gateway.writes != 0) {
			t.Fatal("audit-only permission allowed finalization", w.Code)
		}
	}
}
func TestWorkflowRoutesAllowAuditReadsButDenyAuditOnlyReview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &candidateRouteGateway{}
	router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowManagement: &app.EvaluationAdministration{Gateway: gateway}}})
	engine := gin.New()
	engine.Use(aiRouteSnapshotMiddleware(false))
	router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
	base := "/internal/v2/interpretation/ai-workflow/evaluations/00000000-0000-4000-8000-000000000001"
	for _, suffix := range []string{"/candidates", "/candidates/candidate:1?expected_version=1", "/gates?expected_version=1"} {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest("GET", base+suffix, nil))
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	request := httptest.NewRequest("POST", base+"/reviews", strings.NewReader(`{"expected_version":1,"role":"assessment_semantics","reviews":[{"candidate_id":"candidate:1","decision":"approve","reason":"核对"}]}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, request)
	if w.Code != 403 || gateway.reads != 3 || gateway.writes != 0 {
		t.Fatalf("status %d reads %d writes %d", w.Code, gateway.reads, gateway.writes)
	}
}
