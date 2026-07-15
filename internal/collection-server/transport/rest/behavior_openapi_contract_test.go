package rest

import "testing"

func TestCollectionOpenAPIBehaviorAssessmentContractFields(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t, "../../../../api/rest/collection.yaml")
	for _, path := range []string{
		"/api/v1/behavior-assessments/{id}",
		"/api/v1/behavior-assessments/{id}/report",
		"/api/v1/behavior-assessments/{id}/report-status",
		"/api/v1/behavior-assessments/{id}/wait-report",
	} {
		op, ok := spec.Paths[path]["get"].(map[string]any)
		if !ok {
			t.Fatalf("missing GET %s", path)
		}
		if !openAPIHasRequiredQueryParam(op, "testee_id") {
			t.Fatalf("%s must require testee_id", path)
		}
	}

	schemas := loadOpenAPIComponents(t, "../../../../api/rest/collection.yaml")
	dimension, ok := schemas["evaluation.DimensionInterpretResponse"].(map[string]any)
	if !ok {
		t.Fatal("missing evaluation.DimensionInterpretResponse schema")
	}
	properties, ok := dimension["properties"].(map[string]any)
	if !ok {
		t.Fatal("dimension response is missing properties")
	}
	for _, field := range []string{"derived_scores", "level", "norm_reference"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("behavior report dimension is missing %s", field)
		}
	}
	normReference, ok := schemas["evaluation.NormReferenceResponse"].(map[string]any)
	if !ok {
		t.Fatal("missing evaluation.NormReferenceResponse schema")
	}
	normProperties, ok := normReference["properties"].(map[string]any)
	if !ok {
		t.Fatal("norm reference response is missing properties")
	}
	for _, field := range []string{"score_kind", "benchmark", "table_version", "form_variant", "min_age_months", "max_age_months", "gender"} {
		if _, ok := normProperties[field]; !ok {
			t.Fatalf("norm reference is missing %s", field)
		}
	}
}
