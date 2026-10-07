package rest

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAIAssetWritesKeepIndividualBodyLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []struct {
		name, path, body string
		limit            int
		engine           func() (*gin.Engine, *int)
	}{
		{"profile", profileBase + "/register", profileBody(), 256 * 1024, func() (*gin.Engine, *int) {
			gateway := &profileRouteGateway{}
			return profileRouter(gateway, true), &gateway.calls
		}},
		{"suite", suiteBase + "/register", suiteBody(), 16 * 1024, func() (*gin.Engine, *int) {
			gateway := &suiteRouteGateway{}
			return suiteRouter(gateway, true), &gateway.calls
		}},
		{"semantic", semanticDraftBase + "/" + publicationRouteID + "/revise", `{"draft_id":"` + publicationRouteID + `","command_id":"` + publicationRouteID + `","reason":"edit","expected_revision":1}`, 240 * 1024, func() (*gin.Engine, *int) {
			gateway := &semanticDraftRouteGateway{}
			return semanticDraftRouter(gateway, true), &gateway.calls
		}},
		{"solution", solutionBase + "/" + publicationRouteID + "/save", `{"command_id":"` + publicationRouteID + `","reason":"edit","expected_revision":1}`, 256 * 1024, func() (*gin.Engine, *int) {
			gateway := &solutionRouteGateway{}
			return solutionRouter(gateway, true), &gateway.calls
		}},
	} {
		for _, excess := range []int{0, 1} {
			t.Run(route.name+strings.Repeat("-over-limit", excess), func(t *testing.T) {
				prefix := strings.TrimSuffix(route.body, "}") + `,"padding":"`
				suffix := `"}`
				body := prefix + strings.Repeat("x", route.limit+excess-len(prefix)-len(suffix)) + suffix
				engine, calls := route.engine()
				response := httptest.NewRecorder()
				request := httptest.NewRequest("POST", route.path, strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				engine.ServeHTTP(response, request)
				wantStatus, wantCalls := 200, 1
				if excess > 0 {
					wantStatus, wantCalls = 400, 0
				}
				if response.Code != wantStatus || *calls != wantCalls {
					t.Fatalf("body length %d response/calls = %d/%d, want %d/%d: %s", len(body), response.Code, *calls, wantStatus, wantCalls, response.Body.String())
				}
			})
		}
	}
}
