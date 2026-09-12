package answersheet_test

import (
	"context"
	mongoanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"go.mongodb.org/mongo-driver/bson"
	"testing"
	"time"
)

func TestScopedAnswerListAndCountAgainstMongoReplicaSet(t *testing.T) {
	db := openDurableSubmitMongo(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	repo, err := mongoanswersheet.NewRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	docs := []interface{}{
		bson.M{"domain_id": int64(1), "org_id": int64(1), "testee_id": int64(401), "filler_id": int64(7), "questionnaire_code": "Q1", "filled_at": time.Now()},
		bson.M{"domain_id": int64(2), "org_id": int64(1), "testee_id": int64(402), "filler_id": int64(7), "questionnaire_code": "Q1", "filled_at": time.Now()},
		bson.M{"domain_id": int64(3), "org_id": int64(2), "testee_id": int64(401), "filler_id": int64(7), "questionnaire_code": "Q1", "filled_at": time.Now()},
		bson.M{"domain_id": int64(4), "org_id": int64(1), "testee_id": int64(401), "filler_id": int64(7), "questionnaire_code": "Q2", "filled_at": time.Now()},
	}
	if _, err := db.Collection("answersheets").InsertMany(ctx, docs); err != nil {
		t.Fatal(err)
	}
	reader := mongoanswersheet.NewAnswerSheetReadModel(repo)
	filler := uint64(7)
	for _, tc := range []struct {
		questionnaire string
		filler        *uint64
		ids           []uint64
		want          int64
	}{
		{"", nil, []uint64{401}, 2}, {"Q1", &filler, []uint64{401}, 1}, {"missing", &filler, []uint64{401}, 0}, {"", nil, nil, 0},
	} {
		filter := surveyreadmodel.AnswerSheetFilter{OrgID: 1, QuestionnaireCode: tc.questionnaire, FillerID: tc.filler, RestrictToStoreScope: true, StoreScopedTesteeIDs: tc.ids}
		rows, err := reader.ListAnswerSheets(ctx, filter, surveyreadmodel.PageRequest{Page: 1, PageSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		count, err := reader.CountAnswerSheets(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		if count != tc.want || int64(len(rows)) != tc.want {
			t.Fatalf("list/count mismatch: rows=%d count=%d want=%d", len(rows), count, tc.want)
		}
		for _, row := range rows {
			if row.ID.Uint64() != 1 && row.ID.Uint64() != 4 {
				t.Fatal("scope or company leaked")
			}
		}
	}
}
