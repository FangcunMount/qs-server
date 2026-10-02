//go:build integration && reliable_messaging_m4

package runtimeclosure_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	appTestee "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	execution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	mongointerpretation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	actorMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	"github.com/FangcunMount/qs-server/internal/pkg/attentionprojection"
	appmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	legacywire "github.com/FangcunMount/reliable-messaging/wire/legacy"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/nsqio/go-nsq"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type m6AttentionClient struct {
	handlers.InternalClient
	client pb.InternalServiceClient
}

func (c m6AttentionClient) SyncAssessmentAttention(ctx context.Context, req *pb.SyncAssessmentAttentionRequest) (*pb.SyncAssessmentAttentionResponse, error) {
	return c.client.SyncAssessmentAttention(ctx, req)
}

type m6AttentionSyncClient struct{ client m6AttentionClient }

func (c m6AttentionSyncClient) SyncAssessmentAttention(ctx context.Context, testeeID uint64, riskLevel string, markKeyFocus bool) error {
	_, err := c.client.SyncAssessmentAttention(ctx, &pb.SyncAssessmentAttentionRequest{
		TesteeId: testeeID, RiskLevel: riskLevel, MarkKeyFocus: markKeyFocus,
	})
	return err
}

func TestAttentionFactRecoveryAfterActualNSQConfirmationLoss(t *testing.T) {
	dsn := os.Getenv("M6_ATTENTION_MYSQL_DSN")
	if dsn == "" {
		t.Skip("owned disposable MySQL/Mongo/Redis/NSQ proof requires explicit environment")
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	mongoURI := os.Getenv("M6_HOTRANK_MONGO_URI")
	nsqAddress := os.Getenv("M6_HOTRANK_NSQ_ADDRESS")
	nsqHTTP := os.Getenv("M6_HOTRANK_NSQ_HTTP")
	redisAddress := os.Getenv("M6_HOTRANK_REDIS_ADDRESS")
	owner := os.Getenv("M6_HOTRANK_OWNER")
	container := os.Getenv("M6_HOTRANK_NSQ_CONTAINER")
	if err != nil || parsed.Net != "tcp" || !strings.HasPrefix(parsed.Addr, "127.0.0.1:") || parsed.DBName != "m6_attention_test" || !strings.HasPrefix(mongoURI, "mongodb://127.0.0.1:") || !strings.Contains(mongoURI, "directConnection=true") || !strings.HasPrefix(nsqAddress, "127.0.0.1:") || !strings.HasPrefix(nsqHTTP, "http://127.0.0.1:") || !strings.HasPrefix(redisAddress, "127.0.0.1:") || !strings.HasPrefix(container, "m6-hotrank-") || owner == "" {
		t.Fatal("only explicitly owned loopback disposable databases and broker allowed")
	}
	migrationPath := "../../../../internal/pkg/migration/migrations/mysql/000068_migrate_interpretation_runtime_ledgers.up.sql"
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
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
	attentionService := appTestee.NewAssessmentAttentionService(repo, domainTestee.NewEditor(domainTestee.NewValidator(repo)), apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		return appmysql.NewUnitOfWork(gormDB).WithinTransaction(ctx, fn)
	}))
	var calls atomic.Int32
	var updates atomic.Int32
	if err := gormDB.Callback().Update().After("gorm:update").Register("m6_attention_business_update", func(tx *gorm.DB) {
		if tx.Statement.Table == "testee" && tx.Error == nil && tx.RowsAffected > 0 {
			updates.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if info.FullMethod == pb.InternalService_SyncAssessmentAttention_FullMethodName {
			calls.Add(1)
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
	client := m6AttentionClient{client: pb.NewInternalServiceClient(conn)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	statusClient := redis.NewClient(&redis.Options{Addr: redisAddress, MaxRetries: -1})
	defer statusClient.Close()
	statusReporter, err := reportstatus.NewReporter(&redisruntime.Handle{
		Namespace: "m6-attention-" + owner + "", Client: statusClient,
	}, reportstatus.Config{TTL: time.Hour, Service: "qs-worker"})
	if err != nil {
		t.Fatal(err)
	}
	workerSQL, workerDB := openDB()
	defer func() { _ = workerSQL.Close() }()
	store, err := attentionprojection.NewMySQLStore(workerDB)
	if err != nil {
		t.Fatal(err)
	}
	projector := attentionprojection.NewProjector(store, m6AttentionSyncClient{client}, attentionprojection.DefaultMaxAttempts, logger)
	handler, ok := handlers.NewRegistry().Create("interpretation_report_generated_handler", &handlers.Dependencies{
		Logger: logger, InternalClient: client, AttentionProjector: projector, ReportStatusReporter: statusReporter,
	})
	if !ok {
		t.Fatal("report generated handler absent from production registry")
	}
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	mongoDB := mongoClient.Database("m6_attention_test")
	defer func() { _ = mongoDB.Drop(context.Background()) }()
	if err := mongoDB.CreateCollection(ctx, "rm_outbox"); err != nil {
		t.Fatal(err)
	}
	collection := mongoDB.Collection("rm_outbox")
	if _, err := collection.Indexes().CreateMany(ctx, sdkmongo.Indexes()); err != nil {
		t.Fatal(err)
	}
	catalog, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stager, err := mongostandard.NewStager(collection, eventcatalog.NewCatalog(catalog), eventruntime.SourceAPIServer)
	if err != nil {
		t.Fatal(err)
	}
	generations, err := mongointerpretation.NewGenerationRepository(mongoDB)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := mongointerpretation.NewRunRepository(mongoDB)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := mongointerpretation.NewReportRepository(mongoDB)
	if err != nil {
		t.Fatal(err)
	}
	reportCatalog, err := mongointerpretation.NewReportCatalogProjector(mongoDB)
	if err != nil {
		t.Fatal(err)
	}
	runner := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		session, err := mongoClient.StartSession()
		if err != nil {
			return err
		}
		defer session.EndSession(ctx)
		_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (any, error) { return nil, fn(sc) }, options.Transaction().SetReadPreference(readpref.Primary()).SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))
		return err
	})
	starter, err := execution.NewStarter(runner, generations, runs, reports, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	key := domaingeneration.Key{OutcomeID: meta.New(), ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("v1")}
	started, err := starter.Start(ctx, execution.StartRequest{Key: key, TraceID: "m5-attention-real"})
	if err != nil || started == nil || started.Status != execution.StartStatusStarted {
		t.Fatalf("report run was not admitted: result=%+v err=%v", started, err)
	}
	completedAt := time.Now().UTC()
	assessmentID := meta.New()
	artifact, err := domainreport.NewInterpretReport(domainreport.InterpretReportInput{
		ID: meta.New(), GenerationID: started.Generation.ID(), OutcomeID: key.OutcomeID, InterpretationRunID: started.Run.ID(),
		Association: domainreport.Association{OrgID: 501, AssessmentID: assessmentID, TesteeID: testee.ID().Uint64()},
		ReportType:  policy.ReportTypeStandard, TemplateVersion: key.TemplateVersion,
		BuilderIdentity: domainreport.BuilderIdentityFactorScoring, ContentSchemaVersion: domainreport.ContentSchemaVersionV1,
		Content: domainreport.Content{
			Model:        domainreport.ModelIdentity{Kind: "scale", Code: "SDS", Version: "v1"},
			PrimaryScore: domainreport.NewRawTotalScore(80, nil), Level: domainreport.LevelFromRisk(domainreport.RiskLevelHigh),
			Dimensions: []domainreport.DimensionInterpret{
				domainreport.NewDimensionInterpret(domainreport.NewFactorCode("TOTAL"), "total", 80, nil, domainreport.RiskLevelHigh, "high", "follow up"),
			},
		},
		GeneratedAt: completedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	committer, err := execution.NewInterpretationCommitter(runner, generations, runs, reports, stager, nil, reportCatalog)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := committer.CommitSuccess(ctx, execution.CommitSuccessRequest{
		Generation: started.Generation, Run: started.Run, InterpretReport: artifact,
		BuilderIdentity: artifact.BuilderIdentity(), ContentSchemaVersion: artifact.ContentSchemaVersion(), CompletedAt: completedAt,
	})
	if err != nil || committed == nil || committed.Generation.Status() != domaingeneration.StatusGenerated || committed.Run.Status() != interpretationrun.StatusSucceeded {
		t.Fatalf("report fact and intent commit: result=%+v err=%v", committed, err)
	}
	persistedReport, err := reports.FindByID(ctx, artifact.ID())
	if err != nil || persistedReport == nil {
		t.Fatalf("committed report missing: report=%v err=%v", persistedReport, err)
	}
	persistedGeneration, err := generations.FindByID(ctx, started.Generation.ID())
	if err != nil || persistedGeneration == nil || persistedGeneration.Status() != domaingeneration.StatusGenerated {
		t.Fatalf("committed generation not generated: generation=%v err=%v", persistedGeneration, err)
	}
	persistedRun, err := runs.FindByID(ctx, started.Run.ID())
	if err != nil || persistedRun == nil || persistedRun.Status() != interpretationrun.StatusSucceeded {
		t.Fatalf("committed run not succeeded: run=%v err=%v", persistedRun, err)
	}
	intentCount, err := collection.CountDocuments(ctx, bson.M{"event_type": eventcatalog.InterpretationReportGenerated})
	if err != nil || intentCount != 1 {
		t.Fatalf("standard report intents=%d err=%v", intentCount, err)
	}
	legacyCount, err := mongoDB.Collection("domain_event_outbox").CountDocuments(ctx, bson.M{})
	if err != nil || legacyCount != 0 {
		t.Fatalf("legacy report intents=%d err=%v", legacyCount, err)
	}
	var outboxRow struct {
		MessageID string `bson:"message_id"`
		Payload   []byte `bson:"payload"`
		State     string `bson:"state"`
	}
	if err := collection.FindOne(ctx, bson.M{"event_type": eventcatalog.InterpretationReportGenerated}).Decode(&outboxRow); err != nil || outboxRow.State != "pending" {
		t.Fatalf("committed report intent=%+v err=%v", outboxRow, err)
	}
	eventID := outboxRow.MessageID
	wire, recognized, err := legacywire.Decode(outboxRow.Payload)
	if err != nil || !recognized || wire.UUID != eventID {
		t.Fatalf("committed report wire identity: id=%s recognized=%t err=%v", eventID, recognized, err)
	}

	config := nsq.NewConfig()
	config.HeartbeatInterval, config.MsgTimeout = time.Second, 10*time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	const topic = "qs.evaluation.lifecycle"
	producer, err := nsq.NewProducer(nsqAddress, config)
	if err != nil {
		t.Fatal(err)
	}
	producer.SetLogger(nil, nsq.LogLevelError)
	publisher, err := sdknsq.New(producer, map[string]string{topic: topic}, 1)
	if err != nil {
		t.Fatal(err)
	}
	mongoStore, err := sdkmongo.New(collection)
	if err != nil {
		t.Fatal(err)
	}
	forwarder, err := relay.New(mongoStore, publisher, relay.Config{Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 5 * time.Second, PublishTimeout: 2 * time.Second, WriteTimeout: time.Second, Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {}})
	if err != nil {
		t.Fatal(err)
	}
	relayCtx, stopRelay := context.WithCancel(ctx)
	relayDone := make(chan error, 1)
	go func() { relayDone <- forwarder.Run(relayCtx) }()
	defer producer.Stop()
	stopped := false
	defer func() {
		if !stopped {
			stopRelay()
			<-relayDone
		}
	}()
	for {
		var row struct {
			State string `bson:"state"`
		}
		if err := collection.FindOne(ctx, bson.M{"message_id": eventID}).Decode(&row); err != nil {
			t.Fatal(err)
		}
		if row.State == "published" {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("original report was not confirmed", ctx.Err())
		}
		time.Sleep(20 * time.Millisecond)
	}
	stopRelay()
	if err := <-relayDone; err != nil {
		t.Fatal(err)
	}
	stopped = true
	if err := publisher.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	producer.Stop()
	if hotProofNSQDepth(t, nsqHTTP) != 1 {
		t.Fatal("original report not present in owned broker before loss")
	}
	if _, err := store.FindByReportID(ctx, artifact.ID().String()); err == nil {
		t.Fatal("attention ledger exists before event delivery")
	}
	var focused bool
	if err := sqlDB.QueryRowContext(ctx, "SELECT is_key_focus FROM testee WHERE id=?", testee.ID().Uint64()).Scan(&focused); err != nil || focused {
		t.Fatal("business effect applied before recovery", err)
	}
	restartedHTTP := hotProofRestartOwnedNSQ(t, container, owner)
	if hotProofNSQDepth(t, restartedHTTP) != 0 {
		t.Fatal("confirmed original report was not actually lost")
	}
	if calls.Load() != 0 || updates.Load() != 0 {
		t.Fatal("business application ran before recovery")
	}
	source, err := attentionprojection.NewMongoFactSource(mongoDB)
	if err != nil {
		t.Fatal(err)
	}
	manifestRaw := fmt.Sprintf("%s\t%s\t%d\thigh\ttrue\n", artifact.ID().String(), assessmentID.String(), testee.ID().Uint64())
	fingerprint := sha256.Sum256([]byte(manifestRaw))
	leaseRunner := subsystem.New(subsystem.Options{Component: "worker", Handle: &redisruntime.Handle{Family: redisruntime.FamilyLock, Namespace: "m6-attention-" + owner, Builder: keyspace.NewBuilderWithNamespace("m6-attention-" + owner), Client: statusClient, Configured: true, Available: true}, RenewalEnabled: true})
	newRecovery := func(fp string) *attentionprojection.FactReconciler {
		t.Helper()
		r, err := attentionprojection.NewFactReconciler(source, store, projector, leaseRunner, completedAt.Add(-time.Second), false, time.Minute, 20, attentionprojection.FactManifestGuard{ReportIDs: []string{artifact.ID().String()}, Fingerprint: fp}, logger)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if _, err := newRecovery(strings.Repeat("0", 64)).RunOnce(ctx); err == nil {
		t.Fatal("changed report manifest was applied")
	}
	if calls.Load() != 0 || updates.Load() != 0 {
		t.Fatal("refused manifest changed business state")
	}
	recovery := newRecovery(hex.EncodeToString(fingerprint[:]))
	result, err := recovery.RunOnce(ctx)
	if err != nil || result.Created != 1 || calls.Load() != 1 || updates.Load() != 1 {
		t.Fatalf("original fact recovery result=%+v calls=%d updates=%d err=%v", result, calls.Load(), updates.Load(), err)
	}
	if err := sqlDB.QueryRowContext(ctx, "SELECT is_key_focus FROM testee WHERE id=?", testee.ID().Uint64()).Scan(&focused); err != nil || !focused {
		t.Fatal("real testee business fact not recovered", err)
	}
	result, err = recovery.RunOnce(ctx)
	if err != nil || result.Created != 0 || result.Existing != 1 || calls.Load() != 1 || updates.Load() != 1 {
		t.Fatalf("same manifest repeated effect result=%+v calls=%d updates=%d err=%v", result, calls.Load(), updates.Load(), err)
	}

	// The original delivery may arrive after source reconstruction. It retains
	// its original event UUID; a second ledger/RPC is allowed, but the API must
	// not mutate an already-focused testee again. This is business idempotency,
	// not an exactly-once RPC or single-ledger-row claim.
	raw, err := exec.Command("docker", "inspect", container).Output()
	if err != nil {
		t.Fatal(err)
	}
	var broker []hotProofContainer
	if err := json.Unmarshal(raw, &broker); err != nil || len(broker) != 1 {
		t.Fatal("invalid owned broker state", err)
	}
	bindings := broker[0].NetworkSettings.Ports["4150/tcp"]
	if len(bindings) != 1 || bindings[0].HostIP != "127.0.0.1" {
		t.Fatal("broker lost loopback TCP isolation")
	}
	lateTCP := "127.0.0.1:" + bindings[0].HostPort
	consumer, err := nsq.NewConsumer(topic, "m6-attention-late", config)
	if err != nil {
		t.Fatal(err)
	}
	consumer.SetLogger(nil, nsq.LogLevelError)
	deliveries := make(chan error, 2)
	consumer.AddHandler(nsq.HandlerFunc(func(raw *nsq.Message) error {
		decoded, recognized, e := legacywire.Decode(raw.Body)
		if e == nil && (!recognized || decoded.UUID != eventID || decoded.Metadata["event_type"] != eventcatalog.InterpretationReportGenerated) {
			e = fmt.Errorf("late original identity changed")
		}
		if e == nil {
			e = handler(ctx, eventcatalog.InterpretationReportGenerated, decoded.Payload)
		}
		select {
		case deliveries <- e:
		case <-ctx.Done():
			return ctx.Err()
		}
		return e
	}))
	if err := consumer.ConnectToNSQD(lateTCP); err != nil {
		t.Fatal(err)
	}
	defer func() {
		consumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("late consumer did not stop")
		}
	}()
	late, err := nsq.NewProducer(lateTCP, config)
	if err != nil {
		t.Fatal(err)
	}
	late.SetLogger(nil, nsq.LogLevelError)
	defer late.Stop()
	for i := 0; i < 2; i++ {
		if err := late.Publish(topic, outboxRow.Payload); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-deliveries:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("late original delivery timeout")
		}
	}
	for !m6AttentionChannelSettled(t, restartedHTTP, consumer.Stats().MessagesFinished) {
		if ctx.Err() != nil {
			t.Fatal("late original FIN was not settled")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if calls.Load() != 2 || updates.Load() != 1 {
		t.Fatalf("late original changed business fact again: calls=%d successful testee updates=%d", calls.Load(), updates.Load())
	}
	record, err := store.GetByEventID(ctx, eventID)
	if err != nil || record.Status != attentionprojection.StatusSucceeded {
		t.Fatal("original identity ledger not settled", err)
	}
	var ledgerRows int
	if err := sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM interpretation_attention_projection WHERE report_id=? AND status='succeeded'", artifact.ID().String()).Scan(&ledgerRows); err != nil || ledgerRows != 2 {
		t.Fatalf("recovery and original event ledgers=%d err=%v", ledgerRows, err)
	}
	var finalRow struct {
		State   string `bson:"state"`
		Payload []byte `bson:"payload"`
	}
	if err := collection.FindOne(ctx, bson.M{"message_id": eventID}).Decode(&finalRow); err != nil || finalRow.State != "published" || string(finalRow.Payload) != string(outboxRow.Payload) {
		t.Fatal("recovery requeued or changed original Outbox", err)
	}
	for _, collectionName := range []string{"interpret_report_artifacts", "interpretation_runs"} {
		n, err := mongoDB.Collection(collectionName).CountDocuments(ctx, bson.M{})
		if err != nil || n != 1 {
			t.Fatalf("original lifecycle collection=%s rows=%d err=%v", collectionName, n, err)
		}
	}
	t.Logf("actual report transaction + SDK Relay confirmed -> broker same-ID SIGKILL137 depth1->0; original report manifest and real Redis lease -> real MySQL/gRPC testee fact; repeat+late original NSQ twice settled: RPC calls=2, successful business updates=1, succeeded ledger rows=2; original Outbox remains published; no model/regeneration/production call")
}

func m6AttentionChannelSettled(t *testing.T, address string, finished uint64) bool {
	t.Helper()
	client := http.Client{Timeout: 3 * time.Second}
	r, err := client.Get(address + "/stats?format=json")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var v struct {
		Topics []struct {
			Name     string `json:"topic_name"`
			Channels []struct {
				Name         string `json:"channel_name"`
				Depth        int64  `json:"depth"`
				InFlight     int64  `json:"in_flight_count"`
				MessageCount uint64 `json:"message_count"`
				RequeueCount uint64 `json:"requeue_count"`
			} `json:"channels"`
		} `json:"topics"`
	}
	if r.StatusCode != 200 || json.NewDecoder(io.LimitReader(r.Body, 65536)).Decode(&v) != nil {
		t.Fatal("bounded owned channel observation failed")
	}
	for _, topic := range v.Topics {
		if topic.Name == "qs.evaluation.lifecycle" {
			for _, channel := range topic.Channels {
				if channel.Name == "m6-attention-late" {
					return finished == 2 && channel.MessageCount == 2 && channel.Depth == 0 && channel.InFlight == 0 && channel.RequeueCount == 0
				}
			}
		}
	}
	return false
}
