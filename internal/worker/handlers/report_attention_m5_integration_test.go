//go:build reliable_messaging_m4_integration

package handlers

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/attentionprojection"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// The Worker may ACK a generated report while attention syncing has failed.
// Its durable MySQL ledger must survive a Worker restart and make redelivery
// of the original report event harmless after reconciliation succeeds.
func TestM5ReportGeneratedAttentionRecoversFromDurableMySQLLedger(t *testing.T) {
	dsn := os.Getenv("RM_QS_ATTENTION_MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m5_qs_attention" {
		t.Fatal("disposable m5_qs_attention MySQL at mysql:3306 required")
	}
	migrationPath := os.Getenv("RM_QS_ATTENTION_MIGRATION")
	if migrationPath != "/tmp/m5-attention/000068_migrate_interpretation_runtime_ledgers.up.sql" {
		t.Fatal("invocation-owned attention projection migration required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlDB.Close() }()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	const tableStart = "CREATE TABLE `interpretation_attention_projection` ("
	from := strings.Index(string(migration), tableStart)
	if from < 0 {
		t.Fatal("attention projection schema absent from production migration")
	}
	statement := string(migration)[from:]
	statement = statement[:strings.Index(statement, ";")+1]
	if _, err := sqlDB.ExecContext(ctx, statement); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	openStore := func() *attentionprojection.MySQLStore {
		t.Helper()
		gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		store, err := attentionprojection.NewMySQLStore(gormDB)
		if err != nil {
			t.Fatal(err)
		}
		return store
	}
	client := &fakeWorkerInternalClient{syncAssessmentAttentionErr: errors.New("temporary RPC outage")}
	store := openStore()
	reporter := &reportStatusWriterStub{}
	makeHandler := func(projector *attentionprojection.Projector) HandlerFunc {
		return handleInterpretationReportGenerated(&Dependencies{
			Logger: logger, InternalClient: client,
			AttentionProjector: projector, ReportStatusReporter: reporter,
		})
	}
	payload := mustBuildReportGeneratedOutcomePayload(t, "high", "severe")
	projector := attentionprojection.NewProjector(store, &attentionSyncClientStub{client: client},
		attentionprojection.DefaultMaxAttempts, logger)
	if err := makeHandler(projector)(ctx, eventcatalog.InterpretationReportGenerated, payload); err != nil {
		t.Fatal(err)
	}
	if reporter.completedAssessmentID != "123" || reporter.completedReportID != "report-1" {
		t.Fatalf("report status after failed attention RPC: assessment=%q report=%q",
			reporter.completedAssessmentID, reporter.completedReportID)
	}
	record, err := store.GetByEventID(ctx, "evt-report-generated-outcome")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != attentionprojection.StatusFailed || record.Attempt != 1 ||
		record.ReportID != "report-1" || !record.MarkKeyFocus || client.syncAssessmentAttentionCalls != 1 {
		t.Fatalf("durable failed attention projection=%+v calls=%d", record, client.syncAssessmentAttentionCalls)
	}

	// Reconstruct the store and projector as a restarted Worker would.
	client.syncAssessmentAttentionErr = nil
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	sqlDB, err = sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	store = openStore()
	projector = attentionprojection.NewProjector(store, &attentionSyncClientStub{client: client},
		attentionprojection.DefaultMaxAttempts, logger)
	runner := runnerStub{run: func(ctx context.Context, _ locklease.WorkloadID, _ string, _ time.Duration,
		body func(context.Context) error) (locklease.RunResult, error) {
		return locklease.RunResult{Acquired: true}, body(ctx)
	}}
	reconciler, err := attentionprojection.NewReconciler(projector, runner, 0, 10, logger)
	if err != nil {
		t.Fatal(err)
	}
	if acquired, err := reconciler.RunOnce(ctx); err != nil || !acquired {
		t.Fatalf("reconcile after restart: acquired=%t err=%v", acquired, err)
	}
	record, err = store.GetByEventID(ctx, "evt-report-generated-outcome")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != attentionprojection.StatusSucceeded || record.Attempt != 1 ||
		client.syncAssessmentAttentionCalls != 2 || !client.syncAssessmentAttentionRequest.MarkKeyFocus {
		t.Fatalf("recovered attention projection=%+v calls=%d request=%+v", record,
			client.syncAssessmentAttentionCalls, client.syncAssessmentAttentionRequest)
	}
	if err := makeHandler(projector)(ctx, eventcatalog.InterpretationReportGenerated, payload); err != nil {
		t.Fatal(err)
	}
	if client.syncAssessmentAttentionCalls != 2 {
		t.Fatalf("duplicate report event repeated successful attention RPC: calls=%d", client.syncAssessmentAttentionCalls)
	}
}
