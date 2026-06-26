package rest

import (
	"strings"
	"testing"
)

func TestCollectionOpenAPIPersonalityAssessmentContractFields(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")

	reportOps, ok := spec.Paths["/personality-assessments/{id}/report"]["get"].(map[string]any)
	if !ok {
		t.Fatal("missing GET /personality-assessments/{id}/report")
	}
	if !openAPIHasRequiredQueryParam(reportOps, "testee_id") {
		t.Fatal("personality report OpenAPI must require testee_id query param")
	}

	questionnaireOps, ok := spec.Paths["/questionnaires/{code}"]["get"].(map[string]any)
	if !ok {
		t.Fatal("missing GET /questionnaires/{code}")
	}
	if !openAPIHasQueryParam(questionnaireOps, "version") {
		t.Fatal("questionnaire OpenAPI should document version query param")
	}

	sessionOps, ok := spec.Paths["/personality-assessment-sessions"]["post"].(map[string]any)
	if !ok {
		t.Fatal("missing POST /personality-assessment-sessions")
	}
	desc, _ := sessionOps["description"].(string)
	if !strings.Contains(desc, "session") || !strings.Contains(desc, "answersheets") {
		t.Fatalf("session OpenAPI description should document recommended flow, got %q", desc)
	}
}

func openAPIHasRequiredQueryParam(op map[string]any, name string) bool {
	params, ok := op["parameters"].([]any)
	if !ok {
		return false
	}
	for _, raw := range params {
		param, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if param["in"] == "query" && param["name"] == name && param["required"] == true {
			return true
		}
	}
	return false
}

func openAPIHasQueryParam(op map[string]any, name string) bool {
	params, ok := op["parameters"].([]any)
	if !ok {
		return false
	}
	for _, raw := range params {
		param, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if param["in"] == "query" && param["name"] == name {
			return true
		}
	}
	return false
}
