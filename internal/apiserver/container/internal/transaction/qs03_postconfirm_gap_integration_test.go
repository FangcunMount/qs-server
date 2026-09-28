//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	journey "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/assessmentintake"
	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	assessmentcache "github.com/FangcunMount/qs-server/internal/apiserver/cache/evaluation"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/answersheetgap"
	mongoanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/go-sql-driver/mysql"
	"github.com/nsqio/go-nsq"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	qs03ProofDB      = "rm_qs03_postconfirm"
	qs03ProofSheetID = uint64(90010077)
	qs03ProofTopic   = "qs.evaluation.lifecycle"
)

func qs03ProofConnections(t *testing.T) (*mongo.Database, *gorm.DB, string) {
	t.Helper()
	mongoURI, mysqlDSN, nsqTCP := os.Getenv("RM_QS03_MONGO_URI"), os.Getenv("RM_QS03_MYSQL_DSN"), os.Getenv("RM_QS03_NSQ_TCP")
	parsed, err := mysql.ParseDSN(mysqlDSN)
	if !strings.Contains(mongoURI, "replicaSet=rm-gap") || !strings.Contains(mongoURI, "directConnection=true") ||
		err != nil || parsed.DBName != "m6_qs03_postconfirm" || parsed.Net != "tcp" || !strings.HasPrefix(parsed.Addr, "127.0.0.1:") || !strings.HasPrefix(nsqTCP, "127.0.0.1:") {
		t.Fatal("QS-03 proof requires disposable Mongo, MySQL and NSQ endpoints")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.Background())) })
	require.NoError(t, client.Ping(ctx, nil))
	db, err := gorm.Open(gormmysql.Open(mysqlDSN), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return client.Database(qs03ProofDB), db, nsqTCP
}

func qs03Catalog(t *testing.T, eventType string) eventcatalog.TopicResolver {
	t.Helper()
	config := `version: "1"
topics:
  evaluation:
    name: qs.evaluation.lifecycle
events:
  ` + eventType + `:
    topic: evaluation
    delivery: durable_outbox
    aggregate: ` + map[string]string{"answersheet.submitted": "AnswerSheet", "evaluation.requested": "Assessment"}[eventType] + `
    domain: evaluation
    handler: proof_handler
`
	parsed, err := eventcatalog.Parse([]byte(config))
	require.NoError(t, err)
	return eventcatalog.NewCatalog(parsed)
}

// The script starts disposable stores and a broker with a durable channel.
// This phase commits the real AnswerSheet transaction and lets SDK Relay
// confirm the original event before the script SIGKILLs the broker.
func TestQS03PostConfirmSeed(t *testing.T) {
	mongoDB, mysqlDB, nsqTCP := qs03ProofConnections(t)
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	require.NoError(t, mongoDB.CreateCollection(ctx, "rm_outbox"))
	outbox := mongoDB.Collection("rm_outbox")
	_, err := outbox.Indexes().CreateMany(ctx, sdkmongo.Indexes())
	require.NoError(t, err)
	sheetRepo, err := mongoanswersheet.NewRepository(mongoDB)
	require.NoError(t, err)
	stager, err := mongostandard.NewStager(outbox, qs03Catalog(t, "answersheet.submitted"), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	wake := standardoutbox.NewPostCommitWake()
	durable := appanswersheet.NewTransactionalSubmissionDurableStore(NewMongoRunner(mongoDB, MongoRunnerOptions{Boundary: "qs03_postconfirm", Limiter: &transactionLimiterSpy{}}), sheetRepo, stager, wake)
	admission, err := domainanswersheet.NewAssessmentAdmission("QNR-M4", "1.0.0", "scale", "", "", "MODEL-1", "1.0.0", "M4")
	require.NoError(t, err)
	sheet := standardSubmissionSheet(t, qs03ProofSheetID, "broker lost after confirm", admission)
	originalID := sheet.Events()[0].EventID()
	fingerprint, err := submitport.Fingerprint(sheet)
	require.NoError(t, err)
	_, existed, err := durable.CreateDurably(ctx, sheet, appanswersheet.DurableSubmitMeta{WriterID: 301, IdempotencyKey: "qs03-postconfirm", Fingerprint: fingerprint})
	require.NoError(t, err)
	require.False(t, existed)
	require.EqualValues(t, 1, countStandardDocs(t, ctx, mongoDB.Collection("answersheets"), bson.M{"domain_id": qs03ProofSheetID}))
	require.EqualValues(t, 1, countStandardDocs(t, ctx, outbox, bson.M{"message_id": originalID, "state": "pending"}))
	require.NoError(t, mysqlDB.AutoMigrate(&assessmentmysql.AssessmentPO{}))
	sqlDB, err := mysqlDB.DB()
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)
	var assessments int64
	require.NoError(t, mysqlDB.Table("assessment").Count(&assessments).Error)
	require.Zero(t, assessments)
	config := nsq.NewConfig()
	config.HeartbeatInterval = time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	producer, err := nsq.NewProducer(nsqTCP, config)
	require.NoError(t, err)
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{qs03ProofTopic: qs03ProofTopic}, 1)
	require.NoError(t, err)
	store, err := sdkmongo.New(outbox)
	require.NoError(t, err)
	forwarder, err := relay.New(store, publisher, relay.Config{
		Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 5 * time.Second, Wake: wake.Wake(),
		PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
		Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {},
	})
	require.NoError(t, err)
	relayCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- forwarder.Run(relayCtx) }()
	require.Eventually(t, func() bool {
		return countStandardDocs(t, ctx, outbox, bson.M{"message_id": originalID, "state": "published"}) == 1
	}, 20*time.Second, 50*time.Millisecond)
	stop()
	require.NoError(t, <-done)
	require.NoError(t, publisher.Drain(ctx))
	t.Logf("seed original_event_id=%s outbox=published assessment_count=0", originalID)
}

// The script runs this phase after SIGKILL and same-volume broker restart.
// It finds the missing business effect and invokes the original Worker handler
// with the immutable, frozen original event; no Outbox row is reset or replayed.
func TestQS03PostConfirmRecover(t *testing.T) {
	mongoDB, mysqlDB, _ := qs03ProofConnections(t)
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	scanner, err := answersheetgap.New(mongoDB, mysqlDB)
	require.NoError(t, err)
	var sheet struct {
		DurableAcceptance struct {
			EventID    string    `bson:"event_id"`
			AcceptedAt time.Time `bson:"accepted_at"`
		} `bson:"durable_acceptance"`
	}
	require.NoError(t, mongoDB.Collection("answersheets").FindOne(ctx, bson.M{"domain_id": qs03ProofSheetID}).Decode(&sheet))
	require.NotEmpty(t, sheet.DurableAcceptance.EventID)
	require.True(t, time.Since(sheet.DurableAcceptance.AcceptedAt) >= time.Second)
	page, err := scanner.ScanPage(ctx, 0, qs03ProofSheetID, time.Now().Add(-time.Second), 10)
	require.NoError(t, err)
	require.Len(t, page.Findings, 1)
	require.Equal(t, sheet.DurableAcceptance.EventID, page.Findings[0].EventID)
	require.Equal(t, answersheetgap.Missing, page.Findings[0].Disposition)
	var stored struct {
		MessageID    string `bson:"message_id"`
		State        string `bson:"state"`
		AttemptCount uint64 `bson:"attempt_count"`
		Payload      []byte `bson:"payload"`
	}
	require.NoError(t, mongoDB.Collection("rm_outbox").FindOne(ctx, bson.M{"message_id": sheet.DurableAcceptance.EventID}).Decode(&stored))
	require.Equal(t, "published", stored.State)
	require.EqualValues(t, 1, stored.AttemptCount)
	wire, recognized, err := legacy.Decode(stored.Payload)
	require.NoError(t, err)
	require.True(t, recognized)
	require.Equal(t, stored.MessageID, wire.UUID)
	sheetRepo, err := mongoanswersheet.NewRepository(mongoDB)
	require.NoError(t, err)
	stager, err := mysqlstandard.NewStager(qs03Catalog(t, "evaluation.requested"), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	intake := appintake.NewService(assessmentcache.NewInvalidatingAssessmentRepository(assessmentmysql.NewAssessmentRepository(mysqlDB), nil), proofModelValidator{}, NewMySQLRunner(mysqlDB), stager)
	ensure := journey.NewService(nil, nil, nil, nil, intake, nil, sheetRepo)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	grpcservice.NewAssessmentIntakeService(ensure, intake, nil).RegisterService(server)
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	defer func() { server.Stop(); require.NoError(t, <-serverDone) }()
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	handler, ok := handlers.NewRegistry().Create("answersheet_submitted_handler", &handlers.Dependencies{
		Logger:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
		AssessmentIntakeClient: proofIntakeGRPCClient{pb.NewAssessmentIntakeServiceClient(conn)},
	})
	require.True(t, ok)
	// Race two recovery invocations without the normal Redis processing lock.
	// A loser must recognize the winning committed submission. Neither call
	// may create a second Assessment or downstream evaluation intent.
	start := make(chan struct{})
	results := make(chan error, 2)
	var running sync.WaitGroup
	for range 2 {
		running.Add(1)
		go func() {
			defer running.Done()
			<-start
			results <- handler(ctx, "answersheet.submitted", wire.Payload)
		}()
	}
	close(start)
	running.Wait()
	close(results)
	concurrentErrors := 0
	for err := range results {
		if err != nil {
			concurrentErrors++
			t.Logf("concurrent recovery failed: %v", err)
		}
	}
	require.Zero(t, concurrentErrors, "both concurrent recoveries must converge")
	var afterRaceAssessments, afterRaceIntents int64
	require.NoError(t, mysqlDB.Table("assessment").Where("answer_sheet_id=?", qs03ProofSheetID).Count(&afterRaceAssessments).Error)
	require.NoError(t, mysqlDB.Table("rm_outbox").Where("event_type=?", "evaluation.requested").Count(&afterRaceIntents).Error)
	require.EqualValues(t, 1, afterRaceAssessments)
	require.EqualValues(t, 1, afterRaceIntents)
	require.NoError(t, handler(ctx, "answersheet.submitted", wire.Payload))
	require.NoError(t, handler(ctx, "answersheet.submitted", wire.Payload))
	var assessments []assessmentmysql.AssessmentPO
	require.NoError(t, mysqlDB.Where("answer_sheet_id=?", qs03ProofSheetID).Find(&assessments).Error)
	require.Len(t, assessments, 1)
	require.Equal(t, "submitted", assessments[0].Status)
	require.NotNil(t, assessments[0].EvaluationModelCode)
	require.Equal(t, "MODEL-1", *assessments[0].EvaluationModelCode)
	require.NotNil(t, assessments[0].EvaluationModelVersion)
	require.Equal(t, "1.0.0", *assessments[0].EvaluationModelVersion)
	var intents int64
	require.NoError(t, mysqlDB.Table("rm_outbox").Where("event_type=?", "evaluation.requested").Count(&intents).Error)
	require.EqualValues(t, 1, intents)
	page, err = scanner.ScanPage(ctx, 0, qs03ProofSheetID, time.Now().Add(-time.Second), 10)
	require.NoError(t, err)
	require.Len(t, page.Findings, 1)
	require.Equal(t, answersheetgap.Present, page.Findings[0].Disposition)
	require.EqualValues(t, assessments[0].ID, page.Findings[0].AssessmentID)
	require.EqualValues(t, 1, countStandardDocs(t, ctx, mongoDB.Collection("rm_outbox"), bson.M{"message_id": stored.MessageID, "state": "published", "attempt_count": stored.AttemptCount}))
	t.Logf("recovered original_event_id=%s assessment_id=%d evaluation_intents=1 concurrent_errors=%d", stored.MessageID, assessments[0].ID, concurrentErrors)
}
