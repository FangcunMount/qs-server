package evaluation

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestAssessmentSummaryReaderUsesSingleEvaluatedOnlyWindowQuery(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	reader := NewAssessmentSummaryReader(db)
	latest := time.Date(2026, 8, 2, 9, 30, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("FROM assessment FORCE INDEX (idx_assessment_org_testee_evaluated_summary)")+".*assessment.status = 'evaluated'.*assessment.deleted_at IS NULL").
		WithArgs(int64(7), uint64(10), uint64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"testee_id", "total_evaluated", "last_evaluated_at", "risk_level"}).AddRow(10, 3, latest, "high"))

	got, err := reader.ReadAssessmentSummaries(context.Background(), 7, []uint64{10, 11, 10})
	if err != nil {
		t.Fatal(err)
	}
	if got[10].TotalEvaluated != 3 || got[10].LastEvaluatedAt == nil || !got[10].LastEvaluatedAt.Equal(latest) || got[10].RiskLevel != "high" {
		t.Fatalf("summary=%+v", got[10])
	}
	if _, exists := got[11]; exists {
		t.Fatal("testee without evaluated assessment must be absent")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
