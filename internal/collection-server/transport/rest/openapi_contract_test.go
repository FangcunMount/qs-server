package rest

import (
	"os"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

func TestCollectionOpenAPIContractCoversKeyRoutes(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")
	assertOpenAPIOperation(t, spec, "/answersheets", "post")
	assertOpenAPIOperation(t, spec, "/answersheets/submit-status", "get")
	assertOpenAPIOperation(t, spec, "/assessments", "get")
	assertOpenAPIOperation(t, spec, "/assessments/{id}/wait-report", "get")
	assertOpenAPIOperation(t, spec, "/questionnaires/{code}", "get")
	assertOpenAPIOperation(t, spec, "/typology-assessment-sessions", "post")
	assertOpenAPIOperation(t, spec, "/assessment-models", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models/hot", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models/options", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}", "get")
	assertOpenAPIOperation(t, spec, "/typology-models", "get")
	assertOpenAPIOperation(t, spec, "/typology-models/categories", "get")
	assertOpenAPIOperation(t, spec, "/typology-assessments", "get")
	assertOpenAPIOperation(t, spec, "/typology-assessments/{id}/report", "get")
	assertOpenAPIOperation(t, spec, "/typology-assessments/{id}/report-status", "get")
	assertOpenAPIOperation(t, spec, "/typology-assessments/{id}/wait-report", "get")
	assertOpenAPIOperation(t, spec, "/report-events", "get")
	assertOpenAPIOperation(t, spec, "/testees/{id}/care-context", "get")
	assertOpenAPIOperation(t, spec, "/health", "get")
}

func TestCollectionOpenAPIUsesStringTesteeIDAndCurrentReportStatuses(t *testing.T) {
	t.Parallel()

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/collection.yaml")
	submit := schemas["answersheet.SubmitAnswerSheetRequest"].(map[string]any)
	testeeID := submit["properties"].(map[string]any)["testee_id"].(map[string]any)
	if testeeID["type"] != "string" {
		t.Fatalf("testee_id schema type = %v, want string", testeeID["type"])
	}

	for _, name := range []string{"evaluation.AssessmentStatusResponse", "typologyassessment.AssessmentStatusResponse"} {
		status := schemas[name].(map[string]any)["properties"].(map[string]any)["status"].(map[string]any)
		if !openAPIEnumEquals(status["enum"], "processing", "interpreted", "failed") {
			t.Fatalf("%s.status enum = %v, want processing/interpreted/failed", name, status["enum"])
		}
	}
	frame := schemas["ws.ReportEventsStatusFrame"].(map[string]any)
	data := frame["properties"].(map[string]any)["data"].(map[string]any)
	status := data["properties"].(map[string]any)["status"].(map[string]any)
	if !openAPIEnumEquals(status["enum"], "processing", "interpreted", "failed") {
		t.Fatalf("websocket status enum = %v, want processing/interpreted/failed", status["enum"])
	}
}

func openAPIEnumEquals(raw any, want ...string) bool {
	values, ok := raw.([]any)
	if !ok || len(values) != len(want) {
		return false
	}
	for index, value := range values {
		if value != want[index] {
			return false
		}
	}
	return true
}

func TestCollectionOpenAPIHasNoLegacyPersonalityPaths(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")
	for path := range spec.Paths {
		if strings.Contains(path, "/personality-") {
			t.Fatalf("legacy personality path still in OpenAPI: %s", path)
		}
	}
}

func TestCollectionOpenAPIHasNoLegacyV1AssessmentReadPaths(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")
	for _, path := range []string{
		"/api/v1/assessments/{id}",
		"/api/v1/assessments/{id}/report",
		"/api/v1/answersheets/{id}/assessment",
	} {
		if _, ok := spec.Paths[path]; ok {
			t.Fatalf("legacy v1 assessment read path still in OpenAPI: %s", path)
		}
	}
}

func TestCollectionOpenAPIHasNoScaleRoutes(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")
	for path := range spec.Paths {
		if strings.HasPrefix(path, "/api/v1/scales") {
			t.Fatalf("legacy scale path still in OpenAPI: %s", path)
		}
	}
}

func TestCollectionOpenAPIHasNoLegacyV2AssessmentOutcomePaths(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")
	for path := range spec.Paths {
		if strings.HasPrefix(path, "/api/v2/assessments") {
			t.Fatalf("legacy v2 assessment outcome path still in OpenAPI: %s", path)
		}
	}
}

func TestCollectionOpenAPIHasNoLegacyAssessmentSchemas(t *testing.T) {
	t.Parallel()

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/collection.yaml")
	for name := range schemas {
		if strings.Contains(name, "LegacyAssessment") || strings.Contains(name, "LegacyList") {
			t.Fatalf("legacy assessment schema still in OpenAPI: %s", name)
		}
	}
}

func TestCollectionOpenAPIAssessmentOutcomeSchemasHaveNoLegacyFields(t *testing.T) {
	t.Parallel()

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/collection.yaml")
	for _, name := range []string{
		"evaluation.AssessmentSummaryResponse",
		"typologyassessment.AssessmentDetailResponse",
		"typologyassessment.AssessmentReportResponse",
	} {
		schema, ok := schemas[name].(map[string]any)
		if !ok {
			t.Fatalf("missing outcome schema %s", name)
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("schema %s has no properties", name)
		}
		for _, forbidden := range []string{"scale_code", "scale_name", "total_score", "risk_level"} {
			if _, exists := props[forbidden]; exists {
				t.Fatalf("schema %s still exposes legacy outcome field %q", name, forbidden)
			}
		}
	}
}

func TestCollectionOpenAPIHasNoLegacyAlgorithmQueryParam(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")
	for _, path := range []string{"/api/v1/typology-assessments", "/api/v1/typology-models"} {
		ops, ok := spec.Paths[path]
		if !ok {
			t.Fatalf("OpenAPI missing path %s", path)
		}
		getOps, ok := ops["get"].(map[string]any)
		if !ok {
			t.Fatalf("OpenAPI path %s missing GET operation", path)
		}
		params, _ := getOps["parameters"].([]any)
		for _, raw := range params {
			param, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if param["in"] == "query" && param["name"] == "algorithm" {
				t.Fatalf("legacy algorithm query param still in OpenAPI: %s", path)
			}
		}
	}
}

func TestCollectionOpenAPIHasNoLegacyPersonalitySessionSchemas(t *testing.T) {
	t.Parallel()

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/collection.yaml")
	for name := range schemas {
		if strings.Contains(name, "personalitysession.") {
			t.Fatalf("legacy personality session schema still in OpenAPI: %s", name)
		}
	}
}

func TestCollectionRESTRegistersMedicalAssessmentListRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c := container.NewContainer(
		options.NewOptions(),
		nil,
		nil,
		observability.NewFamilyStatusRegistry("collection-server"),
	)
	if err := c.Initialize(); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	NewRouter(c).RegisterRoutes(router)
	found := false
	for _, route := range router.Routes() {
		if route.Method == "GET" && route.Path == "/api/v1/assessments" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("GET /api/v1/assessments route not registered")
	}
}

func TestCollectionRESTDoesNotRegisterLegacyV2AssessmentOutcomeRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c := container.NewContainer(
		options.NewOptions(),
		nil,
		nil,
		observability.NewFamilyStatusRegistry("collection-server"),
	)
	if err := c.Initialize(); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	NewRouter(c).RegisterRoutes(router)
	for _, route := range router.Routes() {
		if strings.HasPrefix(route.Path, "/api/v2/assessments") {
			t.Fatalf("legacy v2 assessment outcome route still registered: %s %s", route.Method, route.Path)
		}
	}
}

func TestCollectionRESTDoesNotDocumentLegacyTypologyAssessmentCompatibility(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("handler/evaluation_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Deprecated: 请优先使用 /api/v1/typology-assessments") {
		t.Fatal("legacy typology assessment compatibility handler comment still exists")
	}
}

func TestCollectionPublicBusinessRoutesAreCoveredByOpenAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c := container.NewContainer(
		options.NewOptions(),
		nil,
		nil,
		observability.NewFamilyStatusRegistry("collection-server"),
	)
	if err := c.Initialize(); err != nil {
		t.Fatal(err)
	}

	engine := gin.New()
	NewRouter(c).RegisterRoutes(engine)

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")
	missing := 0
	for _, route := range engine.Routes() {
		if !collectionRouteMustBeDocumented(route) {
			continue
		}
		path := normalizeCollectionOpenAPIPath(route.Path)
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

type openAPISpec struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

func loadOpenAPISpec(t *testing.T, path string) openAPISpec {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var spec openAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(spec.Paths) == 0 {
		t.Fatalf("%s has no OpenAPI paths", path)
	}
	return spec
}

func loadOpenAPIComponents(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	components, ok := root["components"].(map[string]any)
	if !ok {
		t.Fatal("missing components")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("missing components.schemas")
	}
	return schemas
}

func assertOpenAPIOperation(t *testing.T, spec openAPISpec, path, method string) {
	t.Helper()
	if path != "/health" && path != "/ping" && !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/internal/") {
		path = "/api/v1" + path
	}
	ops, ok := spec.Paths[path]
	if !ok {
		t.Fatalf("OpenAPI missing path %s", path)
	}
	if _, ok := ops[method]; !ok {
		t.Fatalf("OpenAPI path %s missing method %s", path, method)
	}
}

func collectionRouteMustBeDocumented(route gin.RouteInfo) bool {
	if route.Method != "GET" &&
		route.Method != "POST" &&
		route.Method != "PUT" &&
		route.Method != "DELETE" {
		return false
	}
	switch {
	case strings.HasPrefix(route.Path, "/api/rest/"):
		return false
	case strings.HasPrefix(route.Path, "/swagger-ui/"):
		return false
	case route.Path == "/swagger":
		return false
	default:
		return true
	}
}

func normalizeCollectionOpenAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}
