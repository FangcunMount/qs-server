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
	assertOpenAPIOperation(t, spec, "/answersheets/{id}/assessment", "get")
	assertOpenAPIOperation(t, spec, "/assessments/{id}/wait-report", "get")
	assertOpenAPIOperation(t, spec, "/questionnaires/{code}", "get")
	assertOpenAPIOperation(t, spec, "/scales/categories", "get")
	assertOpenAPIOperation(t, spec, "/testees/{id}/care-context", "get")
	assertOpenAPIOperation(t, spec, "/health", "get")
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
