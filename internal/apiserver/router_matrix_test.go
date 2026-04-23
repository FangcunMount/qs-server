package apiserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	handlerpkg "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
	"github.com/gin-gonic/gin"
)

type routerClinicianQueryStub struct{}

func (*routerClinicianQueryStub) GetByID(context.Context, uint64) (*clinicianApp.ClinicianResult, error) {
	return nil, nil
}

func (*routerClinicianQueryStub) GetByOperator(context.Context, int64, uint64) (*clinicianApp.ClinicianResult, error) {
	return nil, nil
}

func (*routerClinicianQueryStub) ListClinicians(context.Context, clinicianApp.ListClinicianDTO) (*clinicianApp.ClinicianListResult, error) {
	return &clinicianApp.ClinicianListResult{
		Items: []*clinicianApp.ClinicianResult{{ID: 1, OrgID: 88, Name: "Dr. Router", IsActive: true}},
	}, nil
}

func TestRouterRegisterRoutesIncludesKeyPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	router := resttransport.NewRouter(newRouterTestContainer().BuildRESTDeps(nil))
	router.RegisterRoutes(engine)

	routes := engine.Routes()
	assertRoutePresent(t, routes, http.MethodGet, "/health")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/public/assessment-entries/:token")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/questionnaires")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/answersheets")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/scales")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/evaluations/assessments")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/testees/:id")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/testees/:id/clinicians")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/clinicians")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/practitioners")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/staff")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/assessment-entries/:id")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/overview")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/testees/:id/plans")
	assertRoutePresent(t, routes, http.MethodPost, "/internal/v1/plans/tasks/window")
	assertRoutePresent(t, routes, http.MethodPost, "/internal/v1/statistics/sync/daily")
	assertRoutePresent(t, routes, http.MethodGet, "/internal/v1/cache/governance/status")
}

func TestRouterProtectedClinicianRouteRequiresCapabilitySnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	router := resttransport.NewRouter(newRouterTestContainer().BuildRESTDeps(nil))
	router.RegisterRoutes(engine)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clinicians", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRouterProtectedClinicianRoutePassesCapabilityMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(restmiddleware.OrgIDKey, uint64(88))
		c.Set(restmiddleware.UserIDKey, uint64(701))
		c.Set(restmiddleware.AuthzSnapshotKey, &authzapp.Snapshot{
			Roles: []string{"qs:admin"},
			Permissions: []authzapp.Permission{
				{Resource: "qs:*", Action: ".*"},
			},
		})
		c.Next()
	})
	router := resttransport.NewRouter(newRouterTestContainer().BuildRESTDeps(nil))
	router.RegisterRoutes(engine)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clinicians", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func newRouterTestContainer() *container.Container {
	clinicianQuery := &routerClinicianQueryStub{}
	surveyModule := &assembler.SurveyModule{
		Questionnaire: &assembler.QuestionnaireSubModule{
			Handler: handlerpkg.NewQuestionnaireHandler(nil, nil, nil, nil),
		},
		AnswerSheet: &assembler.AnswerSheetSubModule{
			Handler: handlerpkg.NewAnswerSheetHandler(nil, nil),
		},
	}
	scaleModule := &assembler.ScaleModule{
		Handler: handlerpkg.NewScaleHandler(nil, nil, nil, nil, nil),
	}
	evaluationModule := &assembler.EvaluationModule{
		Handler: handlerpkg.NewEvaluationHandler(nil, nil, nil, nil),
	}
	testeeHandler := handlerpkg.NewTesteeHandler(nil, nil, nil, nil, nil, nil, nil, nil)
	operatorClinicianHandler := handlerpkg.NewOperatorClinicianHandler(nil, nil, nil, nil, clinicianQuery, nil, nil, nil)
	assessmentEntryHandler := handlerpkg.NewAssessmentEntryHandler(nil, clinicianQuery, nil, nil)
	return &container.Container{
		SurveyModule: surveyModule,
		ScaleModule:  scaleModule,
		ActorModule: &assembler.ActorModule{
			TesteeHandler:            testeeHandler,
			OperatorClinicianHandler: operatorClinicianHandler,
			AssessmentEntryHandler:   assessmentEntryHandler,
		},
		EvaluationModule: evaluationModule,
		PlanModule: &assembler.PlanModule{
			Handler: handlerpkg.NewPlanHandler(nil, nil),
		},
		StatisticsModule: &assembler.StatisticsModule{
			Handler: handlerpkg.NewStatisticsHandler(nil, nil, nil, nil, nil, nil, nil),
		},
	}
}

func assertRoutePresent(t *testing.T, routes gin.RoutesInfo, method, path string) {
	t.Helper()
	for _, route := range routes {
		if route.Method == method && route.Path == path {
			return
		}
	}
	t.Fatalf("route %s %s not registered", method, path)
}
