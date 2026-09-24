//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	journey "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/assessmentintake"
	apphotrank "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/hotrank"
	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	assessmentcache "github.com/FangcunMount/qs-server/internal/apiserver/cache/evaluation"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	redishotrank "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog/hotrank"
	mongoanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	hotrankport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/hotrank"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/redisadapter"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/alicebob/miniredis/v2"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/nsqio/go-nsq"
	redis "github.com/redis/go-redis/v9"
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
// and a second SDK row. The independent hot-rank channel projects the same
// event once despite a temporary Redis outage and a later lost acknowledgement.
// Lock contention and a lost broker FIN redeliver to the Worker without an
// extra Assessment.
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
	var ensureCalls atomic.Int32
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if strings.HasSuffix(info.FullMethod, "/EnsureAssessment") {
			ensureCalls.Add(1)
		}
		return next(ctx, request)
	}))
	grpcservice.NewAssessmentIntakeService(ensure, assessmentService, nil).RegisterService(server)
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	defer func() { server.Stop(); require.NoError(t, <-serverDone) }()
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer redisClient.Close()
	keys := keyspace.NewBuilderWithNamespace(keyspace.ComposeNamespace("m4-chain", "cache:lock"))
	redisHandle := &redisruntime.Handle{Client: redisClient, Builder: keys}
	lockManager := redisadapter.NewManager("worker", "lock_lease", redisHandle)
	lockRunner := locksubsystem.New(locksubsystem.Options{Component: "worker", Handle: redisHandle, Manager: lockManager, RenewalEnabled: true})
	capability, ok := locklease.Lookup(locklease.WorkloadAnswersheetProcessing)
	require.True(t, ok)
	holderLease, acquired, err := lockManager.AcquireSpec(ctx, capability.Spec, "answersheet:processing:90010003")
	require.NoError(t, err)
	require.True(t, acquired)
	require.NotNil(t, holderLease)
	handler, ok := handlers.NewRegistry().Create("answersheet_submitted_handler", &handlers.Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AssessmentIntakeClient: proofIntakeGRPCClient{pb.NewAssessmentIntakeServiceClient(conn)},
		LockManager: lockManager, LockRunner: lockRunner, LockKeyBuilder: keys,
	})
	require.True(t, ok)
	config := nsq.NewConfig()
	config.HeartbeatInterval, config.MsgTimeout = time.Second, time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	config.DefaultRequeueDelay = time.Second
	const topic, channel = "qs.evaluation.lifecycle", "rm-qs-standard-chain"
	consumer, err := nsq.NewConsumer(topic, channel, config)
	require.NoError(t, err)
	consumer.SetLogger(nil, nsq.LogLevelError)
	type delivery struct {
		id       nsq.MessageID
		attempts uint16
		err      error
	}
	delivered := make(chan delivery, 5)
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
	// Keep the hot-rank Redis failure independent of the Worker's lock Redis.
	hotrankMini := miniredis.RunT(t)
	hotrankMini.SetError("LOADING hot-rank Redis temporarily unavailable")
	hotrankRedisClient := redis.NewClient(&redis.Options{Addr: hotrankMini.Addr()})
	defer hotrankRedisClient.Close()
	hotrankProjection := redishotrank.NewRedisScaleHotRankProjection(hotrankRedisClient, keys)
	hotrankHandler := apphotrank.NewEventConsumer(hotrankProjection)
	hotrankConsumer, err := nsq.NewConsumer(topic, "qs-apiserver-modelcatalog-hot-rank-v1", config)
	require.NoError(t, err)
	hotrankConsumer.SetLogger(nil, nsq.LogLevelError)
	hotrankDelivered := make(chan delivery, 3)
	var hotrankCalls atomic.Int32
	hotrankConsumer.AddHandler(nsq.HandlerFunc(func(raw *nsq.Message) error {
		decoded, recognized, handleErr := messaging.DecodeMessagePayload(raw.Body)
		if handleErr == nil && !recognized {
			handleErr = fmt.Errorf("hot-rank channel lost original NSQ envelope")
		}
		if handleErr == nil && (decoded.UUID != submittedID || decoded.Metadata["event_type"] != "answersheet.submitted") {
			handleErr = fmt.Errorf("hot-rank channel changed AnswerSheet event identity")
		}
		if handleErr == nil {
			handleErr = hotrankHandler(ctx, "answersheet.submitted", decoded.Payload)
		}
		if handleErr == nil && hotrankCalls.Add(1) == 1 {
			handleErr = fmt.Errorf("simulate lost hot-rank acknowledgement after projection")
		}
		select {
		case hotrankDelivered <- delivery{id: raw.ID, attempts: raw.Attempts, err: handleErr}:
		case <-ctx.Done():
			return ctx.Err()
		}
		return handleErr
	}))
	proxy, stopProxy := dropFirstFINProxy(t, ctx, nsqAddress)
	defer stopProxy()
	require.NoError(t, consumer.ConnectToNSQD(proxy))
	require.NoError(t, hotrankConsumer.ConnectToNSQD(nsqAddress))
	defer func() {
		consumer.Stop()
		hotrankConsumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("consumer did not stop")
		}
		select {
		case <-hotrankConsumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("hot-rank consumer did not stop")
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
	var firstHotrank nsq.MessageID
	select {
	case got := <-hotrankDelivered:
		require.ErrorContains(t, got.err, "LOADING hot-rank Redis temporarily unavailable")
		require.EqualValues(t, 1, got.attempts)
		firstHotrank = got.id
		hotrankMini.SetError("")
	case <-ctx.Done():
		t.Fatal("standard hot-rank Redis outage was not observed", ctx.Err())
	}
	var first nsq.MessageID
	for i := range 3 {
		select {
		case got := <-delivered:
			var assessments, events int64
			require.NoError(t, mysqlDB.Model(&assessmentmysql.AssessmentPO{}).Count(&assessments).Error)
			require.NoError(t, mysqlDB.Table("rm_outbox").Where("event_type=?", "evaluation.requested").Count(&events).Error)
			if i == 0 {
				require.True(t, errors.Is(got.err, handlers.ErrAnswerSheetProcessingInProgress), "first delivery must retry lock contention: %v", got.err)
				require.Zero(t, assessments)
				require.Zero(t, events)
				require.Zero(t, ensureCalls.Load())
				first = got.id
				require.EqualValues(t, 1, got.attempts)
				// The holder exits without release or a committed Assessment.
				mini.FastForward(capability.Spec.DefaultTTL)
			} else {
				require.NoError(t, got.err)
				require.EqualValues(t, 1, assessments)
				require.EqualValues(t, 1, events)
				require.Equal(t, first, got.id)
				require.EqualValues(t, i+1, got.attempts)
				require.EqualValues(t, i, ensureCalls.Load())
			}
		case <-ctx.Done():
			t.Fatal("standard AnswerSheet chain timed out", ctx.Err())
		}
	}
	for i := 1; i < 3; i++ {
		select {
		case got := <-hotrankDelivered:
			if i == 1 {
				require.ErrorContains(t, got.err, "lost hot-rank acknowledgement")
			} else {
				require.NoError(t, got.err)
			}
			require.Equal(t, firstHotrank, got.id)
			require.EqualValues(t, i+1, got.attempts)
		case <-ctx.Done():
			t.Fatal("standard hot-rank projection timed out", ctx.Err())
		}
	}
	hotrankEntries, err := hotrankProjection.Top(ctx, hotrankport.Query{WindowDays: 1, Limit: 5})
	require.NoError(t, err)
	require.Len(t, hotrankEntries, 1)
	require.Equal(t, "QNR-M4", hotrankEntries[0].QuestionnaireCode)
	require.EqualValues(t, 1, hotrankEntries[0].Score)
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
