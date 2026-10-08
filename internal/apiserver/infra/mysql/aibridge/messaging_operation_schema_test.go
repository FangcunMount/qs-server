package aibridge

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestOperationSchemaReadinessRejectsMissingConstraintAndInvalidIndexes(t *testing.T) {
	for _, scenario := range []string{"complete", "unenforced", "missing_index", "wrong_order", "prefix_index", "invisible"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectBegin()
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery("SELECT retired,retirement_evidence,retired_at,body_sha256,aggregate_sequence,created_at").WillReturnRows(sqlmock.NewRows([]string{"retired"}))
			count := 1
			if scenario == "unenforced" {
				count = 0
			}
			mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM information_schema.TABLE_CONSTRAINTS").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
			if count == 1 {
				rows := sqlmock.NewRows([]string{"index", "sequence", "column", "prefix", "non_unique", "type", "visible"})
				for _, name := range []string{"idx_ai_messaging_operations_pending_stats", "idx_ai_messaging_operations_request_stats"} {
					columns := []string{"organization_id", "retired", "kind", "decision", "aggregate_key", "command_id"}
					if strings.Contains(name, "request") {
						columns = []string{"organization_id", "retired", "aggregate_key", "kind", "decision", "command_id"}
					}
					if scenario == "missing_index" && strings.Contains(name, "request") {
						continue
					}
					for i, column := range columns {
						var prefix any
						visible := "YES"
						if i == 0 && scenario == "wrong_order" {
							column = "subject_id"
						}
						if i == 0 && scenario == "prefix_index" {
							prefix = int64(8)
						}
						if i == 0 && scenario == "invisible" {
							visible = "NO"
						}
						rows.AddRow(name, i+1, column, prefix, 1, "BTREE", visible)
					}
				}
				mock.ExpectQuery("SELECT INDEX_NAME,SEQ_IN_INDEX,COLUMN_NAME,SUB_PART,NON_UNIQUE,INDEX_TYPE,IS_VISIBLE").WillReturnRows(rows)
			}
			got := RequireMessagingOperationSchema(context.Background(), tx)
			if (got == nil) != (scenario == "complete") {
				t.Fatal("unexpected readiness", got)
			}
			mock.ExpectRollback()
			if err = tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
