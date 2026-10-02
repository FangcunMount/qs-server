//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration && m4_07_new_chain

package transaction

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	journey "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/assessmentintake"
	appanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongoanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"github.com/FangcunMount/reliable-messaging/transport"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	legacywire "github.com/FangcunMount/reliable-messaging/wire/legacy"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
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

// This small deterministic batch traces the standard M4 profile through the
// original business handler. It is not a capacity acceptance run.
func TestM407NewAnswerSheetBatchThroughStandardProfile(t *testing.T) {
	batchCount, spacing := m407LoadSettings(t)
	outageAfter := m407NSQOutageAfter(t, batchCount)
	mongoOutageAfter := m407MongoOutageAfter(t, batchCount)
	lostConfirm := os.Getenv("RM_QS_M407_LOST_CONFIRM") == "1"
	require.False(t, outageAfter > 0 && mongoOutageAfter > 0, "faults must be isolated")
	if lostConfirm {
		require.Equal(t, 1, batchCount)
		require.Zero(t, outageAfter)
		require.Zero(t, mongoOutageAfter)
	}
	runTimeout := m407RunTimeout(batchCount, spacing)
	if outageAfter > 0 || mongoOutageAfter > 0 || lostConfirm {
		runTimeout += 2 * time.Minute
	}
	const firstSheetID uint64 = 90010003
	mongoURI, dsn, nsqAddress := os.Getenv("RM_QS_MONGO_URI"), os.Getenv("RM_QS_ASSESSMENT_DSN"), os.Getenv("RM_QS_NSQ_TCP")
	catalogPath := os.Getenv("RM_QS_CATALOG")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if !strings.HasPrefix(mongoURI, "mongodb://mongo:27017/") || !strings.Contains(mongoURI, "replicaSet=rm-test") ||
		err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m4_qs_new_chain" ||
		nsqAddress != "nsqd:4150" || catalogPath != "/tmp/m4-07/configs/events.yaml" {
		t.Fatal("disposable Mongo rm-test, m4_qs_new_chain MySQL, nsqd:4150 and copied event catalog required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), runTimeout)
	defer cancel()
	commandMetricsPath := os.Getenv("RM_QS_M407_COMMAND_METRICS")
	connectURI := mongoURI
	var mongoFaultProxy *m407TCPFaultProxy
	if mongoOutageAfter > 0 {
		mongoFaultProxy = newM407TCPFaultProxy(t, "mongo:27017")
		connectURI = fmt.Sprintf("mongodb://%s/?replicaSet=rm-test&directConnection=true", mongoFaultProxy.Address())
	}
	clientOptions := options.Client().ApplyURI(connectURI)
	if mongoOutageAfter > 0 {
		clientOptions.SetServerSelectionTimeout(2 * time.Second)
	}
	var commandMetrics *m407MongoCommandMetrics
	if commandMetricsPath != "" {
		commandMetrics = newM407MongoCommandMetrics("m4_07_new_answer_chain", "rm_outbox")
		clientOptions.SetMonitor(commandMetrics.Monitor())
	}
	client, err := mongo.Connect(ctx, clientOptions)
	require.NoError(t, err)
	defer client.Disconnect(context.Background())
	mongoDB := client.Database("m4_07_new_answer_chain")
	if os.Getenv("RM_QS_M407_KEEP_MONGO_PROFILE") != "1" {
		defer mongoDB.Drop(context.Background())
	}
	require.NoError(t, mongoDB.CreateCollection(ctx, "rm_outbox"))
	mongoOutbox := mongoDB.Collection("rm_outbox")
	_, err = mongoOutbox.Indexes().CreateMany(ctx, sdkmongo.Indexes())
	require.NoError(t, err)
	sheetRepo, err := mongoanswersheet.NewRepository(mongoDB)
	require.NoError(t, err)
	catalogConfig, err := eventcatalog.Load(catalogPath)
	require.NoError(t, err)
	catalog := eventcatalog.NewCatalog(catalogConfig)
	mysqlDB, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := mysqlDB.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, mysqlDB.AutoMigrate(&assessmentmysql.AssessmentPO{}))
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)
	mysqlStager, err := mysqlstandard.NewStager(catalog, eventruntime.SourceAPIServer)
	require.NoError(t, err)
	assessmentService := appintake.NewService(assessmentmysql.NewAssessmentRepository(mysqlDB), proofModelValidator{}, NewMySQLRunner(mysqlDB), mysqlStager)
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
	nsqConfig := nsq.NewConfig()
	nsqConfig.HeartbeatInterval, nsqConfig.ReadTimeout, nsqConfig.WriteTimeout = time.Second, 3*time.Second, time.Second
	const topic, channel = "qs.evaluation.lifecycle", "m4-07-new-answer-chain"
	expectedIDs := make(map[string]struct{}, batchCount)
	for i := range batchCount {
		expectedIDs[m407AnswerEventID(firstSheetID+uint64(i))] = struct{}{}
	}
	consumer, err := nsq.NewConsumer(topic, channel, nsqConfig)
	require.NoError(t, err)
	consumer.SetLogger(nil, nsq.LogLevelError)
	consumer.ChangeMaxInFlight(16)
	if lostConfirm {
		consumer.ChangeMaxInFlight(1)
	}
	handlerSuccess := make(chan m407Handled, batchCount*2)
	consumer.AddHandler(nsq.HandlerFunc(func(raw *nsq.Message) error {
		decoded, recognized, decodeErr := legacywire.Decode(raw.Body)
		expected := false
		if decodeErr == nil && recognized {
			_, expected = expectedIDs[decoded.UUID]
		}
		if decodeErr == nil && (!recognized || !expected || decoded.Metadata["event_type"] != "answersheet.submitted") {
			decodeErr = fmt.Errorf("standard profile changed answer event identity")
		}
		if decodeErr == nil {
			decodeErr = handler(ctx, "answersheet.submitted", decoded.Payload)
		}
		if decodeErr == nil {
			select {
			case handlerSuccess <- m407Handled{EventID: decoded.UUID, BrokerAt: time.Unix(0, raw.Timestamp), At: time.Now()}:
			default:
			}
		}
		return decodeErr
	}))
	require.NoError(t, consumer.ConnectToNSQD(nsqAddress))
	defer func() { consumer.Stop(); <-consumer.StopChan }()
	legacyPublisher, err := messagingruntime.NewSDKNSQWirePublisher(nsqAddress)
	require.NoError(t, err)
	defer legacyPublisher.Close()
	publisherAddress := nsqAddress
	var faultProxy *m407TCPFaultProxy
	if outageAfter > 0 {
		faultProxy = newM407TCPFaultProxy(t, publisherAddress)
		publisherAddress = faultProxy.Address()
	}
	producer, err := nsq.NewProducer(publisherAddress, nsqConfig)
	require.NoError(t, err)
	producer.SetLogger(nil, nsq.LogLevelError)
	defer producer.Stop()
	registry, err := eventcatalog.NewEffectiveRegistry(catalog, eventcatalog.DefaultSpecs())
	require.NoError(t, err)
	routes := make(map[string]string)
	for _, evt := range registry.Snapshot() {
		if evt.OutboxProfile == eventcatalog.OutboxProfileMongoDomain {
			routes[evt.Topic] = evt.Topic
		}
	}
	basePublisher, err := sdknsq.New(producer, routes, 16)
	require.NoError(t, err)
	var publisher transport.Publisher = basePublisher
	var lostConfirmPublisher *m407SDKLostConfirmPublisher
	if lostConfirm {
		lostConfirmPublisher = &m407SDKLostConfirmPublisher{Publisher: basePublisher, accepted: make(chan string, 1)}
		publisher = lostConfirmPublisher
	}
	defer func() { require.NoError(t, basePublisher.Drain(ctx)) }()
	store, err := sdkmongo.New(mongoOutbox)
	require.NoError(t, err)
	stager, err := mongostandard.NewStager(mongoOutbox, catalog, eventruntime.SourceAPIServer)
	require.NoError(t, err)
	status, err := mongostandard.NewStatusReader(mongoOutbox)
	require.NoError(t, err)
	wake := standardoutbox.NewPostCommitWake()
	supervisor, err := standardoutbox.NewRelaySupervisor(standardoutbox.SupervisorOptions{
		Name: "mongo-domain-events", InitialBackoff: 500 * time.Millisecond, MaxBackoff: 30 * time.Second,
		NewRelay: func(observe relay.Observer) (standardoutbox.RelayRunner, error) {
			return relay.New(store, publisher, relay.Config{
				Concurrency: 16, PollInterval: time.Second, Lease: 30 * time.Second,
				PublishTimeout: 10 * time.Second, WriteTimeout: 5 * time.Second,
				Wake: wake.Wake(), Retry: standardoutbox.SDKRetryPolicy(), Observe: observe,
			})
		},
	})
	require.NoError(t, err)
	profile := eventsubsystem.StandardProfile{
		Binding:    appEventing.ProfileBinding{Stager: stager, PostCommit: wake},
		Supervisor: supervisor, Drain: basePublisher.Drain, DrainTimeout: 15 * time.Second,
		Status: appEventing.NamedOutboxStatusReader{Name: "mongo-domain-events", Reader: status},
	}
	selected, err := eventsubsystem.NewWithStandardProfiles(eventsubsystem.Options{
		MongoDB: mongoDB, Catalog: catalog, WirePublisher: legacyPublisher, PublisherMode: eventruntime.PublishModeMQ,
		Consumers: map[string]eventsubsystem.ConsumerOptions{"modelcatalog.hot_rank_projection": {Enabled: false}},
	}, map[eventcatalog.OutboxProfile]eventsubsystem.StandardProfile{eventcatalog.OutboxProfileMongoDomain: profile})
	require.NoError(t, err)
	defer func() { require.NoError(t, selected.Close()) }()
	binding := selected.Profile(eventcatalog.OutboxProfileMongoDomain)
	require.NotNil(t, binding.Stager)
	require.NotNil(t, binding.PostCommit)
	durable := appanswersheet.NewTransactionalSubmissionDurableStore(NewMongoRunner(mongoDB, MongoRunnerOptions{
		Boundary: "m4_07_new_answer_submit", Limiter: &transactionLimiterSpy{},
	}), sheetRepo, binding.Stager, binding.PostCommit)
	require.NoError(t, selected.Start(ctx))
	admission, err := domainanswersheet.NewAssessmentAdmission("QNR-M4", "1.0.0", "scale", "", "", "MODEL-1", "1.0.0", "M4")
	require.NoError(t, err)
	ref, err := domainanswersheet.NewQuestionnaireRef("QNR-M4", "1.0.0", "M4")
	require.NoError(t, err)
	submission, err := domainanswersheet.NewSubmissionContext(
		actor.NewFillerRef(301, actor.FillerTypeSelf), actor.NewTesteeRef(meta.FromUint64(401)),
		meta.FromUint64(501), "task-m4", admission,
	)
	require.NoError(t, err)
	answer, err := domainanswersheet.NewAnswer(meta.NewCode("Q1"), domainquestionnaire.TypeText, domainanswersheet.NewStringValue("through NSQ"), 0)
	require.NoError(t, err)
	fixedAt := time.Date(2026, 9, 23, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	manifest := sha256.New()
	returnedAt := make(map[string]time.Time, batchCount)
	var recoveryStarted time.Time
	for i := range batchCount {
		sheetID := firstSheetID + uint64(i)
		submittedAt := fixedAt.Add(time.Duration(i) * time.Second)
		sheet, err := domainanswersheet.Submit(meta.FromUint64(sheetID), ref, submission, []domainanswersheet.Answer{answer}, submittedAt)
		require.NoError(t, err)
		eventID := m407AnswerEventID(sheetID)
		require.NoError(t, sheet.SetM407PendingEventIdentity(eventID, submittedAt))
		eventJSON, err := json.Marshal(sheet.Events()[0])
		require.NoError(t, err)
		eventHash := sha256.Sum256(eventJSON)
		fingerprint, err := submitport.Fingerprint(sheet)
		require.NoError(t, err)
		_, err = fmt.Fprintf(manifest, "%d|%s|%s|%x\n", sheetID, eventID, fingerprint, eventHash)
		require.NoError(t, err)
		if mongoOutageAfter > 0 && i == mongoOutageAfter {
			_, _, err = durable.CreateDurably(ctx, sheet, appanswersheet.DurableSubmitMeta{
				WriterID: 301, IdempotencyKey: fmt.Sprintf("m4-07-chain-%d", sheetID), Fingerprint: fingerprint,
			})
			require.Error(t, err, "disconnected Mongo transaction must not accept the next answer")
			mongoFaultProxy.SetAvailable(true)
			recoveryStarted = time.Now()
			require.Eventually(t, func() bool {
				answers, answerErr := mongoDB.Collection("answersheets").CountDocuments(ctx, bson.M{})
				messages, messageErr := mongoOutbox.CountDocuments(ctx, bson.M{})
				return answerErr == nil && messageErr == nil &&
					answers == int64(mongoOutageAfter) && messages == int64(mongoOutageAfter)
			}, 10*time.Second, 20*time.Millisecond)
			t.Logf("m4_07_fault stream=new_mongo phase=mongo_reconnected rejected_sheet=%d committed_prefix=%d", sheetID, mongoOutageAfter)
		}
		_, existed, err := durable.CreateDurably(ctx, sheet, appanswersheet.DurableSubmitMeta{
			WriterID: 301, IdempotencyKey: fmt.Sprintf("m4-07-chain-%d", sheetID), Fingerprint: fingerprint,
		})
		require.NoError(t, err)
		require.False(t, existed)
		returnedAt[eventID] = time.Now()
		if outageAfter > 0 && i+1 == outageAfter {
			require.Eventually(t, func() bool {
				return countStandardDocs(t, ctx, mongoOutbox, bson.M{"state": "published"}) == int64(outageAfter)
			}, 20*time.Second, 20*time.Millisecond)
			faultProxy.SetAvailable(false)
			t.Logf("m4_07_fault stream=new_mongo phase=nsq_producer_down committed_prefix=%d", outageAfter)
		}
		if mongoOutageAfter > 0 && i+1 == mongoOutageAfter {
			require.Eventually(t, func() bool {
				return countStandardDocs(t, ctx, mongoOutbox, bson.M{"state": "published"}) == int64(mongoOutageAfter)
			}, 20*time.Second, 20*time.Millisecond)
			mongoFaultProxy.SetAvailable(false)
			t.Logf("m4_07_fault stream=new_mongo phase=mongo_down committed_prefix=%d", mongoOutageAfter)
		}
		if i+1 < batchCount {
			time.Sleep(spacing)
		}
	}
	if outageAfter > 0 {
		time.Sleep(2 * time.Second)
		total := countStandardDocs(t, ctx, mongoOutbox, bson.M{})
		published := countStandardDocs(t, ctx, mongoOutbox, bson.M{"state": "published"})
		require.EqualValues(t, batchCount, total)
		require.EqualValues(t, outageAfter, published)
		t.Logf("m4_07_fault stream=new_mongo phase=before_recovery committed=%d published=%d durable_backlog=%d", total, published, total-published)
		recoveryStarted = time.Now()
		faultProxy.SetAvailable(true)
	}
	require.EqualValues(t, batchCount, countStandardDocs(t, ctx, mongoDB.Collection("answersheets"), bson.M{}))
	require.EqualValues(t, batchCount, countStandardDocs(t, ctx, mongoOutbox, bson.M{}))
	if lostConfirm {
		select {
		case id := <-lostConfirmPublisher.accepted:
			require.Equal(t, m407AnswerEventID(firstSheetID), id)
		case <-ctx.Done():
			t.Fatal("standard publisher never accepted the injected message", ctx.Err())
		}
		require.Eventually(t, func() bool {
			var row struct {
				State         string `bson:"state"`
				FailureCount  uint64 `bson:"failure_count"`
				LastErrorCode string `bson:"last_error_code"`
			}
			err := mongoOutbox.FindOne(ctx, bson.M{"message_id": m407AnswerEventID(firstSheetID)}).Decode(&row)
			return err == nil && row.State == "retry_wait" && row.FailureCount == 1 && row.LastErrorCode == "publish_unknown"
		}, 5*time.Second, 20*time.Millisecond)
		t.Logf("m4_07_fault stream=new_mongo phase=sender_unknown_persisted event_id=%s", m407AnswerEventID(firstSheetID))
	}
	expectedDeliveries := batchCount
	if lostConfirm {
		expectedDeliveries++
	}
	handled := make(map[string]m407Handled, batchCount)
	received := 0
	for len(handled) < batchCount || lostConfirm && received < expectedDeliveries {
		select {
		case result := <-handlerSuccess:
			received++
			if _, duplicate := handled[result.EventID]; duplicate && !lostConfirm && mongoOutageAfter == 0 {
				t.Fatalf("standard chain handled event %s twice", result.EventID)
			} else if !duplicate {
				handled[result.EventID] = result
			}
		case <-ctx.Done():
			t.Fatal("standard answer chain did not reach all Worker handlers", ctx.Err(), len(handled))
		}
	}
	require.Len(t, handled, batchCount)
	if lostConfirm {
		t.Logf("m4_07_fault stream=new_mongo phase=confirmed_to_sender_unknown event_id=%s broker_acceptances=%d", m407AnswerEventID(firstSheetID), expectedDeliveries)
	}
	deadline := time.Now().Add(10 * time.Second)
	if outageAfter > 0 || mongoOutageAfter > 0 {
		deadline = recoveryStarted.Add(120 * time.Second)
	}
	for countStandardDocs(t, ctx, mongoOutbox, bson.M{"state": "published"}) != int64(batchCount) {
		if time.Now().After(deadline) {
			t.Fatal("standard Mongo Outbox batch did not all publish")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var assessments, mysqlEvents int64
	require.NoError(t, mysqlDB.Model(&assessmentmysql.AssessmentPO{}).Count(&assessments).Error)
	require.NoError(t, mysqlDB.Table("rm_outbox").Where("event_type=?", "evaluation.requested").Count(&mysqlEvents).Error)
	require.EqualValues(t, batchCount, assessments)
	require.EqualValues(t, batchCount, mysqlEvents)
	var assessmentSheetIDs []uint64
	require.NoError(t, mysqlDB.Model(&assessmentmysql.AssessmentPO{}).Order("answer_sheet_id").Pluck("answer_sheet_id", &assessmentSheetIDs).Error)
	require.Len(t, assessmentSheetIDs, batchCount)
	for i, sheetID := range assessmentSheetIDs {
		require.Equal(t, firstSheetID+uint64(i), sheetID)
	}
	var stats m407ChannelStats
	if mongoOutageAfter > 0 {
		// A lost database writeback can cause another physical delivery.
		// Accept extras only after NSQ has FINed every channel message; the
		// persisted Assessment and next Outbox must remain one each.
		stats = waitM407NSQDrainAtLeast(t, ctx, topic, channel, batchCount)
	} else {
		stats = waitM407NSQDrain(t, ctx, topic, channel, expectedDeliveries)
	}
	if mongoOutageAfter > 0 {
		for received < int(stats.MessageCount) {
			select {
			case result := <-handlerSuccess:
				received++
				if _, known := handled[result.EventID]; !known {
					t.Fatalf("standard chain handled unexpected event %s after drain", result.EventID)
				}
			case <-ctx.Done():
				t.Fatal("standard answer chain missed a broker delivery after Mongo recovery", ctx.Err())
			}
		}
		t.Logf("m4_07_fault stream=new_mongo phase=delivery_reconciled unique=%d broker_channel_messages=%d repeat_handler_successes=%d nsq_requeue=%d nsq_timeout=%d", batchCount, stats.MessageCount, received-batchCount, stats.RequeueCount, stats.TimeoutCount)
	}
	if outageAfter > 0 {
		require.LessOrEqual(t, time.Since(recoveryStarted), 120*time.Second)
		t.Logf("m4_07_fault stream=new_mongo phase=drained committed=%d published=%d recovery_ms=%d", batchCount, batchCount, time.Since(recoveryStarted).Milliseconds())
	}
	if mongoOutageAfter > 0 {
		require.LessOrEqual(t, time.Since(recoveryStarted), 120*time.Second)
		t.Logf("m4_07_fault stream=new_mongo phase=mongo_recovered committed=%d published=%d recovery_ms=%d", batchCount, batchCount, time.Since(recoveryStarted).Milliseconds())
	}
	var maxHandlerLag time.Duration
	ids := make([]string, 0, batchCount)
	for id := range handled {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		result := handled[id]
		if lag := result.At.Sub(returnedAt[id]); lag > maxHandlerLag {
			maxHandlerLag = lag
		}
		var row struct {
			ConfirmedAt time.Time `bson:"transport_confirmed_at"`
		}
		require.NoError(t, mongoOutbox.FindOne(ctx, bson.M{"message_id": id}).Decode(&row))
		require.False(t, row.ConfirmedAt.IsZero())
		t.Logf("m4_07_timeline stream=new_mongo event_id=%s submit_returned_at=%s nsqd_message_at=%s outbox_transport_confirmed_at=%s handler_done_at=%s", id,
			m407FormatTime(returnedAt[id]), m407FormatTime(result.BrokerAt), m407FormatTime(row.ConfirmedAt), m407FormatTime(result.At))
	}
	var submitRate float64
	if batchCount > 1 {
		first := returnedAt[m407AnswerEventID(firstSheetID)]
		last := returnedAt[m407AnswerEventID(firstSheetID+uint64(batchCount-1))]
		submitRate = float64(batchCount-1) / last.Sub(first).Seconds()
	}
	t.Logf("standard full chain batch diagnostic: count=%d spacing=%s submit_return_rate_per_sec=%.3f manifest_sha256=%x nsq_delivery=%d nsq_fin_samples=%d nsq_e2e_percentiles_ns=%v nsq_requeue=%d nsq_timeout=%d max_submit_return_to_worker_handler=%s", batchCount, spacing, submitRate, manifest.Sum(nil), stats.MessageCount, stats.E2EProcessingLatency.Count, stats.E2EProcessingLatency.Percentiles, stats.RequeueCount, stats.TimeoutCount, maxHandlerLag)
	if commandMetrics != nil {
		require.NoError(t, commandMetrics.Save(commandMetricsPath))
	}
}

// The real NSQ adapter confirms the first publish; this test-only wrapper
// hides that result from the Relay so it retries the unchanged message.
type m407SDKLostConfirmPublisher struct {
	transport.Publisher
	once     sync.Once
	accepted chan string
}

func (p *m407SDKLostConfirmPublisher) Publish(ctx context.Context, item message.Message) transport.Result {
	result := p.Publisher.Publish(ctx, item)
	if result.Outcome != transport.Confirmed {
		return result
	}
	injected := false
	p.once.Do(func() {
		injected = true
		p.accepted <- item.Input().ID
	})
	if injected {
		return transport.Result{Outcome: transport.Unknown}
	}
	return result
}

type m407Handled struct {
	EventID  string
	BrokerAt time.Time
	At       time.Time
}

func m407FormatTime(at time.Time) string {
	return at.In(time.FixedZone("UTC+8", 8*60*60)).Format(time.RFC3339Nano)
}

func m407LoadSettings(t *testing.T) (int, time.Duration) {
	t.Helper()
	count := 16
	if raw := os.Getenv("RM_QS_M407_BATCH_COUNT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		require.NoError(t, err)
		count = parsed
	}
	require.GreaterOrEqual(t, count, 1)
	require.LessOrEqual(t, count, 4096)
	spacingMS := 0
	if raw := os.Getenv("RM_QS_M407_SPACING_MS"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		require.NoError(t, err)
		spacingMS = parsed
	}
	require.GreaterOrEqual(t, spacingMS, 0)
	require.LessOrEqual(t, spacingMS, 150)
	return count, time.Duration(spacingMS) * time.Millisecond
}

func m407RunTimeout(count int, spacing time.Duration) time.Duration {
	if count <= 256 {
		return 45 * time.Second
	}
	return time.Duration(count)*spacing + 5*time.Minute
}

func m407AnswerEventID(sheetID uint64) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("m4-07-answer:%d", sheetID))).String()
}

type m407ChannelStats struct {
	ChannelName          string `json:"channel_name"`
	Depth                int64  `json:"depth"`
	InFlightCount        int64  `json:"in_flight_count"`
	DeferredCount        int64  `json:"deferred_count"`
	MessageCount         int64  `json:"message_count"`
	RequeueCount         int64  `json:"requeue_count"`
	TimeoutCount         int64  `json:"timeout_count"`
	E2EProcessingLatency struct {
		Count       int                  `json:"count"`
		Percentiles []map[string]float64 `json:"percentiles"`
	} `json:"e2e_processing_latency"`
}

func waitM407NSQDrain(t *testing.T, ctx context.Context, topic, channel string, count int) m407ChannelStats {
	return waitM407NSQDrainCount(t, ctx, topic, channel, count, false)
}

func waitM407NSQDrainAtLeast(t *testing.T, ctx context.Context, topic, channel string, count int) m407ChannelStats {
	return waitM407NSQDrainCount(t, ctx, topic, channel, count, true)
}

func waitM407NSQDrainCount(t *testing.T, ctx context.Context, topic, channel string, count int, allowExtra bool) m407ChannelStats {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://nsqd:4151/stats?format=json", nil)
		require.NoError(t, err)
		response, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
		require.NoError(t, err)
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			t.Fatalf("NSQ stats returned HTTP %d", response.StatusCode)
		}
		var snapshot struct {
			Topics []struct {
				TopicName string             `json:"topic_name"`
				Channels  []m407ChannelStats `json:"channels"`
			} `json:"topics"`
		}
		err = json.NewDecoder(response.Body).Decode(&snapshot)
		response.Body.Close()
		require.NoError(t, err)
		for _, foundTopic := range snapshot.Topics {
			if foundTopic.TopicName != topic {
				continue
			}
			for _, stats := range foundTopic.Channels {
				if stats.ChannelName != channel {
					continue
				}
				countMatches := stats.MessageCount == int64(count)
				if allowExtra {
					countMatches = stats.MessageCount >= int64(count)
				}
				if countMatches && stats.E2EProcessingLatency.Count == int(stats.MessageCount) && stats.Depth == 0 && stats.InFlightCount == 0 && stats.DeferredCount == 0 {
					if !allowExtra {
						require.Zero(t, stats.RequeueCount)
						require.Zero(t, stats.TimeoutCount)
					}
					require.Len(t, stats.E2EProcessingLatency.Percentiles, 3)
					return stats
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("NSQ channel %s/%s did not drain %d messages: %+v", topic, channel, count, snapshot.Topics)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
