package rest

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

func (g *candidateRouteGateway) CorrectEvaluationReview(_ context.Context, s app.EvaluationScope, _ app.EvaluationReviewCorrection) (app.EvaluationState, error) {
	g.writes++
	return app.EvaluationState{RunID: s.RunID}, nil
}

func TestWorkflowReviewCorrectionRouteRequiresCurrentOrgAdmin(t *testing.T) {
	for _, admin := range []bool{false, true} {
		gateway := &candidateRouteGateway{}
		router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowManagement: &app.EvaluationAdministration{Gateway: gateway}}})
		engine := gin.New()
		engine.Use(aiRouteSnapshotMiddleware(admin))
		router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
		body := `{"command_id":"00000000-0000-4000-8000-000000000009","expected_version":151,"candidate_id":"candidate:1","role":"safety_product","previous_review_fingerprint":"sha256:` + strings.Repeat("a", 64) + `","candidate_output_fingerprint":"sha256:` + strings.Repeat("b", 64) + `","decision":"approve","reason":"复核正文与引用","confirm":true}`
		request := httptest.NewRequest("POST", "/internal/v2/interpretation/ai-workflow/evaluations/00000000-0000-4000-8000-000000000001/review-corrections", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, request)
		if admin && (w.Code != 200 || gateway.writes != 1) {
			t.Fatal("authorized correction not forwarded", w.Code)
		}
		if !admin && (w.Code != 403 || gateway.writes != 0) {
			t.Fatal("audit-only user corrected review", w.Code)
		}
	}
}
