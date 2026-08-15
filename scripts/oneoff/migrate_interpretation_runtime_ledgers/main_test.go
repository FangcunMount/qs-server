package main

import (
	"context"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	drivermysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestMongoAdmissionDomainRejectsFingerprintDrift(t *testing.T) {
	at := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	source := mongoAdmissionFailure{
		DomainID: 1, OutcomeID: 2, EventID: "evt-1", Kind: "catalog_not_found",
		Code: "catalog_not_found", SafeMessage: "missing", Fingerprint: "event:wrong",
		Attempt: 1, Decision: "manual_required", FirstFailedAt: at, LastFailedAt: at, OccurredAt: at,
	}
	if _, err := source.domain(); err == nil {
		t.Fatal("fingerprint drift was accepted")
	}
}

func TestConvertOrgCountsRejectsInvalidOrganizationKey(t *testing.T) {
	if _, err := convertOrgCounts(map[string]mongoDriftCounts{"invalid": {Missing: 1}}); err == nil {
		t.Fatal("invalid organization key was accepted")
	}
}

func TestComponentDatabaseEnvironmentBuildsEscapedConnections(t *testing.T) {
	t.Setenv("QS_APISERVER_MYSQL_HOST", "mysql-rds-proxy:3306")
	t.Setenv("QS_APISERVER_MYSQL_USERNAME", "qs_user")
	t.Setenv("QS_APISERVER_MYSQL_PASSWORD", "p@ss:/word")
	t.Setenv("QS_APISERVER_MYSQL_DATABASE", "qs")
	t.Setenv("QS_APISERVER_MONGODB_HOST", "mongo:27017")
	t.Setenv("QS_APISERVER_MONGODB_USERNAME", "mongo@user")
	t.Setenv("QS_APISERVER_MONGODB_PASSWORD", "mongo:/password")

	dsn, err := componentMySQLDSNFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	mysqlConfig, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if mysqlConfig.Addr != "mysql-rds-proxy:3306" || mysqlConfig.User != "qs_user" || mysqlConfig.Passwd != "p@ss:/word" || mysqlConfig.DBName != "qs" {
		t.Fatalf("unexpected MySQL config: %#v", mysqlConfig)
	}

	mongoURI, err := componentMongoURIFromEnv("qs")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(mongoURI)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	if parsed.Host != "mongo:27017" || parsed.User.Username() != "mongo@user" || password != "mongo:/password" || parsed.Path != "/qs" {
		t.Fatalf("unexpected Mongo URI metadata: host=%s user=%s path=%s", parsed.Host, parsed.User.Username(), parsed.Path)
	}
}

func TestFlushAttentionBatchInsertsAndVerifiesRows(t *testing.T) {
	db, mock := newMigrationMockGorm(t)
	at := time.Date(2026, 8, 15, 6, 0, 0, 0, time.UTC)
	batch := []mysqlAttentionProjectionRow{
		{EventID: "evt-1", ReportID: "report-1", AssessmentID: "assessment-1", TesteeID: 1, Status: "succeeded", CreatedAt: at, UpdatedAt: at},
		{EventID: "evt-2", ReportID: "report-2", AssessmentID: "assessment-2", TesteeID: 2, Status: "succeeded", CreatedAt: at, UpdatedAt: at},
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `interpretation_attention_projection`")).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery("SELECT .* FROM `interpretation_attention_projection` WHERE event_id IN").
		WithArgs("evt-1", "evt-2").
		WillReturnRows(sqlmock.NewRows([]string{"event_id", "report_id", "updated_at"}).
			AddRow("evt-1", "report-1", at).
			AddRow("evt-2", "report-2", at))

	changed, err := flushAttentionBatch(context.Background(), db, batch)
	if err != nil {
		t.Fatal(err)
	}
	if changed != 2 {
		t.Fatalf("changed = %d, want 2", changed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFlushAttentionBatchRejectsOlderTarget(t *testing.T) {
	db, mock := newMigrationMockGorm(t)
	at := time.Date(2026, 8, 15, 6, 0, 0, 0, time.UTC)
	batch := []mysqlAttentionProjectionRow{{
		EventID: "evt-1", ReportID: "report-1", AssessmentID: "assessment-1", TesteeID: 1,
		Status: "succeeded", CreatedAt: at, UpdatedAt: at,
	}}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `interpretation_attention_projection`")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT .* FROM `interpretation_attention_projection` WHERE event_id IN").
		WithArgs("evt-1").
		WillReturnRows(sqlmock.NewRows([]string{"event_id", "report_id", "updated_at"}).
			AddRow("evt-1", "report-1", at.Add(-time.Minute)))

	if _, err := flushAttentionBatch(context.Background(), db, batch); err == nil {
		t.Fatal("older target row was accepted")
	}
}

func newMigrationMockGorm(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}
