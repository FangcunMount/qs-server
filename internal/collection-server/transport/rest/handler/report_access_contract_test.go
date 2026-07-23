package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/testeeaccess"
	collectionmiddleware "github.com/FangcunMount/qs-server/internal/collection-server/transport/rest/middleware"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
)

type deniedTesteeAccess struct{}

func (deniedTesteeAccess) Authorize(context.Context, string, uint64) error {
	return testeeaccess.ErrAccessDenied
}

func TestAllReportStatusRoutesReturnSameForbiddenContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	medical := NewEvaluationHandler(nil, &fakeWaitReportService{})
	typology := NewTypologyAssessmentHandler(&fakeTypologyAssessmentQueryService{}, nil)
	behavior := NewBehaviorAssessmentHandler(&fakeBehaviorAssessmentQueryService{}, nil)

	tests := []struct {
		name    string
		path    string
		handler gin.HandlerFunc
	}{
		{name: "medical status", path: "/medical/:id/report-status", handler: medical.GetReportStatus},
		{name: "medical wait", path: "/medical/:id/wait-report", handler: medical.WaitReport},
		{name: "typology status", path: "/typology/:id/report-status", handler: typology.GetReportStatus},
		{name: "typology wait", path: "/typology/:id/wait-report", handler: typology.WaitReport},
		{name: "behavior status", path: "/behavior/:id/report-status", handler: behavior.GetReportStatus},
		{name: "behavior wait", path: "/behavior/:id/wait-report", handler: behavior.WaitReport},
	}

	var wantBody string
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("user_claims", &pkgmiddleware.UserClaims{UserID: "user-9"})
				c.Next()
			})
			router.GET(tt.path, collectionmiddleware.TesteeAccessMiddleware(deniedTesteeAccess{}, "testee_id"), tt.handler)

			recorder := httptest.NewRecorder()
			requestPath := tt.path
			requestPath = strings.ReplaceAll(requestPath, ":id", "42") + "?testee_id=7"
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
			}
			if wantBody == "" {
				wantBody = recorder.Body.String()
			} else if recorder.Body.String() != wantBody {
				t.Fatalf("forbidden response differs: got=%s want=%s", recorder.Body.String(), wantBody)
			}
		})
	}
}
