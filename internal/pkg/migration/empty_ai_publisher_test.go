package migration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	sqlmysql "github.com/go-sql-driver/mysql"
)

func TestEmptyPublisherGuardRejectsRecordsAndReleasesItsSession(t *testing.T) {
	raw, err := migrations.ReadFile("migrations/mysql/000111_retire_empty_ai_publisher.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"empty", "nonempty", "query_error", "cancelled", "ddl_error", "unlock_error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
				err := sqlmock.QueryMatcherEqual.Match(expected, actual)
				if err == nil && mode == "cancelled" && strings.HasPrefix(actual, "SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*)") {
					cancel()
				}
				return err
			})
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			conn, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			d := &retirementDatabaseDriver{backend: BackendMySQL, version: 111, ctx: ctx, sqlConn: conn}
			mock.ExpectQuery("SELECT @@SESSION.lock_wait_timeout").WillReturnRows(sqlmock.NewRows([]string{"wait"}).AddRow(600))
			mock.ExpectExec("SET SESSION lock_wait_timeout = 2").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec("LOCK TABLES `ai_messaging_outbox` WRITE").WillReturnResult(sqlmock.NewResult(0, 0))
			count := mock.ExpectQuery("SELECT /*+ MAX_EXECUTION_TIME(2000) */ COUNT(*) FROM `ai_messaging_outbox`")
			switch mode {
			case "query_error":
				count.WillReturnError(errors.New("stored facts unavailable"))
			case "cancelled":
				count.WillDelayFor(time.Second).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
			case "nonempty":
				count.WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
			default:
				count.WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
			}
			prepared := mode == "empty" || mode == "ddl_error" || mode == "unlock_error"
			if prepared {
				mock.ExpectExec("SET @qs_m5_empty_ai_publisher_guard = 1").WillReturnResult(sqlmock.NewResult(0, 0))
				q := mock.ExpectExec(string(raw))
				if mode == "ddl_error" {
					q.WillReturnError(errors.New("DDL failed"))
				} else {
					q.WillReturnResult(sqlmock.NewResult(0, 0))
				}
			}
			unlock := mock.ExpectExec("UNLOCK TABLES")
			if mode == "unlock_error" {
				unlock.WillReturnError(errors.New("release unknown"))
			} else {
				unlock.WillReturnResult(sqlmock.NewResult(0, 0))
			}
			mock.ExpectExec("SET @qs_m5_empty_ai_publisher_guard = NULL, @qs_m5_ai_publisher_sql = NULL").WillReturnResult(sqlmock.NewResult(0, 0))
			deallocate := mock.ExpectExec("DEALLOCATE PREPARE qs_m5_ai_publisher_stmt")
			if prepared {
				deallocate.WillReturnResult(sqlmock.NewResult(0, 0))
			} else {
				deallocate.WillReturnError(&sqlmysql.MySQLError{Number: 1243})
			}
			mock.ExpectExec("SET SESSION lock_wait_timeout = ?").WithArgs(uint64(600)).WillReturnResult(sqlmock.NewResult(0, 0))
			if mode == "unlock_error" {
				mock.ExpectClose()
			}
			err = d.runEmptyAIPublisherStep(raw)
			if (mode == "empty") != (err == nil) {
				t.Fatal(mode, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
