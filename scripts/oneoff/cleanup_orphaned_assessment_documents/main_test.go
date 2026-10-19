package main

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.mongodb.org/mongo-driver/bson"
)

func validConfig() config {
	return config{mongoURI: "mongodb://localhost", mongoDB: "qs", mysqlDSN: "dsn", source: "answersheets", batchSize: 1000, workers: 8, answerSheetCreatedBefore: time.Now().Add(-24 * time.Hour)}
}

func TestValidateConfigRequiresAnswerSheetCutoff(t *testing.T) {
	c := validConfig()
	c.source = "answersheets"
	c.answerSheetCreatedBefore = time.Time{}
	if err := validateConfig(c); err == nil {
		t.Fatal("expected answersheet cutoff validation error")
	}
	c.answerSheetCreatedBefore = time.Now().Add(-24 * time.Hour)
	if err := validateConfig(c); err != nil {
		t.Fatalf("valid config: %v", err)
	}
}

func TestPhaseUsesAssessmentOwnershipKey(t *testing.T) {
	if got := answerSheetPhase().lookupSQL(2); got != "SELECT answer_sheet_id FROM assessment WHERE answer_sheet_id IN (?,?)" {
		t.Fatalf("answersheet lookup = %q", got)
	}
}

func TestAnswerSheetFilterIncludesSafetyCutoff(t *testing.T) {
	cutoff := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	c := validConfig()
	c.answerSheetCreatedBefore = cutoff
	want := bson.M{"domain_id": bson.M{"$gt": uint64(9)}, "deleted_at": nil, "created_at": bson.M{"$lt": cutoff}}
	if got := answerSheetPhase().filter(c, 9); !reflect.DeepEqual(got, want) {
		t.Fatalf("filter = %#v, want %#v", got, want)
	}
}

func TestProcessPageReportsOnlyMissingAssessmentReferences(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT answer_sheet_id FROM assessment").WithArgs(uint64(1), uint64(2)).WillReturnRows(sqlmock.NewRows([]string{"answer_sheet_id"}).AddRow(1))
	got, err := processPage(t.Context(), db, answerSheetPhase(), []bson.M{{"domain_id": int64(1)}, {"domain_id": int64(2)}})
	if err != nil || got.scanned != 2 || got.candidates != 1 || got.failed != 0 {
		t.Fatalf("audit=%+v err=%v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestProcessPageDoesNotReportCandidatesAfterReadFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	failure := errors.New("ownership query interrupted")
	mock.ExpectQuery("SELECT answer_sheet_id FROM assessment").WithArgs(uint64(1)).WillReturnRows(sqlmock.NewRows([]string{"answer_sheet_id"}).AddRow(1).RowError(0, failure))
	got, err := processPage(t.Context(), db, answerSheetPhase(), []bson.M{{"domain_id": int64(1)}})
	if !errors.Is(err, failure) || got.candidates != 0 || got.failed != 1 {
		t.Fatalf("audit=%+v err=%v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
