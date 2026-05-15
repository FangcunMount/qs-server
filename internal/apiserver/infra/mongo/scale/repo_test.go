package scale

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestScaleVersionCompatibilityFilterMatchesScaleVersionOrLegacyQuestionnaireVersion(t *testing.T) {
	t.Parallel()

	filter := scaleVersionCompatibilityFilter("2.0.0")
	if len(filter) != 2 {
		t.Fatalf("filter len = %d, want 2", len(filter))
	}
	if filter[0]["scale_version"] != "2.0.0" {
		t.Fatalf("first branch = %#v, want exact scale_version", filter[0])
	}
	legacy, ok := filter[1]["$or"].([]bson.M)
	if !ok || len(legacy) != 3 {
		t.Fatalf("legacy branch = %#v, want three scale_version missing/empty variants", filter[1])
	}
	if filter[1]["questionnaire_version"] != "2.0.0" {
		t.Fatalf("legacy questionnaire_version = %#v, want 2.0.0", filter[1]["questionnaire_version"])
	}
}
