//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	journey "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/assessmentintake"
	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	assessmentcache "github.com/FangcunMount/qs-server/internal/apiserver/cache/evaluation"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
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
	mysqldriver "github.com/go-sql-driver/mysql"
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

// A committed real AnswerSheet flows from the SDK Mongo row through NSQ's
// original consumer and Worker handler to the real MySQL Assessment transition
// and a second SDK row. A lost broker FIN redelivers the same message ID.
func TestStandardAnswerSheetToAssessmentAcrossNSQ(t *testing.T) {
	mongoURI, dsn, nsqAddress := os.Getenv("RM_QS_MONGO_URI"), os.Getenv("RM_QS_ASSESSMENT_DSN"), os.Getenv("RM_QS_NSQ_TCP")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if !strings.HasPrefix(mongoURI, "mongodb://mongo:27017/") || !strings.Contains(mongoURI, "replicaSet=rm-test") ||
		err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m4_qs_chain" || nsqAddress != "nsqd:4150" {
		t.Fatal("disposable Mongo, m4_qs_chain MySQL and nsqd:4150 required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	require.NoError(t, err)
	defer mongoClient.Disconnect(context.Background())
	mongoDB := mongoClient.Database("rm_qs_standard_chain")
	defer mongoDB.Drop(context.Background())
	require.NoError(t, mongoDB.CreateCollection(ctx, "rm_outbox"))
	mongoOutbox := mongoDB.Collection("rm_outbox")
	_, err = mongoOutbox.Indexes().CreateMany(ctx, sdkmongo.Indexes())
	require.NoError(t, err)
	sheetRepo, err := mongoanswersheet.NewRepository(mongoDB)
	require.NoError(t, err)
	mongoCatalog, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  evaluation:
    name: qs.evaluation.lifecycle
events:
  answersheet.submitted:
    topic: evaluation
    delivery: durable_outbox
    aggregate: AnswerSheet
    domain: survey
    handler: answersheet_submitted_handler
`))
	require.NoError(t, err)
	mongoStager, err := mongostandard.NewStager(mongoOutbox, eventcatalog.NewCatalog(mongoCatalog), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	mongoRunner := NewMongoRunner(mongoDB, MongoRunnerOptions{Boundary: "answersheet_submit_m4", Limiter: &transactionLimiterSpy{}})
	postCommitWake := standardoutbox.NewPostCommitWake()
	durable := appanswersheet.NewTransactionalSubmissionDurableStore(mongoRunner, sheetRepo, mongoStager, postCommitWake)
	admission, err := domainanswersheet.NewAssessmentAdmission("QNR-M4", "1.0.0", "scale", "", "", "MODEL-1", "1.0.0", "M4")
	require.NoError(t, err)
	sheet := standardSubmissionSheet(t, 90010003, "through NSQ", admission)
	submittedID := sheet.Events()[0].EventID()
	fingerprint, err := submitport.Fingerprint(sheet)
	require.NoError(t, err)
	_, existed, err := durable.CreateDurably(ctx, sheet, appanswersheet.DurableSubmitMeta{WriterID: 301, IdempotencyKey: "m4-chain", Fingerprint: fingerprint})
	require.NoError(t, err)
	require.False(t, existed)
	require.EqualValues(t, 1, countStandardDocs(t, ctx, mongoDB.Collection("answersheets"), bson.M{}))

	mysqlDB, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := mysqlDB.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, mysqlDB.AutoMigrate(&assessmentmysql.AssessmentPO{}))
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)
	mysqlCatalog, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  evaluation:
    name: qs.evaluation.lifecycle
events:
  evaluation.requested:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Assessment
    domain: evaluation
    handler: evaluation_requested_handler
`))
	require.NoError(t, err)
	mysqlStager, err := mysqlstandard.NewStager(eventcatalog.NewCatalog(mysqlCatalog), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	assessmentService := appintake.NewService(assessmentcache.NewInvalidatingAssessmentRepository(assessmentmysql.NewAssessmentRepository(mysqlDB), nil), proofModelValidator{}, NewMySQLRunner(mysqlDB), mysqlStager)
	ensure := journey.NewService(nil, nil, nil, nil, assessmentService, nil, sheetRepo)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	grpcservice.NewAssessmentIntakeService(ensure, assessmentService, nil).RegisterService(server)
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	defer func() { server.Stop(); require.NoError(t, <-serverDone) }()
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	handler, ok := handlers.NewRegistry().Create("answersheet_submitted_handler", &handlers.Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AssessmentIntakeClient: proofIntakeGRPCClient{pb.NewAssessmentIntakeServiceClient(conn)},
	})
	require.True(t, ok)
	config := nsq.NewConfig()
	config.HeartbeatInterval, config.MsgTimeout = time.Second, time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	const topic, channel = "qs.evaluation.lifecycle", "rm-qs-standard-chain"
	consumer, err := nsq.NewConsumer(topic, channel, config)
	require.NoError(t, err)
	consumer.SetLogger(nil, nsq.LogLevelError)
	type delivery struct {
		id       nsq.MessageID
		attempts uint16
		err      error
	}
	delivered := make(chan delivery, 4)
	consumer.AddHandler(nsq.HandlerFunc(func(raw *nsq.Message) error {
		decoded, recognized, decodeErr := messaging.DecodeMessagePayload(raw.Body)
		if decodeErr == nil && !recognized {
			decodeErr = fmt.Errorf("standard message lost original NSQ envelope")
		}
		if decodeErr == nil && (decoded.UUID != submittedID || decoded.Metadata["event_type"] != "answersheet.submitted") {
			decodeErr = fmt.Errorf("standard message changed AnswerSheet event identity")
		}
		if decodeErr == nil {
			decodeErr = handler(ctx, "answersheet.submitted", decoded.Payload)
		}
		select {
		case delivered <- delivery{id: raw.ID, attempts: raw.Attempts, err: decodeErr}:
		case <-ctx.Done():
			return ctx.Err()
		}
		return decodeErr
	}))
	proxy, stopProxy := dropFirstFINProxy(t, ctx, nsqAddress)
	defer stopProxy()
	require.NoError(t, consumer.ConnectToNSQD(proxy))
	defer func() {
		consumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("consumer did not stop")
		}
	}()
	producer, err := nsq.NewProducer(nsqAddress, config)
	require.NoError(t, err)
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{topic: topic}, 1)
	require.NoError(t, err)
	defer func() { require.NoError(t, publisher.Drain(ctx)) }()
	mongoStore, err := sdkmongo.New(mongoOutbox)
	require.NoError(t, err)
	forwarder, err := relay.New(mongoStore, publisher, relay.Config{
		Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 5 * time.Second, Wake: postCommitWake.Wake(),
		PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
		Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {},
	})
	require.NoError(t, err)
	relayCtx, stopRelay := context.WithCancel(ctx)
	relayDone := make(chan error, 1)
	go func() { relayDone <- forwarder.Run(relayCtx) }()
	defer stopRelay()
	var first nsq.MessageID
	for i := range 2 {
		select {
		case got := <-delivered:
			require.NoError(t, got.err)
			var assessments, events int64
			require.NoError(t, mysqlDB.Model(&assessmentmysql.AssessmentPO{}).Count(&assessments).Error)
			require.NoError(t, mysqlDB.Table("rm_outbox").Where("event_type=?", "evaluation.requested").Count(&events).Error)
			require.EqualValues(t, 1, assessments)
			require.EqualValues(t, 1, events)
			if i == 0 {
				first = got.id
				require.EqualValues(t, 1, got.attempts)
			} else {
				require.Equal(t, first, got.id)
				require.Greater(t, got.attempts, uint16(1))
			}
		case <-ctx.Done():
			t.Fatal("standard AnswerSheet chain timed out", ctx.Err())
		}
	}
	stopRelay()
	select {
	case relayErr := <-relayDone:
		require.NoError(t, relayErr)
	case <-ctx.Done():
		t.Fatal("standard Mongo relay did not drain", ctx.Err())
	}
	require.NoError(t, publisher.Drain(ctx))
	require.EqualValues(t, 1, countStandardDocs(t, ctx, mongoOutbox, bson.M{"message_id": submittedID, "state": "published"}))
	var assessment assessmentmysql.AssessmentPO
	require.NoError(t, mysqlDB.Where("answer_sheet_id=?", uint64(90010003)).First(&assessment).Error)
	require.Equal(t, "submitted", assessment.Status)
	require.NotNil(t, assessment.EvaluationModelCode)
	require.Equal(t, "MODEL-1", *assessment.EvaluationModelCode)
	require.NotNil(t, assessment.EvaluationModelVersion)
	require.Equal(t, "1.0.0", *assessment.EvaluationModelVersion)
	mysqlStore, err := sdkmysql.New(sqlDB)
	require.NoError(t, err)
	mysqlClaims, err := mysqlStore.ClaimDue(ctx, 2, time.Minute)
	require.NoError(t, err)
	require.Len(t, mysqlClaims, 1)
	require.Equal(t, "evaluation.requested", mysqlClaims[0].Message.Input().EventType)
	require.Equal(t, "org:501", mysqlClaims[0].Message.Input().Scope)
	require.NoError(t, mysqlStore.Confirm(ctx, mysqlClaims[0]))
}
