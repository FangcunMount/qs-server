package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/planentry"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

type entryRouteReader struct{ calls int }

func (r *entryRouteReader) ResolveTaskEntry(_ context.Context, id string) (*planentry.Entry, error) {
	r.calls++
	return &planentry.Entry{TaskID: id, TesteeID: "31", QuestionnaireCode: "exact"}, nil
}

type entryRouteAccess struct{}

func (entryRouteAccess) Authorize(context.Context, string, uint64) error { return nil }

func TestTaskEntryRouteContract(t *testing.T) {
	for _, tc := range []struct {
		path  string
		login bool
		want  int
		reads int
	}{
		{path: "/api/v1/plan-task-entries/42", login: true, want: http.StatusOK, reads: 1},
		{path: "/api/v1/plan-task-entries/42", want: http.StatusUnauthorized},
		{path: "/api/v1/plan-task-entries/42/old-secret", login: true, want: http.StatusNotFound},
		{path: "/api/v1/plan-task-entries/42/old-secret", want: http.StatusNotFound},
	} {
		t.Run(tc.path+http.StatusText(tc.want), func(t *testing.T) {
			c := mustNewCollectionContainer(t, options.NewOptions(), nil, nil, nil)
			r := &Router{container: c}
			engine := gin.New()
			api := engine.Group("/api/v1")
			api.Use(func(ctx *gin.Context) {
				if tc.login {
					ctx.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "11"})
				}
			})
			reader := &entryRouteReader{}
			r.registerPlanEntryRoutes(api, planentry.NewService(reader, entryRouteAccess{}))
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != tc.want || reader.calls != tc.reads {
				t.Fatalf("status=%d reads=%d body=%s", response.Code, reader.calls, response.Body)
			}
		})
	}
}
