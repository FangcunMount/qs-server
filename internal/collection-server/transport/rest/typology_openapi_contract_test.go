package rest

import (
	"strings"
	"testing"
)

func TestCollectionOpenAPITypologyAssessmentContractFields(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")

	reportOps, ok := spec.Paths["/typology-assessments/{id}/report"]["get"].(map[string]any)
	if !ok {
		t.Fatal("missing GET /typology-assessments/{id}/report")
	}
	if !openAPIHasRequiredQueryParam(reportOps, "testee_id") {
		t.Fatal("typology report OpenAPI must require testee_id query param")
	}

	questionnaireOps, ok := spec.Paths["/questionnaires/{code}"]["get"].(map[string]any)
	if !ok {
		t.Fatal("missing GET /questionnaires/{code}")
	}
	if !openAPIHasQueryParam(questionnaireOps, "version") {
		t.Fatal("questionnaire OpenAPI should document version query param")
	}

	sessionOps, ok := spec.Paths["/typology-assessment-sessions"]["post"].(map[string]any)
	if !ok {
		t.Fatal("missing POST /typology-assessment-sessions")
	}
	desc, _ := sessionOps["description"].(string)
	if !strings.Contains(desc, "session") || !strings.Contains(desc, "answersheets") {
		t.Fatalf("session OpenAPI description should document recommended flow, got %q", desc)
	}
}

func TestCollectionOpenAPITypologyModelSchemaHasRoutingFields(t *testing.T) {
	t.Parallel()

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/collection.yaml")
	schema, ok := schemas["typologymodel.TypologyModelSummaryResponse"].(map[string]any)
	if !ok {
		t.Fatal("missing typologymodel.TypologyModelSummaryResponse schema")
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema has no properties")
	}
	for _, field := range []string{"kind", "sub_kind", "product_channel", "algorithm_family", "payload_format", "decision_kind"} {
		if _, ok := props[field]; !ok {
			t.Fatalf("TypologyModelSummaryResponse missing field %q", field)
		}
	}
	kindProp, ok := props["kind"].(map[string]any)
	if !ok {
		t.Fatal("kind property must be an object")
	}
	if example, _ := kindProp["example"].(string); example != "typology" {
		t.Fatalf("kind example = %v, want typology", kindProp["example"])
	}
}

func TestCollectionOpenAPIModelIdentityHasRoutingFields(t *testing.T) {
	t.Parallel()

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/collection.yaml")
	schema, ok := schemas["evaluation.ModelIdentityResponse"].(map[string]any)
	if !ok {
		t.Fatal("missing evaluation.ModelIdentityResponse schema")
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema has no properties")
	}
	for _, field := range []string{"product_channel", "algorithm_family"} {
		if _, ok := props[field]; !ok {
			t.Fatalf("ModelIdentityResponse missing field %q", field)
		}
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
