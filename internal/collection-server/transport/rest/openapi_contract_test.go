package rest

import (
	"os"
	"testing"

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
