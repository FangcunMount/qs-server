package apiserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	handlerpkg "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
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
	router := NewRouter(newRouterTestContainer(), nil)
	router.RegisterRoutes(engine)

	routes := engine.Routes()
	assertRoutePresent(t, routes, http.MethodGet, "/health")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/public/assessment-entries/:token")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/testees/:id")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/clinicians")
	assertRoutePresent(t, routes, http.MethodPost, "/internal/v1/plans/tasks/window")
	assertRoutePresent(t, routes, http.MethodPost, "/internal/v1/statistics/sync/daily")
}

func TestRouterProtectedClinicianRouteRequiresCapabilitySnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	router := NewRouter(newRouterTestContainer(), nil)
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
	router := NewRouter(newRouterTestContainer(), nil)
	router.RegisterRoutes(engine)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clinicians", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func newRouterTestContainer() *container.Container {
	actorHandlerClinicianQuery := &routerClinicianQueryStub{}
	actorHandler := handlerpkg.NewActorHandler(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		actorHandlerClinicianQuery,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	return &container.Container{
		SurveyModule:     assembler.NewSurveyModule(),
		ScaleModule:      assembler.NewScaleModule(),
		ActorModule:      &assembler.ActorModule{ActorHandler: actorHandler},
		EvaluationModule: &assembler.EvaluationModule{},
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
