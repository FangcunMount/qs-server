package rest

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestApiserverOpenAPIContractCoversKeyPublicRoutes(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/apiserver.yaml")
	assertOpenAPIOperation(t, spec, "/questionnaires", "get")
	assertOpenAPIOperation(t, spec, "/questionnaires/{code}", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models", "post")
	assertOpenAPIOperation(t, spec, "/assessment-models/options", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/restore-draft", "post")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/definition", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/definition", "put")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/questionnaire", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/questionnaire", "put")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/codes/apply", "post")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/preview-report", "post")
	assertOpenAPIOperation(t, spec, "/answersheets/admin-submit", "post")
	assertOpenAPIOperation(t, spec, "/evaluations/assessments", "get")
	assertOpenAPIOperation(t, spec, "/plans/{id}/tasks", "get")
	assertOpenAPIOperation(t, spec, "/statistics/overview", "get")
	assertOpenAPIOperation(t, spec, "/statistics/clinicians", "get")
	assertOpenAPIOperation(t, spec, "/statistics/clinicians/{id}", "get")
	assertOpenAPIOperation(t, spec, "/statistics/clinicians/me/overview", "get")
	assertOpenAPIOperation(t, spec, "/statistics/clinicians/me/entries", "get")
	assertOpenAPIOperation(t, spec, "/statistics/clinicians/me/testees-summary", "get")
	assertOpenAPIOperation(t, spec, "/statistics/entries", "get")
	assertOpenAPIOperation(t, spec, "/statistics/entries/{id}", "get")
	assertOpenAPIOperation(t, spec, "/statistics/testees/{testee_id}/periodic", "get")
	assertOpenAPIOperation(t, spec, "/statistics/questionnaires/batch", "post")
	assertOpenAPIOperation(t, spec, "/statistics/system", "get")
	assertOpenAPIOperation(t, spec, "/testees/{id}", "get")
	assertOpenAPIOperation(t, spec, "/health", "get")
}

func TestApiserverOpenAPIHasExplicitModelAndInterpretationWireSchemas(t *testing.T) {
	t.Parallel()

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/apiserver.yaml")
	for name, properties := range map[string][]string{
		"response.DefinitionV2Wire":             {"Measure", "Calibration", "Conclusions", "Outcomes", "ReportMap"},
		"response.PreviewReportRequestWire":     {"answers", "sample_id"},
		"response.PreviewReportWire":            {"outcome", "score_detail", "report_sections"},
		"response.InterpretationGenerationWire": {"ID", "OutcomeID", "LatestRun", "Report"},
		"response.InterpretationRunWire":        {"ID", "GenerationID", "Status", "Failure"},
	} {
		schema, ok := schemas[name].(map[string]any)
		if !ok {
			t.Fatalf("missing explicit wire schema %s", name)
		}
		actual, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("wire schema %s has no properties", name)
		}
		for _, property := range properties {
			if _, ok := actual[property]; !ok {
				t.Fatalf("wire schema %s missing property %q", name, property)
			}
		}
	}
}

func TestApiserverOpenAPIPreservesRootAndOperationSecurity(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../api/rest/apiserver.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if _, ok := root["security"].([]any); !ok {
		t.Fatal("OpenAPI must retain root security")
	}
	paths := root["paths"].(map[string]any)
	publicInfo := paths["/api/v1/public/info"].(map[string]any)["get"].(map[string]any)
	if security, ok := publicInfo["security"].([]any); !ok || len(security) != 0 {
		t.Fatal("public operation must explicitly override root security")
	}
	protected := paths["/api/v1/clinicians/me/workbench/queues/summary"].(map[string]any)["get"].(map[string]any)
	if _, ok := protected["security"].([]any); !ok {
		t.Fatal("operation-level security must be retained")
	}
}

type openAPISpec struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

type openAPIRoot struct {
	Paths      map[string]map[string]any `yaml:"paths"`
	Components struct {
		Schemas map[string]any `yaml:"schemas"`
	} `yaml:"components"`
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
	var root openAPIRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(root.Components.Schemas) == 0 {
		t.Fatalf("%s has no OpenAPI schemas", path)
	}
	return root.Components.Schemas
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
