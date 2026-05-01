package questionnaire

import (
	"reflect"
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

func TestQuestionnaireHeadReadModelPipelineAppliesSortPaginationAndProjection(t *testing.T) {
	t.Parallel()

	pipeline := questionnaireHeadReadModelPipeline(surveyreadmodel.QuestionnaireFilter{Status: "draft"}, surveyreadmodel.PageRequest{Page: 2, PageSize: 10})
	if len(pipeline) != 5 {
		t.Fatalf("pipeline length = %d, want 5: %#v", len(pipeline), pipeline)
	}
	if !reflect.DeepEqual(pipeline[1], bson.M{"$sort": bson.M{"updated_at": -1}}) {
		t.Fatalf("sort stage = %#v, want updated_at desc", pipeline[1])
	}
	if !reflect.DeepEqual(pipeline[2], bson.M{"$skip": int64(10)}) {
		t.Fatalf("skip stage = %#v, want 10", pipeline[2])
	}
	if !reflect.DeepEqual(pipeline[3], bson.M{"$limit": int64(10)}) {
		t.Fatalf("limit stage = %#v, want 10", pipeline[3])
	}
	project, ok := pipeline[4]["$project"].(bson.M)
	if !ok {
		t.Fatalf("project stage = %#v, want bson.M", pipeline[4])
	}
	for _, field := range []string{"code", "title", "description", "img_url", "version", "status", "type", "question_count", "created_by", "updated_by"} {
		if project[field] != 1 {
			t.Fatalf("project[%s] = %#v, want 1", field, project[field])
		}
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

func TestQuestionnairePublishedReadModelPipelineGroupsByCodeBeforePaging(t *testing.T) {
	t.Parallel()

	pipeline := questionnairePublishedReadModelPipeline(surveyreadmodel.QuestionnaireFilter{Type: "MedicalScale"}, surveyreadmodel.PageRequest{Page: 3, PageSize: 5})
	wantStages := []string{"$match", "$addFields", "$sort", "$group", "$replaceRoot", "$sort", "$skip", "$limit", "$project"}
	if len(pipeline) != len(wantStages) {
		t.Fatalf("pipeline length = %d, want %d: %#v", len(pipeline), len(wantStages), pipeline)
	}
	for i, stageKey := range wantStages {
		if _, ok := pipeline[i][stageKey]; !ok {
			t.Fatalf("stage %d = %#v, want %s", i, pipeline[i], stageKey)
		}
	}
	if !reflect.DeepEqual(pipeline[6], bson.M{"$skip": int64(10)}) {
		t.Fatalf("skip stage = %#v, want 10", pipeline[6])
	}
	if !reflect.DeepEqual(pipeline[7], bson.M{"$limit": int64(5)}) {
		t.Fatalf("limit stage = %#v, want 5", pipeline[7])
	}
}

func TestQuestionnairePublishedReadModelCountPipelineCountsUniqueCodes(t *testing.T) {
	t.Parallel()

	pipeline := questionnairePublishedReadModelCountPipeline(surveyreadmodel.QuestionnaireFilter{Title: "PHQ"})
	wantStages := []string{"$match", "$addFields", "$sort", "$group", "$count"}
	if len(pipeline) != len(wantStages) {
		t.Fatalf("pipeline length = %d, want %d: %#v", len(pipeline), len(wantStages), pipeline)
	}
	for i, stageKey := range wantStages {
		if _, ok := pipeline[i][stageKey]; !ok {
			t.Fatalf("stage %d = %#v, want %s", i, pipeline[i], stageKey)
		}
	}
	group, ok := pipeline[3]["$group"].(bson.M)
	if !ok || group["_id"] != "$code" {
		t.Fatalf("group stage = %#v, want group by code", pipeline[3])
	}
	if pipeline[4]["$count"] != "total" {
		t.Fatalf("count stage = %#v, want total", pipeline[4])
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
