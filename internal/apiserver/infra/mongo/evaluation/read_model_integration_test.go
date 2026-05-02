package evaluation

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func openEvaluationMongoContractDB(t *testing.T) *mongo.Database {
	t.Helper()

	uri := os.Getenv("QS_SERVER_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("set QS_SERVER_TEST_MONGO_URI to run Mongo evaluation read model contract tests")
	}
	dbName := os.Getenv("QS_SERVER_TEST_MONGO_DB")
	if dbName == "" {
		dbName = "qs_server_contract_test"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect mongo test db: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Fatalf("ping mongo test db: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Disconnect(context.Background())
	})

	return client.Database(dbName)
}

func TestReportReadModelListReportsFiltersAgainstMongo(t *testing.T) {
	db := openEvaluationMongoContractDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	baseID := uint64(time.Now().UnixNano() / int64(time.Millisecond))
	testeeID := baseID + 1000
	scaleCode := fmt.Sprintf("scale-report-%d", baseID)
	otherScaleCode := fmt.Sprintf("scale-other-%d", baseID)
	now := time.Now().UTC().Truncate(time.Second)

	ids := []primitive.ObjectID{
		primitive.NewObjectID(),
		primitive.NewObjectID(),
		primitive.NewObjectID(),
		primitive.NewObjectID(),
	}
	docs := []interface{}{
		InterpretReportPO{
			BaseDocument: base.BaseDocument{
				ID:        ids[0],
				DomainID:  meta.FromUint64(baseID + 1),
				CreatedAt: now.Add(-3 * time.Minute),
				UpdatedAt: now.Add(-3 * time.Minute),
			},
			ScaleName:  "抑郁自评",
			ScaleCode:  scaleCode,
			TesteeID:   testeeID,
			TotalScore: 90,
			RiskLevel:  "high",
			Conclusion: "高风险",
		},
		InterpretReportPO{
			BaseDocument: base.BaseDocument{
				ID:        ids[1],
				DomainID:  meta.FromUint64(baseID + 2),
				CreatedAt: now.Add(-1 * time.Minute),
				UpdatedAt: now.Add(-1 * time.Minute),
			},
			ScaleName:  "抑郁自评",
			ScaleCode:  scaleCode,
			TesteeID:   testeeID,
			TotalScore: 55,
			RiskLevel:  "medium",
			Conclusion: "中风险",
		},
		InterpretReportPO{
			BaseDocument: base.BaseDocument{
				ID:        ids[2],
				DomainID:  meta.FromUint64(baseID + 3),
				CreatedAt: now.Add(-2 * time.Minute),
				UpdatedAt: now.Add(-2 * time.Minute),
			},
			ScaleName:  "焦虑自评",
			ScaleCode:  otherScaleCode,
			TesteeID:   testeeID,
			TotalScore: 95,
			RiskLevel:  "severe",
			Conclusion: "严重风险",
		},
		InterpretReportPO{
			BaseDocument: base.BaseDocument{
				ID:        ids[3],
				DomainID:  meta.FromUint64(baseID + 4),
				CreatedAt: now,
				UpdatedAt: now,
			},
			ScaleName:  "抑郁自评",
			ScaleCode:  scaleCode,
			TesteeID:   testeeID + 1,
			TotalScore: 91,
			RiskLevel:  "high",
			Conclusion: "其他受试者",
		},
	}

	collection := db.Collection((&InterpretReportPO{}).CollectionName())
	if _, err := collection.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert reports: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = collection.DeleteMany(cleanupCtx, bson.M{"_id": bson.M{"$in": ids}})
	})

	reader := NewReportReadModel(db)
	highRows, total, err := reader.ListReports(ctx, evaluationreadmodel.ReportFilter{
		TesteeID:     &testeeID,
		HighRiskOnly: true,
		ScaleCode:    scaleCode,
	}, evaluationreadmodel.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list high risk reports: %v", err)
	}
	if total != 1 || len(highRows) != 1 || highRows[0].AssessmentID != baseID+1 {
		t.Fatalf("high risk filtered rows = %#v total=%d, want report %d", highRows, total, baseID+1)
	}

	rows, total, err := reader.ListReports(ctx, evaluationreadmodel.ReportFilter{
		TesteeID: &testeeID,
	}, evaluationreadmodel.PageRequest{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("list reports by testee: %v", err)
	}
	if total != 3 || len(rows) != 2 {
		t.Fatalf("paged rows = %#v total=%d, want 2 of 3", rows, total)
	}
	if rows[0].AssessmentID != baseID+2 || rows[1].AssessmentID != baseID+3 {
		t.Fatalf("rows order = %#v, want created_at desc", rows)
	}

	reportRow, err := reader.GetReportByAssessmentID(ctx, baseID+1)
	if err != nil {
		t.Fatalf("get report by assessment id: %v", err)
	}
	if reportRow.ScaleCode != scaleCode || reportRow.RiskLevel != "high" || reportRow.AssessmentID != baseID+1 {
		t.Fatalf("unexpected report row: %#v", reportRow)
	}
}
