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
	assertOpenAPIOperation(t, spec, "/questionnaires/{code}/versions", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models", "post")
	assertOpenAPIOperation(t, spec, "/assessment-models/options", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/definition", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/definition", "put")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/questionnaire", "get")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/questionnaire", "put")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/codes/apply", "post")
	assertOpenAPIOperation(t, spec, "/assessment-models/{code}/preview-report", "post")
	assertOpenAPIOperation(t, spec, "/assessment-releases/{code}/publish", "post")
	assertOpenAPIOperation(t, spec, "/assessment-releases/{code}/unpublish", "post")
	assertOpenAPIOperation(t, spec, "/assessment-releases/{code}/archive", "post")
	assertOpenAPIOperation(t, spec, "/assessment-releases/{code}/versions", "get")
	assertOpenAPIOperation(t, spec, "/norm-tables", "get")
	assertOpenAPIOperation(t, spec, "/norm-tables", "post")
	assertOpenAPIOperation(t, spec, "/norm-tables/{version}", "get")
	assertOpenAPIOperation(t, spec, "/answersheets/admin-submit", "post")
	assertOpenAPIOperation(t, spec, "/evaluations/assessments", "get")
	assertOpenAPIOperation(t, spec, "/plans/{id}/tasks", "get")
	assertOpenAPIOperation(t, spec, "/api/v2/statistics/overview", "get")
	assertOpenAPIOperation(t, spec, "/api/v2/statistics/clinicians", "get")
	assertOpenAPIOperation(t, spec, "/api/v2/statistics/clinicians/{id}", "get")
	assertOpenAPIOperation(t, spec, "/api/v2/statistics/clinicians/me/overview", "get")
	assertOpenAPIOperation(t, spec, "/api/v2/statistics/clinicians/me/entries", "get")
	assertOpenAPIOperation(t, spec, "/api/v2/statistics/clinicians/me/testees-summary", "get")
	assertOpenAPIOperation(t, spec, "/api/v2/statistics/entries", "get")
	assertOpenAPIOperation(t, spec, "/api/v2/statistics/entries/{id}", "get")
	assertOpenAPIOperation(t, spec, "/api/v2/statistics/contents/batch", "post")
	assertOpenAPIOperationAbsent(t, spec, "/api/v1/statistics/overview", "get")
	assertOpenAPIOperation(t, spec, "/api/v2/plans/testees/{testee_id}/enrollments", "get")
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

func TestStatisticsOpenAPIExposesRunModesAndAuditedCacheResume(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/apiserver.yaml")
	for path, method := range map[string]string{
		"/api/v2/statistics/overview":                    "get",
		"/internal/v2/statistics/runs":                   "post",
		"/internal/v2/statistics/runs/{id}/resume-cache": "post",
	} {
		assertOpenAPIOperation(t, spec, path, method)
	}
	resume := spec.Paths["/internal/v2/statistics/runs/{id}/resume-cache"]["post"].(map[string]any)
	if _, ok := resume["requestBody"].(map[string]any); !ok {
		t.Fatal("resume-cache OpenAPI must require an audited request body")
	}

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/apiserver.yaml")
	runRequest := schemas["handler.StatisticsRunRequest"].(map[string]any)["properties"].(map[string]any)
	if _, ok := runRequest["mode"]; !ok {
		t.Fatal("StatisticsRunRequest must expose mode")
	}
	run := schemas["statistics.Run"].(map[string]any)["properties"].(map[string]any)
	for _, property := range []string{"error_code", "error_message", "source_counts", "fact_counts", "result_counts"} {
		if _, ok := run[property]; !ok {
			t.Fatalf("statistics.Run missing %s", property)
		}
	}
	resumeRequest := schemas["handler.StatisticsResumeCacheRequest"].(map[string]any)["properties"].(map[string]any)
	for _, property := range []string{"confirm", "reason"} {
		if _, ok := resumeRequest[property]; !ok {
			t.Fatalf("StatisticsResumeCacheRequest missing %s", property)
		}
	}
}

func TestAdminSubmitOpenAPIExposesOptionalIdempotencyContract(t *testing.T) {
	t.Parallel()

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/apiserver.yaml")
	schema, ok := schemas["request.AdminSubmitAnswerSheetRequest"].(map[string]any)
	if !ok {
		t.Fatal("missing admin-submit request schema")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("admin-submit request schema has no properties")
	}
	idempotency, ok := properties["idempotency_key"].(map[string]any)
	if !ok {
		t.Fatal("admin-submit request schema missing idempotency_key")
	}
	if idempotency["minLength"] != 8 || idempotency["maxLength"] != 128 {
		t.Fatalf("unexpected idempotency_key bounds: %#v", idempotency)
	}
	for _, required := range schema["required"].([]any) {
		if required == "idempotency_key" {
			t.Fatal("idempotency_key must remain optional for existing clients")
		}
	}

	spec := loadOpenAPISpec(t, "../../../../api/rest/apiserver.yaml")
	operation := spec.Paths["/api/v1/answersheets/admin-submit"]["post"].(map[string]any)
	responses := operation["responses"].(map[string]any)
	for _, status := range []string{"400", "409", "503"} {
		if _, ok := responses[status]; !ok {
			t.Fatalf("admin-submit OpenAPI missing %s response", status)
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

func assertOpenAPIOperationAbsent(t *testing.T, spec openAPISpec, path, method string) {
	t.Helper()
	if !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/internal/") {
		path = "/api/v1" + path
	}
	if ops, ok := spec.Paths[path]; ok {
		if _, registered := ops[method]; registered {
			t.Fatalf("OpenAPI unexpectedly contains %s %s", method, path)
		}
	}
}
