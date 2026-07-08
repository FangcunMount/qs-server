package rest

import (
	"strings"
	"testing"
)

func TestApiserverOpenAPIAssessmentModelKindSemantics(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/apiserver.yaml")
	listOps, ok := spec.Paths["/assessment-models"]["get"].(map[string]any)
	if !ok {
		t.Fatal("missing GET /assessment-models")
	}
	desc, _ := listOps["description"].(string)
	if !strings.Contains(desc, "typology") {
		t.Fatalf("list description should document typology canonical kind, got %q", desc)
	}

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/apiserver.yaml")
	summary, ok := schemas["modelcatalog.ModelSummary"].(map[string]any)
	if !ok {
		t.Fatal("missing modelcatalog.ModelSummary schema")
	}
	props, ok := summary["properties"].(map[string]any)
	if !ok {
		t.Fatal("ModelSummary has no properties")
	}
	kindProp, ok := props["kind"].(map[string]any)
	if !ok {
		t.Fatal("kind property must be an object")
	}
	if example, _ := kindProp["example"].(string); example != "typology" {
		t.Fatalf("ModelSummary.kind example = %v, want typology", kindProp["example"])
	}

	identity, ok := schemas["response.ModelIdentityResponse"].(map[string]any)
	if !ok {
		t.Fatal("missing response.ModelIdentityResponse schema")
	}
	idProps, ok := identity["properties"].(map[string]any)
	if !ok {
		t.Fatal("ModelIdentityResponse has no properties")
	}
	idKind, ok := idProps["kind"].(map[string]any)
	if !ok {
		t.Fatal("ModelIdentityResponse.kind must be an object")
	}
	if example, _ := idKind["example"].(string); example != "personality" {
		t.Fatalf("ModelIdentityResponse.kind example = %v, want personality", idKind["example"])
	}
}
