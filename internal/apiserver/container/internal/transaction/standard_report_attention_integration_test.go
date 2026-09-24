//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	appTestee "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	actorMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	"github.com/FangcunMount/qs-server/internal/pkg/attentionprojection"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	mysqldriver "github.com/go-sql-driver/mysql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type m5AttentionClient struct {
	handlers.InternalClient
	client pb.InternalServiceClient
}

func (c m5AttentionClient) SyncAssessmentAttention(ctx context.Context, req *pb.SyncAssessmentAttentionRequest) (*pb.SyncAssessmentAttentionResponse, error) {
	return c.client.SyncAssessmentAttention(ctx, req)
}

type m5AttentionSyncClient struct{ client m5AttentionClient }

func (c m5AttentionSyncClient) SyncAssessmentAttention(ctx context.Context, testeeID uint64, riskLevel string, markKeyFocus bool) error {
	_, err := c.client.SyncAssessmentAttention(ctx, &pb.SyncAssessmentAttentionRequest{
		TesteeId: testeeID, RiskLevel: riskLevel, MarkKeyFocus: markKeyFocus,
	})
	return err
}

type m5AttentionLockRunner struct{}

func (m5AttentionLockRunner) Run(ctx context.Context, _ locklease.WorkloadID, _ string, _ time.Duration, body func(context.Context) error) (locklease.RunResult, error) {
	return locklease.RunResult{Acquired: true}, body(ctx)
}

// An ACKed report event can leave an attention failure in MySQL. The restart
// path must reach the real API gRPC service and persist the testee fact once.
func TestM5ReportAttentionReconcileReachesRealTesteeFact(t *testing.T) {
	dsn := os.Getenv("RM_QS_ATTENTION_REAL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m5_qs_attention_real" {
		t.Fatal("disposable m5_qs_attention_real MySQL at mysql:3306 required")
	}
	migrationPath := os.Getenv("RM_QS_ATTENTION_MIGRATION")
	if migrationPath != "/tmp/m5-attention/000068_migrate_interpretation_runtime_ledgers.up.sql" {
		t.Fatal("invocation-owned attention projection migration required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	openDB := func() (*sql.DB, *gorm.DB) {
		t.Helper()
		sqlDB, err := sql.Open("mysql", dsn)
		if err != nil {
			t.Fatal(err)
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			t.Fatal(err)
		}
		gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		return sqlDB, gormDB
	}
	sqlDB, gormDB := openDB()
	defer func() { _ = sqlDB.Close() }()
	if err := gormDB.AutoMigrate(&actorMySQL.TesteePO{}); err != nil {
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
	repo := actorMySQL.NewTesteeRepository(gormDB)
	testee := domainTestee.NewTestee(501, "M5 proof", domainTestee.GenderUnknown, nil)
	if err := repo.Save(ctx, testee); err != nil {
		t.Fatal(err)
	}
	attentionService := appTestee.NewAssessmentAttentionService(repo, domainTestee.NewEditor(domainTestee.NewValidator(repo)), NewMySQLRunner(gormDB))
	var calls atomic.Int32
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if info.FullMethod == pb.InternalService_SyncAssessmentAttention_FullMethodName && calls.Add(1) == 1 {
			return nil, status.Error(codes.Unavailable, "temporary API gRPC outage")
		}
		return next(ctx, request)
	}))
	grpcservice.NewInternalService(attentionService, nil, nil, nil, nil, nil, nil, nil).RegisterService(server)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	defer func() { server.Stop(); <-serverDone }()
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := m5AttentionClient{client: pb.NewInternalServiceClient(conn)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	workerSQL, workerDB := openDB()
	defer func() { _ = workerSQL.Close() }()
	store, err := attentionprojection.NewMySQLStore(workerDB)
	if err != nil {
		t.Fatal(err)
	}
	projector := attentionprojection.NewProjector(store, m5AttentionSyncClient{client}, attentionprojection.DefaultMaxAttempts, logger)
	handler, ok := handlers.NewRegistry().Create("interpretation_report_generated_handler", &handlers.Dependencies{
		Logger: logger, InternalClient: client, AttentionProjector: projector,
	})
	if !ok {
		t.Fatal("report generated handler absent from production registry")
	}
	eventID := "m5-attention-real-event"
	payload, err := json.Marshal(map[string]any{
		"id": eventID, "eventType": eventcatalog.InterpretationReportGenerated,
		"occurredAt": time.Now().UTC(), "aggregateType": "ReportGeneration", "aggregateID": "m5-generation",
		"data": map[string]any{
			"org_id": 501, "generation_id": "m5-generation", "run_id": "m5-run", "report_id": "m5-report",
			"assessment_id": "123", "outcome_id": "456", "testee_id": testee.ID().Uint64(),
			"attempt": 1, "report_type": "standard", "template_version": "v2", "builder_identity": "factor-scoring",
			"content_schema_version": "report-content/v2", "model": map[string]any{"kind": "scale", "algorithm": "scale_default", "code": "SDS"},
			"level": map[string]any{"code": "severe", "label": "severe", "severity": "high"}, "generated_at": time.Now().UTC(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler(ctx, eventcatalog.InterpretationReportGenerated, payload); err != nil {
		t.Fatal(err)
	}
	record, err := store.GetByEventID(ctx, eventID)
	if err != nil || record.Status != attentionprojection.StatusFailed || calls.Load() != 1 {
		t.Fatalf("first delivery record=%+v calls=%d err=%v", record, calls.Load(), err)
	}
	var focused bool
	if err := sqlDB.QueryRowContext(ctx, "SELECT is_key_focus FROM testee WHERE id=?", testee.ID().Uint64()).Scan(&focused); err != nil || focused {
		t.Fatalf("attention fact changed before recovery: focused=%t err=%v", focused, err)
	}
	if err := workerSQL.Close(); err != nil {
		t.Fatal(err)
	}
	workerSQL, workerDB = openDB()
	store, err = attentionprojection.NewMySQLStore(workerDB)
	if err != nil {
		t.Fatal(err)
	}
	projector = attentionprojection.NewProjector(store, m5AttentionSyncClient{client}, attentionprojection.DefaultMaxAttempts, logger)
	reconciler, err := attentionprojection.NewReconciler(projector, m5AttentionLockRunner{}, 0, 10, logger)
	if err != nil {
		t.Fatal(err)
	}
	if acquired, err := reconciler.RunOnce(ctx); err != nil || !acquired {
		t.Fatalf("reconcile after connection restart: acquired=%t err=%v", acquired, err)
	}
	record, err = store.GetByEventID(ctx, eventID)
	if err != nil || record.Status != attentionprojection.StatusSucceeded || calls.Load() != 2 {
		t.Fatalf("recovered record=%+v calls=%d err=%v", record, calls.Load(), err)
	}
	if err := sqlDB.QueryRowContext(ctx, "SELECT is_key_focus FROM testee WHERE id=?", testee.ID().Uint64()).Scan(&focused); err != nil || !focused {
		t.Fatalf("real testee attention fact absent: focused=%t err=%v", focused, err)
	}
	handler, ok = handlers.NewRegistry().Create("interpretation_report_generated_handler", &handlers.Dependencies{
		Logger: logger, InternalClient: client, AttentionProjector: projector,
	})
	if !ok {
		t.Fatal("report generated handler absent after Worker reconstruction")
	}
	if err := handler(ctx, eventcatalog.InterpretationReportGenerated, payload); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("duplicate event called API again: calls=%d", calls.Load())
	}
}
