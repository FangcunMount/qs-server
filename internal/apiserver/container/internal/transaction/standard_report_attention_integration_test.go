//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/messaging"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	appTestee "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	actorMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	"github.com/FangcunMount/qs-server/internal/pkg/attentionprojection"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/nsqio/go-nsq"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

// A synthetic report event takes the standard Mongo outbox, NSQ and original
// Worker path. Its ACK can leave an attention failure in MySQL; restart must
// reach the real API gRPC service and persist the testee fact once.
func TestM5ReportAttentionReconcileReachesRealTesteeFact(t *testing.T) {
	dsn := os.Getenv("RM_QS_ATTENTION_REAL_DSN")
	mongoURI, nsqAddress := os.Getenv("RM_QS_ATTENTION_MONGO_URI"), os.Getenv("RM_QS_NSQ_TCP")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m5_qs_attention_real" ||
		!strings.HasPrefix(mongoURI, "mongodb://mongo:27017/") || !strings.Contains(mongoURI, "replicaSet=rm-test") || nsqAddress != "nsqd:4150" {
		t.Fatal("disposable m5_qs_attention_real MySQL, Mongo replica set and nsqd:4150 required")
	}
	migrationPath := os.Getenv("RM_QS_ATTENTION_MIGRATION")
	if migrationPath != "/tmp/m5-attention/000068_migrate_interpretation_runtime_ledgers.up.sql" {
		t.Fatal("invocation-owned attention projection migration required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
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
	firstHandler := handler
	evt := event.New(eventcatalog.InterpretationReportGenerated, "ReportGeneration", "m5-generation", map[string]any{
		"org_id": 501, "generation_id": "m5-generation", "run_id": "m5-run", "report_id": "m5-report",
		"assessment_id": "123", "outcome_id": "456", "testee_id": testee.ID().Uint64(),
		"attempt": 1, "report_type": "standard", "template_version": "v2", "builder_identity": "factor-scoring",
		"content_schema_version": "report-content/v2", "model": map[string]any{"kind": "scale", "algorithm": "scale_default", "code": "SDS"},
		"level": map[string]any{"code": "severe", "label": "severe", "severity": "high"}, "generated_at": time.Now().UTC(),
	})
	eventID := evt.EventID()
	payload, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	mongoDB := mongoClient.Database("m5_qs_attention_real")
	defer func() { _ = mongoDB.Drop(context.Background()) }()
	if err := mongoDB.CreateCollection(ctx, "rm_outbox"); err != nil {
		t.Fatal(err)
	}
	collection := mongoDB.Collection("rm_outbox")
	if _, err := collection.Indexes().CreateMany(ctx, sdkmongo.Indexes()); err != nil {
		t.Fatal(err)
	}
	catalog, err := eventcatalog.Load("/configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stager, err := mongostandard.NewStager(collection, eventcatalog.NewCatalog(catalog), eventruntime.SourceAPIServer)
	if err != nil {
		t.Fatal(err)
	}
	session, err := mongoClient.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.WithTransaction(ctx, func(tx mongo.SessionContext) (any, error) {
		return nil, stager.Stage(tx, evt)
	})
	session.EndSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config := nsq.NewConfig()
	config.HeartbeatInterval, config.MsgTimeout = time.Second, 10*time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	const topic = "qs.evaluation.lifecycle"
	consumer, err := nsq.NewConsumer(topic, "rm-m5-attention-real", config)
	if err != nil {
		t.Fatal(err)
	}
	consumer.SetLogger(nil, nsq.LogLevelError)
	deliveries := make(chan error, 1)
	consumer.AddHandler(nsq.HandlerFunc(func(raw *nsq.Message) error {
		decoded, recognized, deliveryErr := messaging.DecodeMessagePayload(raw.Body)
		if deliveryErr == nil && (!recognized || decoded.UUID != eventID || decoded.Metadata["event_type"] != eventcatalog.InterpretationReportGenerated) {
			deliveryErr = fmt.Errorf("standard report event lost its original NSQ identity")
		}
		if deliveryErr == nil {
			deliveryErr = firstHandler(ctx, eventcatalog.InterpretationReportGenerated, decoded.Payload)
		}
		select {
		case deliveries <- deliveryErr:
		case <-ctx.Done():
			return ctx.Err()
		}
		return deliveryErr
	}))
	if err := consumer.ConnectToNSQD(nsqAddress); err != nil {
		t.Fatal(err)
	}
	defer func() {
		consumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("NSQ consumer did not stop")
		}
	}()
	producer, err := nsq.NewProducer(nsqAddress, config)
	if err != nil {
		t.Fatal(err)
	}
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{topic: topic}, 1)
	if err != nil {
		t.Fatal(err)
	}
	mongoStore, err := sdkmongo.New(collection)
	if err != nil {
		t.Fatal(err)
	}
	forwarder, err := relay.New(mongoStore, publisher, relay.Config{
		Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 5 * time.Second,
		PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
		Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	relayCtx, stopRelay := context.WithCancel(ctx)
	relayDone := make(chan error, 1)
	go func() { relayDone <- forwarder.Run(relayCtx) }()
	defer func() {
		stopRelay()
		if err := <-relayDone; err != nil {
			t.Errorf("standard relay: %v", err)
		}
		if err := publisher.Drain(ctx); err != nil {
			t.Errorf("publisher drain: %v", err)
		}
	}()
	select {
	case err := <-deliveries:
		if err != nil {
			t.Fatalf("standard report delivery: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("standard report not delivered", ctx.Err())
	}
	var outboxState struct {
		State string `bson:"state"`
	}
	if err := collection.FindOne(ctx, bson.M{"message_id": eventID}).Decode(&outboxState); err != nil || outboxState.State != "published" {
		t.Fatalf("standard Mongo outbox state=%q err=%v", outboxState.State, err)
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
