package apiserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
	handlerpkg "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
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
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/clinicians")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/clinicians/:id")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/entries")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/entries/:id")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/clinicians/me/overview")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/clinicians/me/entries")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/clinicians/me/testees-summary")
	assertRoutePresent(t, routes, http.MethodPost, "/api/v1/statistics/questionnaires/batch")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/system")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/questionnaires/:code")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/testees/:testee_id")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/testees/:testee_id/periodic")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/statistics/plans/:plan_id")
	assertRoutePresent(t, routes, http.MethodGet, "/api/v1/testees/:id/plans")
	assertRoutePresent(t, routes, http.MethodPost, "/internal/v1/plans/tasks/window")
	assertRoutePresent(t, routes, http.MethodPost, "/internal/v1/statistics/sync/daily")
	assertRoutePresent(t, routes, http.MethodPost, "/internal/v1/statistics/sync/org-snapshot")
	assertRoutePresent(t, routes, http.MethodPost, "/internal/v1/statistics/sync/plan")
	assertRoutePresent(t, routes, http.MethodGet, "/internal/v1/cache/governance/status")
	assertRoutePresent(t, routes, http.MethodGet, "/internal/v1/events/status")
	assertRoutePresent(t, routes, http.MethodGet, "/internal/v1/resilience/status")
}

func TestRouterPublicBusinessRoutesAreCoveredByOpenAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	router := resttransport.NewRouter(newRouterTestContainer().BuildRESTDeps(nil))
	router.RegisterRoutes(engine)

	spec := loadRouterMatrixOpenAPI(t, "../../api/rest/apiserver.yaml")
	missing := 0
	for _, route := range engine.Routes() {
		if !routeMustBeDocumented(route) {
			continue
		}
		path := normalizeOpenAPIPath(route.Path)
		method := strings.ToLower(route.Method)
		ops, ok := spec.Paths[path]
		if !ok {
			t.Errorf("OpenAPI missing route %s %s normalized as %s", route.Method, route.Path, path)
			missing++
			continue
		}
		if _, ok := ops[method]; !ok {
			t.Errorf("OpenAPI path %s missing method %s for route %s %s", path, method, route.Method, route.Path)
			missing++
		}
	}
	if missing > 0 {
		t.Fatalf("OpenAPI is missing %d registered public/business routes", missing)
	}
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

func TestTransportPlaneDoesNotUseLegacyInterfaceImplementation(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("go.mod not found")
		}
		root = next
	}

	forbidden := []string{
		"internal/apiserver/interface/restful",
		"internal/apiserver/interface/grpc/service",
	}
	err = filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasSuffix(filepath.ToSlash(path), "/internal/apiserver/interface/grpc/proto") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(data)
		for _, token := range forbidden {
			if strings.Contains(source, token) {
				t.Fatalf("%s imports legacy transport implementation path %s", path, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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

type routerMatrixOpenAPISpec struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

func loadRouterMatrixOpenAPI(t *testing.T, path string) routerMatrixOpenAPISpec {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var spec routerMatrixOpenAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(spec.Paths) == 0 {
		t.Fatalf("%s has no OpenAPI paths", path)
	}
	return spec
}

func routeMustBeDocumented(route gin.RouteInfo) bool {
	if route.Method != http.MethodGet &&
		route.Method != http.MethodPost &&
		route.Method != http.MethodPut &&
		route.Method != http.MethodDelete {
		return false
	}
	switch {
	case strings.HasPrefix(route.Path, "/internal/v1/"):
		return false
	case strings.HasPrefix(route.Path, "/governance/"):
		return false
	case strings.HasPrefix(route.Path, "/api/rest/"):
		return false
	case strings.HasPrefix(route.Path, "/swagger-ui/"):
		return false
	case route.Path == "/swagger" || route.Path == "/readyz":
		return false
	default:
		return true
	}
}

func normalizeOpenAPIPath(path string) string {
	path = strings.TrimPrefix(path, "/api/v1")
	if path == "" {
		path = "/"
	}
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}
