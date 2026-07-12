package rest

import (
	"strings"
	"testing"
)

// k6 mixed.js 主压测路径与 OpenAPI 对齐守卫（不含已下线 R121 端点）。
func TestCollectionOpenAPICoversK6PerfPaths(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")
	required := map[string][]string{
		"/answersheets":                            {"post"},
		"/answersheets/submit-status":              {"get"},
		"/assessment-models":                       {"get"},
		"/assessment-models/hot":                   {"get"},
		"/assessment-models/options":               {"get"},
		"/assessment-models/{code}":                {"get"},
		"/typology-models":                         {"get"},
		"/typology-models/categories":              {"get"},
		"/typology-models/{code}":                  {"get"},
		"/questionnaires/{code}":                   {"get"},
		"/typology-assessment-sessions":            {"post"},
		"/typology-assessments":                    {"get"},
		"/typology-assessments/{id}/report-status": {"get"},
		"/typology-assessments/{id}/report":        {"get"},
		"/typology-assessments/{id}/wait-report":   {"get"},
		"/assessments/{id}/report-status":          {"get"},
		"/report-events":                           {"get"},
	}
	for path, methods := range required {
		for _, method := range methods {
			assertOpenAPIOperation(t, spec, path, method)
		}
	}

	reportOps, ok := spec.Paths["/api/v1/report-events"]["get"].(map[string]any)
	if !ok {
		t.Fatal("missing GET /api/v1/report-events")
	}
	desc, _ := reportOps["description"].(string)
	if !strings.Contains(desc, "subscribe") {
		t.Fatalf("report-events description should document subscribe, got %q", desc)
	}
}

func TestCollectionOpenAPISubmitStatusHasAssessmentID(t *testing.T) {
	t.Parallel()

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/collection.yaml")
	schema, ok := schemas["answersheet.SubmitStatusResponse"].(map[string]any)
	if !ok {
		t.Fatal("missing answersheet.SubmitStatusResponse schema")
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("SubmitStatusResponse has no properties")
	}
	if _, ok := props["assessment_id"]; !ok {
		t.Fatal("SubmitStatusResponse missing assessment_id")
	}
}

func TestCollectionOpenAPIHasNoK6RemovedLegacyPaths(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")
	for _, path := range []string{
		"/api/v1/answersheets/{id}/assessment",
		"/api/v1/personality-models",
		"/api/v1/personality-assessment-sessions",
		"/api/v1/personality-assessments",
	} {
		if _, ok := spec.Paths[path]; ok {
			t.Fatalf("legacy path should not be in OpenAPI: %s", path)
		}
	}
}
