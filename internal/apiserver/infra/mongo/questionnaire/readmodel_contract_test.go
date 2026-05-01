package questionnaire

import (
	"testing"
	"time"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"go.mongodb.org/mongo-driver/bson"
)

func TestQuestionnaireHeadReadModelFilterAppliesTypedFilter(t *testing.T) {
	t.Parallel()

	filter := questionnaireHeadReadModelFilter(surveyreadmodel.QuestionnaireFilter{
		Status: "published",
		Title:  "PHQ",
		Type:   "mental",
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

func TestQuestionnairePublishedReadModelFilterDefaultsToPublishedSnapshotSemantics(t *testing.T) {
	t.Parallel()

	filter := questionnairePublishedReadModelFilter(surveyreadmodel.QuestionnaireFilter{Title: "PHQ"})
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

func TestQuestionnaireRowsFromPOMapsSummaryFieldsWithoutDomainAggregate(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	rows := questionnaireRowsFromPO([]QuestionnairePO{
		{
			Code:          "Q_A",
			Version:       "1.0",
			Title:         "Questionnaire A",
			Description:   "desc",
			ImgUrl:        "https://example.test/q.png",
			Status:        "published",
			Type:          "MedicalScale",
			QuestionCount: 7,
		},
	})
	if len(rows) != 1 {
		t.Fatalf("row count = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Code != "Q_A" || row.Version != "1.0" || row.Title != "Questionnaire A" {
		t.Fatalf("unexpected identity fields: %#v", row)
	}
	if row.Description != "desc" || row.ImgURL != "https://example.test/q.png" {
		t.Fatalf("unexpected display fields: %#v", row)
	}
	if row.Status != "published" || row.Type != "MedicalScale" || row.QuestionCount != 7 {
		t.Fatalf("unexpected status fields: %#v", row)
	}

	audited := QuestionnairePO{}
	audited.CreatedBy = 1001
	audited.CreatedAt = now
	audited.UpdatedBy = 1002
	audited.UpdatedAt = now.Add(time.Minute)
	rows = questionnaireRowsFromPO([]QuestionnairePO{audited})
	row = rows[0]
	if row.CreatedBy.Uint64() != 1001 || row.UpdatedBy.Uint64() != 1002 {
		t.Fatalf("audit ids = (%d,%d), want (1001,1002)", row.CreatedBy.Uint64(), row.UpdatedBy.Uint64())
	}
	if !row.CreatedAt.Equal(now) || !row.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("audit times = (%s,%s), want (%s,%s)", row.CreatedAt, row.UpdatedAt, now, now.Add(time.Minute))
	}
}
