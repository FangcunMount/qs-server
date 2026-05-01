package questionnaire

import (
	"testing"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"go.mongodb.org/mongo-driver/bson"
)

func TestQuestionnaireFilterToConditionsMapsTypedFilter(t *testing.T) {
	t.Parallel()

	got := questionnaireFilterToConditions(surveyreadmodel.QuestionnaireFilter{
		Status: "published",
		Title:  "PHQ",
		Type:   "mental",
	})

	if got["status"] != "published" {
		t.Fatalf("status = %#v, want published", got["status"])
	}
	if got["title"] != "PHQ" {
		t.Fatalf("title = %#v, want PHQ", got["title"])
	}
	if got["type"] != "mental" {
		t.Fatalf("type = %#v, want mental", got["type"])
	}
}

func TestBuildHeadListFilterAppliesCommonConditions(t *testing.T) {
	t.Parallel()

	filter := buildHeadListFilter(map[string]interface{}{
		"status": "published",
		"title":  "PHQ",
		"type":   "mental",
	})

	if got := filter["deleted_at"]; got != nil {
		t.Fatalf("deleted_at = %#v, want nil", got)
	}
	if got := filter["status"]; got != domainQuestionnaire.STATUS_PUBLISHED.String() {
		t.Fatalf("status = %#v, want published", got)
	}
	if got := filter["type"]; got != "mental" {
		t.Fatalf("type = %#v, want mental", got)
	}
	titleQuery, ok := filter["title"].(bson.M)
	if !ok {
		t.Fatalf("title query = %#v, want bson.M", filter["title"])
	}
	if titleQuery["$regex"] != "PHQ" || titleQuery["$options"] != "i" {
		t.Fatalf("title query = %#v, want case-insensitive regex PHQ", titleQuery)
	}
	if _, ok := filter["$or"]; !ok {
		t.Fatalf("expected head role candidates in filter: %#v", filter)
	}
}

func TestBuildPublishedListFilterDefaultsToPublishedSnapshotSemantics(t *testing.T) {
	t.Parallel()

	filter := buildPublishedListFilter(map[string]interface{}{"title": "PHQ"})
	if _, ok := filter["status"]; ok {
		t.Fatalf("top-level status should be folded into published $or, got %#v", filter["status"])
	}

	branches, ok := filter["$or"].(bson.A)
	if !ok {
		t.Fatalf("$or = %#v, want bson.A", filter["$or"])
	}
	if len(branches) != 2 {
		t.Fatalf("$or branch count = %d, want 2", len(branches))
	}
	snapshotBranch, ok := branches[0].(bson.M)
	if !ok {
		t.Fatalf("snapshot branch = %#v, want bson.M", branches[0])
	}
	if snapshotBranch["record_role"] != domainQuestionnaire.RecordRolePublishedSnapshot.String() {
		t.Fatalf("record_role = %#v, want published snapshot", snapshotBranch["record_role"])
	}
	if snapshotBranch["is_active_published"] != true {
		t.Fatalf("is_active_published = %#v, want true", snapshotBranch["is_active_published"])
	}
	if snapshotBranch["status"] != domainQuestionnaire.STATUS_PUBLISHED.String() {
		t.Fatalf("status = %#v, want published", snapshotBranch["status"])
	}
}
