package aibridge

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// This exercises ownership and failure propagation using a real database/sql
// Tx over sqlmock. Held/terminal/organization semantics have native fixtures.
func TestCurrentMessagingObservationBorrowsTransactionAndRejectsUnknown(t *testing.T) {
	for _, scenario := range []string{"counts", "integrity_conflict", "read_unknown"} {
		t.Run(scenario, func(t *testing.T) {
			db, m, e := sqlmock.New()
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = db.Close() }()
			m.ExpectBegin()
			tx, e := db.BeginTx(t.Context(), nil)
			if e != nil {
				t.Fatal(e)
			}
			m.ExpectQuery("SELECT EXISTS").WithArgs(int64(42), int64(42), int64(42), int64(42)).WillReturnRows(sqlmock.NewRows([]string{"invalid"}).AddRow(scenario == "integrity_conflict"))
			if scenario != "integrity_conflict" {
				q := m.ExpectQuery(`SELECT COUNT\(\*\)`).WithArgs(int64(42))
				if scenario == "read_unknown" {
					q.WillReturnError(errors.New("read unknown"))
				} else {
					q.WillReturnRows(sqlmock.NewRows([]string{"requests", "pending", "attempts"}).AddRow(1, 3, 15))
				}
			}
			v, e := ReadCurrentMessagingOrganization(t.Context(), tx, 42)
			if scenario == "counts" && (e != nil || v.Requests != 1 || v.CommandsPending != 3 || v.CommandAttempts != 15) {
				t.Fatal(v, e)
			}
			if scenario != "counts" && (e == nil || v != (CurrentMessagingOrganization{})) {
				t.Fatal("unknown produced statistics", v, e)
			}
			// Only the original caller ends its transaction.
			m.ExpectRollback()
			if e = tx.Rollback(); e != nil {
				t.Fatal(e)
			}
			if e = m.ExpectationsWereMet(); e != nil {
				t.Fatal(e)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := ReadCurrentMessagingOrganization(ctx, nil, 42); e == nil {
		t.Fatal("canceled/unbound reader accepted")
	}
}
