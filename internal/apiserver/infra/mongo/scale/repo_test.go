package scale

import (
	"testing"

	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	"go.mongodb.org/mongo-driver/bson"
)

func TestScaleVersionCompatibilityFilterMatchesScaleVersionOrLegacyQuestionnaireVersion(t *testing.T) {
	t.Parallel()

	filter := scaleVersionCompatibilityFilter("2.0.0")
	if len(filter) != 2 {
		t.Fatalf("filter len = %d, want 2", len(filter))
	}
	exact, ok := filter[0].(bson.M)
	if !ok || exact["scale_version"] != "2.0.0" {
		t.Fatalf("first branch = %#v, want exact scale_version", filter[0])
	}
	legacyBranch, ok := filter[1].(bson.M)
	if !ok {
		t.Fatalf("legacy branch = %#v, want bson.M", filter[1])
	}
	legacy, ok := legacyBranch["$or"].(bson.A)
	if !ok || len(legacy) != 3 {
		t.Fatalf("legacy branch = %#v, want three scale_version missing/empty variants", filter[1])
	}
	if legacyBranch["questionnaire_version"] != "2.0.0" {
		t.Fatalf("legacy questionnaire_version = %#v, want 2.0.0", legacyBranch["questionnaire_version"])
	}
}

func TestPublishedQuestionnaireRefFilterMatchesPublishedSnapshotVersion(t *testing.T) {
	t.Parallel()

	filter := publishedQuestionnaireRefFilter("Q-SDS", "2.0.0")
	if filter["questionnaire_code"] != "Q-SDS" {
		t.Fatalf("questionnaire_code = %#v, want Q-SDS", filter["questionnaire_code"])
	}
	if filter["questionnaire_version"] != "2.0.0" {
		t.Fatalf("questionnaire_version = %#v, want 2.0.0", filter["questionnaire_version"])
	}
	if filter["record_role"] != scaledefinition.RecordRolePublishedSnapshot.String() {
		t.Fatalf("record_role = %#v, want published_snapshot", filter["record_role"])
	}
	if filter["deleted_at"] != nil {
		t.Fatalf("deleted_at = %#v, want nil", filter["deleted_at"])
	}
	if _, ok := filter["$or"]; ok {
		t.Fatalf("questionnaire ref filter must not use compatibility $or: %#v", filter)
	}
}
