package main

import (
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBackfillEvaluationDryRunAndRepeatedApplyAreIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	columns := []string{"id", "assessment_id", "attempt_no", "retryable", "org_id", "testee_id", "questionnaire_code", "questionnaire_version", "answer_sheet_id", "evaluation_model_kind", "evaluation_model_sub_kind", "evaluation_model_algorithm", "evaluation_model_code", "evaluation_model_version"}
	row := []driver.Value{uint64(11), uint64(22), 3, true, int64(7), uint64(8), "Q", "v1", uint64(9), nil, nil, nil, nil, nil}
	query := regexp.QuoteMeta("FROM runtime_checkpoint rc")
	mock.ExpectQuery(query).WithArgs(100).WillReturnRows(sqlmock.NewRows(columns).AddRow(row...))
	count, err := backfillEvaluation(t.Context(), db, config{limit: 100}, func() time.Time { return time.Unix(100, 0) })
	if err != nil || count != 1 {
		t.Fatalf("dry-run count/error = %d/%v", count, err)
	}

	mock.ExpectQuery(query).WithArgs(100).WillReturnRows(sqlmock.NewRows(columns).AddRow(row...))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE runtime_checkpoint SET attempt_origin=COALESCE(attempt_origin,'initial'), retry_disposition=?")).
		WithArgs("manual_required", uint64(11)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	count, err = backfillEvaluation(t.Context(), db, config{limit: 100, apply: true}, func() time.Time { return time.Unix(100, 0) })
	if err != nil || count != 1 {
		t.Fatalf("apply count/error = %d/%v", count, err)
	}

	mock.ExpectQuery(query).WithArgs(100).WillReturnRows(sqlmock.NewRows(columns))
	count, err = backfillEvaluation(t.Context(), db, config{limit: 100, apply: true}, func() time.Time { return time.Unix(100, 0) })
	if err != nil || count != 0 {
		t.Fatalf("repeat apply count/error = %d/%v", count, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluationRetryEventIDIsDeterministic(t *testing.T) {
	if first, second := evaluationRetryEventID(22, 2), evaluationRetryEventID(22, 2); first != second || first != "eval-retry:22:2:automatic" {
		t.Fatalf("event ids = %q/%q", first, second)
	}
}
