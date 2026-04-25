package rest

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestApiserverOpenAPIContractCoversKeyPublicRoutes(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/apiserver.yaml")
	assertOpenAPIOperation(t, spec, "/questionnaires", "get")
	assertOpenAPIOperation(t, spec, "/questionnaires/{code}", "get")
	assertOpenAPIOperation(t, spec, "/answersheets/admin-submit", "post")
	assertOpenAPIOperation(t, spec, "/scales", "get")
	assertOpenAPIOperation(t, spec, "/scales/{code}/publish", "post")
	assertOpenAPIOperation(t, spec, "/evaluations/assessments", "get")
	assertOpenAPIOperation(t, spec, "/plans/{id}/tasks", "get")
	assertOpenAPIOperation(t, spec, "/statistics/system", "get")
	assertOpenAPIOperation(t, spec, "/testees/{id}", "get")
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
