package evaluation

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type historicalUnknownPool struct{ gorm.ConnPool }

func (*historicalUnknownPool) Commit() error   { return nil }
func (*historicalUnknownPool) Rollback() error { return nil }

func TestHistoricalReferencesRequireOriginalBorrowedTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	g, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var nilSQL *sql.Tx
	var nilPrepared *gorm.PreparedStmtTX
	for _, scenario := range []struct {
		name  string
		pool  gorm.ConnPool
		valid bool
	}{
		{"sql", tx, true}, {"prepared", &gorm.PreparedStmtTX{Tx: tx}, true},
		{"normal", db, false}, {"wrapper", &historicalUnknownPool{ConnPool: tx}, false},
		{"nil", nilSQL, false}, {"nil-prepared", nilPrepared, false},
		{"prepared-nil-tx", &gorm.PreparedStmtTX{Tx: nilSQL}, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			candidate := g.Session(&gorm.Session{NewDB: true})
			candidate.Statement.ConnPool = scenario.pool
			bound, err := historicalTx(hostmysql.WithTx(t.Context(), candidate))
			if scenario.valid {
				if err != nil || bound.Statement.ConnPool != scenario.pool {
					t.Fatal("original transaction not preserved", err)
				}
			} else if err == nil {
				t.Fatal("untrusted transaction accepted")
			}
		})
	}
	mock.ExpectRollback()
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHistoricalReferencesFreezeUnknownColumnsAndNULL(t *testing.T) {
	base := historicalSQLRow{"id": optionalString("42"), "new_business_fact": nil, "history": nil}
	same := historicalSQLRow{"id": optionalString("42"), "new_business_fact": nil, "history": optionalString("new append")}
	if !historicalSameFacts(base, same, "history") {
		t.Fatal("maintenance append changed business baseline")
	}
	empty := ""
	same["new_business_fact"] = &empty
	if historicalSameFacts(base, same, "history") {
		t.Fatal("NULL collapsed to empty")
	}
	delete(same, "new_business_fact")
	if historicalSameFacts(base, same, "history") {
		t.Fatal("unknown column disappeared")
	}
}
