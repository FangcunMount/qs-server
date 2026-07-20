package main

import (
	"database/sql/driver"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBackfillReportInputDryRunCountsUpgradeCandidate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	columns := []string{"id", "report_input_json", "model_kind", "model_sub_kind", "model_algorithm", "model_code", "model_version", "model_title", "algorithm_family"}
	reportInput := `{"schema_version":2,"InterpretationAssets":{"Outcomes":[{"OutcomeCode":"low","Summary":"偏低"}]},"payload":{"Scale":{"Code":"PHQ9","Factors":[{"Code":"TOTAL","IsTotalScore":true}]}}}`
	row := []driver.Value{uint64(1), reportInput, "scale", nil, "scale_default", "PHQ9", "v1", "PHQ9", "factor_scoring"}
	mock.ExpectQuery(regexp.QuoteMeta("FROM evaluation_outcome")).WithArgs(500).WillReturnRows(sqlmock.NewRows(columns).AddRow(row...))
	stats, err := run(t.Context(), db, config{limit: 500, mysqlDSN: "test:test@tcp(localhost:3306)/test"})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Scanned != 1 || stats.Upgraded != 1 || stats.Skipped != 0 {
		t.Fatalf("stats = %#v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
