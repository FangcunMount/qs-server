//go:build integration && reliable_messaging_m4

package answersheetgap

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// This verifies the actual Mongo standard row and MySQL effect lookup, not
// merely the pure payload classifier. The databases must be disposable.
func TestScanPageFindsPublishedAnswerSheetMissingAssessment(t *testing.T) {
	mongoURI, mysqlDSN := os.Getenv("RM_QS03_MONGO_URI"), os.Getenv("RM_QS03_MYSQL_DSN")
	if !strings.Contains(mongoURI, "directConnection=true") || !strings.Contains(mysqlDSN, "/m6_qs03_gap?") {
		t.Skip("disposable QS-03 Mongo and MySQL fixtures are required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatal(err)
	}
	defer mongoClient.Disconnect(context.Background())
	if err := mongoClient.Ping(ctx, nil); err != nil {
		t.Fatal(err)
	}
	mongoDB := mongoClient.Database("rm_qs03_gap_test")
	defer mongoDB.Drop(context.Background())
	mysqlDB, err := gorm.Open(mysql.Open(mysqlDSN), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := mysqlDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := mysqlDB.Exec("CREATE TABLE assessment (id BIGINT UNSIGNED PRIMARY KEY, org_id BIGINT UNSIGNED NOT NULL, answer_sheet_id BIGINT UNSIGNED NOT NULL UNIQUE, deleted_at DATETIME(3) NULL)").Error; err != nil {
		t.Fatal(err)
	}
	defer mysqlDB.Exec("DROP TABLE assessment")
	sheet, row := validOriginal(t)
	acceptedAt := time.Now().Add(-time.Minute).UTC()
	if _, err := mongoDB.Collection("answersheets").InsertOne(ctx, bson.M{
		"domain_id": sheet.DomainID, "org_id": sheet.OrgID, "testee_id": sheet.TesteeID,
		"filler_id": sheet.FillerID, "questionnaire_code": sheet.QuestionnaireCode,
		"questionnaire_version": sheet.QuestionnaireVer, "task_id": sheet.TaskID,
		"admission": bson.M{"purpose": sheet.Admission.Purpose, "model_kind": sheet.Admission.ModelKind,
			"model_code": sheet.Admission.ModelCode, "model_version": sheet.Admission.ModelVersion},
		"durable_acceptance": bson.M{"schema_version": 1, "event_id": sheet.DurableAcceptance.EventID, "accepted_at": acceptedAt},
		"deleted_at":         nil,
	}); err != nil {
		t.Fatal(err)
	}
	id := bson.D{{Key: "producer", Value: row.Producer}, {Key: "message_id", Value: row.MessageID}, {Key: "destination", Value: row.Destination}}
	if _, err := mongoDB.Collection("rm_outbox").InsertOne(ctx, bson.M{
		"_id": id, "producer": row.Producer, "message_id": row.MessageID,
		"destination": row.Destination, "event_type": row.EventType,
		"schema_version": row.SchemaVersion, "scope": row.Scope,
		"content_type": row.ContentType, "occurred_at": row.OccurredAt,
		"payload": row.Payload, "fingerprint": row.Fingerprint, "state": row.State,
	}); err != nil {
		t.Fatal(err)
	}
	scanner, err := New(mongoDB, mysqlDB)
	if err != nil {
		t.Fatal(err)
	}
	page, err := scanner.ScanPage(ctx, 0, sheet.DomainID, time.Now().Add(-10*time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Findings) != 1 || page.Findings[0].Disposition != Missing || page.Findings[0].EventID != sheet.DurableAcceptance.EventID || !page.Exhausted {
		t.Fatalf("published original with no Assessment should be missing: %+v", page)
	}
	if err := mysqlDB.Exec("INSERT INTO assessment(id,org_id,answer_sheet_id) VALUES (?,?,?)", uint64(801), sheet.OrgID, sheet.DomainID).Error; err != nil {
		t.Fatal(err)
	}
	page, err = scanner.ScanPage(ctx, 0, sheet.DomainID, time.Now().Add(-10*time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Findings) != 1 || page.Findings[0].Disposition != Present || page.Findings[0].AssessmentID != 801 {
		t.Fatalf("persisted Assessment should close the original gap: %+v", page)
	}
	if err := mysqlDB.Exec("DELETE FROM assessment WHERE answer_sheet_id=?", sheet.DomainID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := mongoDB.Collection("rm_outbox").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"state": "pending"}}); err != nil {
		t.Fatal(err)
	}
	page, err = scanner.ScanPage(ctx, 0, sheet.DomainID, time.Now().Add(-10*time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Findings) != 1 || page.Findings[0].Disposition != DeliveryPending {
		t.Fatalf("unconfirmed original delivery must not become a confirmed gap: %+v", page)
	}
}
