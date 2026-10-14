package actor

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOperatorPlansRequireOneKnownCleanPostRetirementSchema(t *testing.T) {
	for _, test := range []struct {
		name       string
		rows       *sqlmock.Rows
		queryError bool
		accepted   bool
	}{
		{"clean102", sqlmock.NewRows([]string{"version", "dirty"}).AddRow(102, false), false, true},
		{"preDDL101", sqlmock.NewRows([]string{"version", "dirty"}).AddRow(101, false), false, false},
		{"dirty102", sqlmock.NewRows([]string{"version", "dirty"}).AddRow(102, true), false, false},
		{"unknownFuture", sqlmock.NewRows([]string{"version", "dirty"}).AddRow(103, false), false, false},
		{"empty", sqlmock.NewRows([]string{"version", "dirty"}), false, false},
		{"ambiguous", sqlmock.NewRows([]string{"version", "dirty"}).AddRow(102, false).AddRow(102, false), false, false},
		{"nullDirty", sqlmock.NewRows([]string{"version", "dirty"}).AddRow(102, nil), false, false},
		{"unavailable", nil, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			db, err := gorm.Open(mysql.New(mysql.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			query := mock.ExpectQuery("SELECT version,dirty FROM schema_migrations LIMIT 2")
			if test.queryError {
				query.WillReturnError(fmt.Errorf("unavailable"))
			} else {
				query.WillReturnRows(test.rows)
			}
			if err := RequireCurrentOperatorPlanSchema(context.Background(), db); (err == nil) != test.accepted {
				t.Fatalf("accepted=%v want=%v", err == nil, test.accepted)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
