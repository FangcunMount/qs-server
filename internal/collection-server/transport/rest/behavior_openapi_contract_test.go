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
}
