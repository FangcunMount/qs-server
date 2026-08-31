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
	assertOpenAPIOperation(t, spec, "/clinicians", "get")
	assertOpenAPIOperation(t, spec, "/clinicians/me", "get")
	assertOpenAPIOperationAbsent(t, spec, "/practitioners", "get")
	assertOpenAPIOperationAbsent(t, spec, "/practitioners/me", "get")
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

func TestAIExplanationAdministrationOpenAPIContract(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/apiserver.yaml")
	for path, method := range map[string]string{
		"/internal/v1/interpretation/ai-explanation/prompt-evaluation-capacity":                                               "get",
		"/internal/v1/interpretation/ai-explanation/prompt-evaluations":                                                       "get",
		"/internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}":                                              "get",
		"/internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/attempts/{case_id}/{attempt}":                 "get",
		"/internal/v2/interpretation/ai-explanation/prompt-evaluations":                                                       "post",
		"/internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}":                                              "get",
		"/internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/executions/{execution_id}/output":             "get",
		"/internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/reviews":                                      "post",
		"/internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/finalize":                                     "post",
		"/internal/v2/interpretation/ai-explanation/prompt-evaluations/{run_id}/result-unknown/resolve":                       "post",
		"/internal/v2/interpretation/ai-explanation/legacy-prompt-evaluations/{run_id}/attempts/{case_id}/{attempt}/rechecks": "post",
		"/internal/v1/interpretation/ai-explanation/profiles":                                                                 "post",
		"/internal/v1/interpretation/ai-explanation/profiles/{profile_id}/versions/{version}":                                 "get",
		"/internal/v1/interpretation/ai-explanation/profiles/{profile_id}/versions/{version}/publish":                         "post",
		"/internal/v1/interpretation/ai-explanation/profiles/{profile_id}/versions/{version}/disable":                         "post",
	} {
		assertOpenAPIOperation(t, spec, path, method)
		operation := spec.Paths[path][method].(map[string]any)
		if security, explicitlyPublic := operation["security"].([]any); explicitlyPublic && len(security) == 0 {
			t.Fatalf("AI explanation administration operation must not override root authentication: %s %s", method, path)
		}
	}
	recheckPath := "/internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/attempts/{case_id}/{attempt}/rechecks"
	assertOpenAPIOperation(t, spec, recheckPath, "get")
	if _, exists := spec.Paths[recheckPath]["post"]; exists {
		t.Fatal("v1 Prompt evaluation Recheck must be read-only")
	}
	assertOpenAPIOperation(t, spec, recheckPath+"/{recheck_id}", "get")
	if _, exists := spec.Paths["/internal/v1/interpretation/ai-explanation/prompt-evaluations"]["post"]; exists {
		t.Fatal("v1 Prompt evaluation catalog must be read-only")
	}
	for _, path := range []string{
		"/internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/recover",
		"/internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/cancel",
		"/internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/reviews",
		"/internal/v1/interpretation/ai-explanation/prompt-evaluations/{run_id}/finalize",
	} {
		if _, exists := spec.Paths[path]; exists {
			t.Fatalf("legacy Prompt evaluation write path must be absent: %s", path)
		}
	}
	start := spec.Paths["/internal/v2/interpretation/ai-explanation/prompt-evaluations"]["post"].(map[string]any)
	responses := start["responses"].(map[string]any)
	for _, status := range []string{"400", "409", "429"} {
		if _, ok := responses[status]; !ok {
			t.Fatalf("Prompt evaluation start OpenAPI missing %s response", status)
		}
	}
	capacityOperation := spec.Paths["/internal/v1/interpretation/ai-explanation/prompt-evaluation-capacity"]["get"].(map[string]any)
	if _, ok := capacityOperation["responses"].(map[string]any)["501"]; !ok {
		t.Fatal("Prompt evaluation capacity OpenAPI missing disabled response")
	}

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/apiserver.yaml")
	capacity := openAPISchemaProperties(t, schemas, "handler.AIExplanationEvaluationCapacityWire")
	for _, property := range []string{"organization_id", "budget_day", "max_active_runs_per_org", "provider_invocations_per_start", "daily_provider_invocation_limit", "reserved_provider_invocations", "remaining_provider_invocations", "available_full_run_starts", "over_limit", "reservations"} {
		if _, ok := capacity[property]; !ok {
			t.Fatalf("evaluation capacity missing property %q", property)
		}
	}
	summary := openAPISchemaProperties(t, schemas, "handler.AIExplanationEvaluationRunWire")
	for _, property := range []string{"run_id", "requested_by", "request_reason", "execution", "recoveries", "recovery_max_provider_invocations", "canceled", "release", "attempts", "progress", "gate", "can_review", "can_finalize"} {
		if _, ok := summary[property]; !ok {
			t.Fatalf("evaluation summary missing property %q", property)
		}
	}
	catalogSummary := openAPISchemaProperties(t, schemas, "handler.AIExplanationEvaluationSummaryWire")
	for _, property := range []string{"run_id", "status", "requested_org_id", "release", "progress", "gate", "can_review", "can_finalize"} {
		if _, ok := catalogSummary[property]; !ok {
			t.Fatalf("evaluation catalog summary missing property %q", property)
		}
	}
	for _, forbidden := range []string{"attempts", "execution", "recoveries", "assessment_input", "raw_provider_output"} {
		if _, ok := catalogSummary[forbidden]; ok {
			t.Fatalf("evaluation catalog summary must not expose %q", forbidden)
		}
	}
	evaluationPage := openAPISchemaProperties(t, schemas, "handler.AIExplanationEvaluationPageWire")
	for _, property := range []string{"items", "next_cursor"} {
		if _, ok := evaluationPage[property]; !ok {
			t.Fatalf("evaluation page missing property %q", property)
		}
	}
	v2Evidence := openAPISchemaProperties(t, schemas, "handler.AIExplanationEvaluationV2Wire")
	for _, property := range []string{"schema_version", "run_id", "status", "release_fingerprint", "reserved_provider_invocations", "required_candidates", "accepted_candidates", "review_ready_candidates", "unresolved_result_unknown_count", "slots", "generation_executions", "semantic_executions", "human_reviews", "gate"} {
		if _, ok := v2Evidence[property]; !ok {
			t.Fatalf("v2 evaluation evidence missing property %q", property)
		}
	}
	profilePage := openAPISchemaProperties(t, schemas, "handler.AIExplanationProfilePageWire")
	for _, property := range []string{"items", "next_cursor"} {
		if _, ok := profilePage[property]; !ok {
			t.Fatalf("Profile page missing property %q", property)
		}
	}

	attemptSummary := openAPISchemaProperties(t, schemas, "handler.AIExplanationReviewAttemptSummary")
	for _, forbidden := range []string{"assessment_input", "normalized_output", "raw_provider_output", "provider_receipt", "semantic"} {
		if _, ok := attemptSummary[forbidden]; ok {
			t.Fatalf("evaluation summary must not expose raw evidence property %q", forbidden)
		}
	}

	attemptDetail := openAPISchemaProperties(t, schemas, "handler.AIExplanationReviewAttemptWire")
	for _, property := range []string{"assessment_input", "normalized_output", "raw_provider_output", "provider_receipt", "semantic", "semantic_execution", "assertions"} {
		if _, ok := attemptDetail[property]; !ok {
			t.Fatalf("attempt evidence detail missing property %q", property)
		}
	}
	recheck := openAPISchemaProperties(t, schemas, "handler.AIExplanationAttemptRecheckWire")
	for _, property := range []string{"recheck_id", "source_run_id", "source_case_id", "source_attempt", "status", "release", "result", "reason"} {
		if _, ok := recheck[property]; !ok {
			t.Fatalf("attempt recheck evidence missing property %q", property)
		}
	}

	draft := openAPISchemaProperties(t, schemas, "handler.AIExplanationProfileDraftRequest")
	for _, property := range []string{"definition", "fingerprint", "reason"} {
		if _, ok := draft[property]; !ok {
			t.Fatalf("profile draft request missing property %q", property)
		}
	}
	if _, ok := draft["actor"]; ok {
		t.Fatal("profile draft request must derive actor from authentication instead of request body")
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

func openAPISchemaProperties(t *testing.T, schemas map[string]any, name string) map[string]any {
	t.Helper()
	schema, ok := schemas[name].(map[string]any)
	if !ok {
		t.Fatalf("missing OpenAPI schema %s", name)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI schema %s has no properties", name)
	}
	return properties
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
