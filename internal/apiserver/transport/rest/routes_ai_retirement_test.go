package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRetiredAIGovernanceRoutesAreNotRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := newRouterWithBudgets(Deps{})
	e := gin.New()
	e.Use(aiRouteSnapshotMiddleware(true))
	r.registerInterpretationInternalRoutes(e.Group("/internal/v1"))
	r.registerInterpretationInternalV2Routes(e.Group("/internal/v2"))
	for _, path := range []string{
		"/internal/v1/interpretation/ai-explanation/profiles",
		"/internal/v1/interpretation/ai-explanation/prompt-evaluation-capacity",
		"/internal/v1/interpretation/ai-explanation/prompt-evaluations",
		"/internal/v1/interpretation/ai-explanation/runs/42",
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != http.StatusNotFound {
				t.Fatalf("retired route %s %s returned %d", method, path, w.Code)
			}
		}
	}
}
