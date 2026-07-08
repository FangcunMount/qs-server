package rest

import (
	"os"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

func TestCollectionOpenAPIContractCoversKeyRoutes(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")
	assertOpenAPIOperation(t, spec, "/answersheets", "post")
	assertOpenAPIOperation(t, spec, "/answersheets/submit-status", "get")
	assertOpenAPIOperation(t, spec, "/assessments/{id}/wait-report", "get")
	assertOpenAPIOperation(t, spec, "/questionnaires/{code}", "get")
	assertOpenAPIOperation(t, spec, "/typology-assessment-sessions", "post")
	assertOpenAPIOperation(t, spec, "/scales/hot", "get")
	assertOpenAPIOperation(t, spec, "/scales/categories", "get")
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
		"/assessments",
		"/assessments/{id}",
		"/assessments/{id}/report",
		"/answersheets/{id}/assessment",
	} {
		if _, ok := spec.Paths[path]; ok {
			t.Fatalf("legacy v1 assessment read path still in OpenAPI: %s", path)
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
	for _, path := range []string{"/typology-assessments", "/typology-models"} {
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

func normalizeCollectionOpenAPIPath(path string) string {
	// basePath 为 /api/v1，OpenAPI 生成时仅剥离 v1 前缀。
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
