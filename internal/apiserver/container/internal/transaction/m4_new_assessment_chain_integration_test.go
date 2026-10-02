//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration && m4_07_new_chain

package transaction

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"github.com/FangcunMount/reliable-messaging/transport"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	legacywire "github.com/FangcunMount/reliable-messaging/wire/legacy"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/nsqio/go-nsq"
	"github.com/stretchr/testify/require"
)

// The real intake service writes the standard MySQL Outbox in its host
// transaction. The real Worker handler has an isolated recording execution
// client, so this never invokes a model or claims evaluation completion.
func TestM407NewAssessmentBatchThroughStandardProfile(t *testing.T) {
	batchCount, spacing := m407LoadSettings(t)
	outageAfter := m407NSQOutageAfter(t, batchCount)
	mysqlOutageAfter := m407MySQLOutageAfter(t, batchCount)
	lostConfirm := os.Getenv("RM_QS_M407_LOST_CONFIRM") == "1"
	if lostConfirm {
		require.Equal(t, 1, batchCount)
		require.Zero(t, outageAfter)
		require.Zero(t, mysqlOutageAfter)
	}
	if mysqlOutageAfter > 0 {
		require.Zero(t, outageAfter)
	}
	runTimeout := m407RunTimeout(batchCount, spacing)
	if outageAfter > 0 || mysqlOutageAfter > 0 || lostConfirm {
		runTimeout += 2 * time.Minute
	}
	const firstSheetID uint64 = 90010003
	parsed, err := mysqldriver.ParseDSN(os.Getenv("RM_QS_ASSESSMENT_DSN"))
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m4_qs_new_chain" ||
		os.Getenv("RM_QS_NSQ_TCP") != "nsqd:4150" || os.Getenv("RM_QS_CATALOG") != "/tmp/m4-07/configs/events.yaml" {
		t.Fatal("disposable new-chain MySQL, NSQ and copied event catalog required")
	}
	parsed.DBName = "m4_qs_new_assessment"
	var mysqlFaultProxy *m407TCPFaultProxy
	if mysqlOutageAfter > 0 {
		mysqlFaultProxy = newM407TCPFaultProxy(t, parsed.Addr)
		parsed.Addr = mysqlFaultProxy.Address()
	}
	ctx, cancel := context.WithTimeout(t.Context(), runTimeout)
	defer cancel()
	db, closeMetrics, err := m407OpenMonitoredMySQL(parsed.FormatDSN())
	require.NoError(t, err)
	defer closeMetrics()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, db.AutoMigrate(&assessmentmysql.AssessmentPO{}))
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)
	catalogConfig, err := eventcatalog.Load(os.Getenv("RM_QS_CATALOG"))
	require.NoError(t, err)
	catalog := eventcatalog.NewCatalog(catalogConfig)

	nsqConfig := nsq.NewConfig()
	nsqConfig.HeartbeatInterval, nsqConfig.ReadTimeout, nsqConfig.WriteTimeout = time.Second, 3*time.Second, time.Second
	const topic, channel = "qs.evaluation.lifecycle", "m4-07-new-assessment"
	expectedByAssessment := make(map[uint64]uint64, batchCount)
	executed := make(chan uint64, batchCount*2)
	recorder := m407EvaluationRecorder{expected: expectedByAssessment, calls: executed}
	workerHandler, ok := handlers.NewRegistry().Create("evaluation_requested_handler", &handlers.Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), EvaluationWorkerClient: recorder,
	})
	require.True(t, ok)
	consumer, err := nsq.NewConsumer(topic, channel, nsqConfig)
	require.NoError(t, err)
	consumer.SetLogger(nil, nsq.LogLevelError)
	consumer.ChangeMaxInFlight(16)
	if lostConfirm {
		consumer.ChangeMaxInFlight(1)
	}
	handled := make(chan m407AssessmentHandled, batchCount*2)
	consumer.AddHandler(nsq.HandlerFunc(func(raw *nsq.Message) error {
		decoded, recognized, decodeErr := legacywire.Decode(raw.Body)
		if decodeErr != nil {
			return decodeErr
		}
		if !recognized || decoded.Metadata["event_type"] != "evaluation.requested" {
			return fmt.Errorf("unexpected standard MySQL assessment message")
		}
		var data eventpayload.EvaluationRequestedData
		env, err := handlers.ParseEventData(decoded.Payload, &data)
		if err != nil {
			return err
		}
		sheetID, err := strconv.ParseUint(data.AnswerSheetID, 10, 64)
		if err != nil {
			return err
		}
		expectedSheet, exists := expectedByAssessment[uint64(data.AssessmentID)]
		if !exists || expectedSheet != sheetID || env.ID != decoded.UUID || data.OrgID != 501 || data.TesteeID != 401 ||
			data.QuestionnaireCode != "QNR-M4" || data.QuestionnaireVer != "1.0.0" || data.ModelKind != "scale" ||
			data.ModelCode != "MODEL-1" || data.ModelVersion != "1.0.0" {
			return fmt.Errorf("standard MySQL assessment event does not match committed business key")
		}
		if err := workerHandler(ctx, "evaluation.requested", decoded.Payload); err != nil {
			return err
		}
		select {
		case handled <- m407AssessmentHandled{eventID: decoded.UUID, assessmentID: uint64(data.AssessmentID), sheetID: sheetID,
			brokerAt: time.Unix(0, raw.Timestamp), handlerDoneAt: time.Now()}:
		default:
		}
		return nil
	}))
	require.NoError(t, consumer.ConnectToNSQD(os.Getenv("RM_QS_NSQ_TCP")))
	defer func() { consumer.Stop(); <-consumer.StopChan }()
	legacyPublisher, err := messagingruntime.NewSDKNSQWirePublisher(os.Getenv("RM_QS_NSQ_TCP"))
	require.NoError(t, err)
	defer legacyPublisher.Close()
	publisherAddress := os.Getenv("RM_QS_NSQ_TCP")
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
		if evt.OutboxProfile == eventcatalog.OutboxProfileAssessmentMySQL {
			routes[evt.Topic] = evt.Topic
		}
	}
	basePublisher, err := sdknsq.New(producer, routes, 16)
	require.NoError(t, err)
	publisher := &m407ObservedSDKPublisher{Publisher: basePublisher, confirmed: make(map[string][]time.Time)}
	if lostConfirm {
		publisher.injectLostConfirm = true
		publisher.lostAccepted = make(chan string, 1)
	}
	defer func() { require.NoError(t, basePublisher.Drain(ctx)) }()
	store, err := sdkmysql.New(sqlDB)
	require.NoError(t, err)
	stager, err := mysqlstandard.NewStager(catalog, eventruntime.SourceAPIServer)
	require.NoError(t, err)
	status, err := mysqlstandard.NewStatusReader(sqlDB)
	require.NoError(t, err)
	wake := standardoutbox.NewPostCommitWake()
	supervisor, err := standardoutbox.NewRelaySupervisor(standardoutbox.SupervisorOptions{
		Name: "assessment-mysql-outbox", InitialBackoff: 500 * time.Millisecond, MaxBackoff: 30 * time.Second,
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
		Status: appEventing.NamedOutboxStatusReader{Name: "assessment-mysql-outbox", Reader: status},
	}
	selected, err := eventsubsystem.NewWithStandardProfiles(eventsubsystem.Options{
		MySQLDB: db, Catalog: catalog, WirePublisher: legacyPublisher, PublisherMode: eventruntime.PublishModeMQ,
		Consumers: map[string]eventsubsystem.ConsumerOptions{"modelcatalog.hot_rank_projection": {Enabled: false}},
	}, map[eventcatalog.OutboxProfile]eventsubsystem.StandardProfile{eventcatalog.OutboxProfileAssessmentMySQL: profile})
	require.NoError(t, err)
	defer func() { require.NoError(t, selected.Close()) }()
	binding := selected.Profile(eventcatalog.OutboxProfileAssessmentMySQL)
	require.NotNil(t, binding.Stager)
	require.NotNil(t, binding.PostCommit)
	commitObserver := newM407CommitObserver(NewMySQLRunner(db))
	service := appintake.NewService(assessmentmysql.NewAssessmentRepository(db), proofModelValidator{}, commitObserver,
		binding.Stager, appintake.WithPostCommitDispatcher(binding.PostCommit))
	require.NoError(t, selected.Start(ctx))
	kind, code, version := "scale", "MODEL-1", "1.0.0"
	manifest := sha256.New()
	created := make([]uint64, 0, batchCount)
	for i := range batchCount {
		sheetID := firstSheetID + uint64(i)
		command := appintake.CreateCommand{OrgID: 501, TesteeID: 401, AnswerSheetID: sheetID,
			QuestionnaireCode: "QNR-M4", QuestionnaireVersion: "1.0.0", OriginType: "adhoc",
			ModelKind: &kind, ModelCode: &code, ModelVersion: &version}
		_, err = fmt.Fprintf(manifest, "%d|501|401|QNR-M4|1.0.0|adhoc|scale|MODEL-1|1.0.0\n", sheetID)
		require.NoError(t, err)
		assessment, err := service.CreateForAnswerSheet(ctx, command)
		require.NoError(t, err)
		require.Equal(t, "pending", assessment.Status)
		created = append(created, assessment.ID)
		expectedByAssessment[assessment.ID] = sheetID
	}
	submitReturnedAt := make(map[uint64]time.Time, batchCount)
	var recoveryStarted time.Time
	var mysqlRecoveryStarted time.Time
	for i, id := range created {
		assessment, err := service.SubmitForEvaluation(context.WithValue(ctx, m407AssessmentCommitKey{}, id), id)
		require.NoError(t, err)
		require.Equal(t, "submitted", assessment.Status)
		submitReturnedAt[id] = time.Now()
		if mysqlOutageAfter > 0 && i+1 == mysqlOutageAfter {
			require.Eventually(t, func() bool {
				var published int64
				return db.Table("rm_outbox").Where("event_type=? AND state=?", "evaluation.requested", "published").Count(&published).Error == nil && published == int64(mysqlOutageAfter)
			}, 20*time.Second, 20*time.Millisecond)
			mysqlFaultProxy.SetAvailable(false)
			attemptCtx, attemptCancel := context.WithTimeout(ctx, 3*time.Second)
			_, rejected := service.SubmitForEvaluation(attemptCtx, created[i+1])
			attemptCancel()
			require.Error(t, rejected, "MySQL-unavailable transaction must not be accepted")
			t.Logf("m4_07_fault stream=new_mysql phase=mysql_down accepted_prefix=%d rejected_assessment=%d", mysqlOutageAfter, created[i+1])
			mysqlRecoveryStarted = time.Now()
			mysqlFaultProxy.SetAvailable(true)
			var pending int64
			require.NoError(t, db.Model(&assessmentmysql.AssessmentPO{}).Where("id=? AND status=?", created[i+1], "pending").Count(&pending).Error)
			require.EqualValues(t, 1, pending)
			var intents int64
			require.NoError(t, db.Table("rm_outbox").Where("event_type=?", "evaluation.requested").Count(&intents).Error)
			require.EqualValues(t, mysqlOutageAfter, intents)
		}
		if outageAfter > 0 && i+1 == outageAfter {
			require.Eventually(t, func() bool {
				var published int64
				return db.Table("rm_outbox").Where("event_type=? AND state=?", "evaluation.requested", "published").Count(&published).Error == nil && published == int64(outageAfter)
			}, 20*time.Second, 20*time.Millisecond)
			faultProxy.SetAvailable(false)
			t.Logf("m4_07_fault stream=new_mysql phase=nsq_producer_down committed_prefix=%d", outageAfter)
		}
		if i+1 < batchCount {
			time.Sleep(spacing)
		}
	}
	if outageAfter > 0 {
		time.Sleep(2 * time.Second)
		var total, published int64
		require.NoError(t, db.Table("rm_outbox").Where("event_type=?", "evaluation.requested").Count(&total).Error)
		require.NoError(t, db.Table("rm_outbox").Where("event_type=? AND state=?", "evaluation.requested", "published").Count(&published).Error)
		require.EqualValues(t, batchCount, total)
		require.EqualValues(t, outageAfter, published)
		t.Logf("m4_07_fault stream=new_mysql phase=before_recovery committed=%d published=%d durable_backlog=%d", total, published, total-published)
		recoveryStarted = time.Now()
		faultProxy.SetAvailable(true)
	}
	if lostConfirm {
		var eventID string
		select {
		case eventID = <-publisher.lostAccepted:
		case <-ctx.Done():
			t.Fatal("standard MySQL publisher never accepted the injected message", ctx.Err())
		}
		require.Eventually(t, func() bool {
			var row struct {
				State         string
				FailureCount  uint64
				LastErrorCode string
			}
			err := db.Table("rm_outbox").Select("state, failure_count, last_error_code").Where("message_id=?", eventID).Take(&row).Error
			return err == nil && row.State == "retry_wait" && row.FailureCount == 1 && row.LastErrorCode == "publish_unknown"
		}, 5*time.Second, 20*time.Millisecond)
		t.Logf("m4_07_fault stream=new_mysql phase=sender_unknown_persisted event_id=%s", eventID)
	}
	expectedDeliveries := batchCount
	if lostConfirm {
		expectedDeliveries++
	}
	seenEvents := make(map[string]struct{}, batchCount)
	seenAssessments := make(map[uint64]struct{}, batchCount)
	observedByAssessment := make(map[uint64]m407AssessmentHandled, batchCount)
	for delivery := 0; delivery < expectedDeliveries; delivery++ {
		select {
		case result := <-handled:
			if _, duplicate := seenEvents[result.eventID]; duplicate && !lostConfirm {
				t.Fatalf("standard MySQL event %s handled twice", result.eventID)
			} else if !duplicate {
				seenEvents[result.eventID] = struct{}{}
				seenAssessments[result.assessmentID] = struct{}{}
				observedByAssessment[result.assessmentID] = result
			}
		case <-ctx.Done():
			t.Fatal("standard MySQL event batch did not reach Worker handler", ctx.Err(), len(seenEvents))
		}
	}
	require.Len(t, seenEvents, batchCount)
	require.Len(t, seenAssessments, batchCount)
	seenCalls := make(map[uint64]struct{}, batchCount)
	for range expectedDeliveries {
		var id uint64
		select {
		case id = <-executed:
		case <-ctx.Done():
			t.Fatal("standard MySQL evaluation recorder did not observe all deliveries", ctx.Err())
		}
		if _, duplicate := seenCalls[id]; duplicate && !lostConfirm {
			t.Fatalf("standard MySQL evaluation client called twice for %d", id)
		}
		seenCalls[id] = struct{}{}
	}
	require.Len(t, seenCalls, batchCount)
	deadline := time.Now().Add(10 * time.Second)
	if outageAfter > 0 {
		deadline = recoveryStarted.Add(120 * time.Second)
	} else if mysqlOutageAfter > 0 {
		deadline = mysqlRecoveryStarted.Add(120 * time.Second)
	}
	for {
		var published int64
		require.NoError(t, db.Table("rm_outbox").Where("event_type=? AND state=?", "evaluation.requested", "published").Count(&published).Error)
		if published == int64(batchCount) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("standard MySQL Outbox published %d of %d", published, batchCount)
		}
		time.Sleep(20 * time.Millisecond)
	}
	var submitted int64
	require.NoError(t, db.Model(&assessmentmysql.AssessmentPO{}).Where("status=?", "submitted").Count(&submitted).Error)
	require.EqualValues(t, batchCount, submitted)
	if mysqlOutageAfter > 0 {
		for _, id := range created {
			result := observedByAssessment[id]
			if confirms := publisher.times(result.eventID); len(confirms) > 1 {
				t.Fatalf("MySQL outage caused %d broker confirmations for stable event_id=%s assessment_id=%d; recording execution client cannot prove one business effect", len(confirms), result.eventID, id)
			}
		}
	}
	stats := waitM407NSQDrain(t, ctx, topic, channel, expectedDeliveries)
	if outageAfter > 0 {
		require.LessOrEqual(t, time.Since(recoveryStarted), 120*time.Second)
		t.Logf("m4_07_fault stream=new_mysql phase=drained committed=%d published=%d recovery_ms=%d", batchCount, batchCount, time.Since(recoveryStarted).Milliseconds())
	} else if mysqlOutageAfter > 0 {
		require.LessOrEqual(t, time.Since(mysqlRecoveryStarted), 120*time.Second)
		t.Logf("m4_07_fault stream=new_mysql phase=mysql_recovered committed=%d published=%d recovery_ms=%d", batchCount, batchCount, time.Since(mysqlRecoveryStarted).Milliseconds())
	}
	for _, id := range created {
		result, ok := observedByAssessment[id]
		require.True(t, ok)
		var confirmedAt time.Time
		require.NoError(t, db.Table("rm_outbox").Select("transport_confirmed_at").Where("message_id=?", result.eventID).Scan(&confirmedAt).Error)
		require.False(t, confirmedAt.IsZero())
		commits, confirms := commitObserver.times(id), publisher.times(result.eventID)
		require.Len(t, commits, 1)
		confirmationsForEvent := 1
		if lostConfirm {
			confirmationsForEvent = 2
		}
		require.Len(t, confirms, confirmationsForEvent)
		require.False(t, confirms[0].Before(commits[0]))
		t.Logf("m4_07_timeline stream=new_mysql sheet_id=%d assessment_id=%d event_id=%s tx_commit_observed_at=%s broker_confirm_observed_at=%s commit_to_broker_confirm_ns=%d submit_returned_at=%s nsqd_message_at=%s outbox_transport_confirmed_at=%s handler_done_at=%s", result.sheetID, id, result.eventID,
			m407FormatTime(commits[0]), m407FormatTime(confirms[0]), confirms[0].Sub(commits[0]).Nanoseconds(), m407FormatTime(submitReturnedAt[id]), m407FormatTime(result.brokerAt), m407FormatTime(confirmedAt), m407FormatTime(result.handlerDoneAt))
	}
	var submitRate float64
	if batchCount > 1 {
		submitRate = float64(batchCount-1) / submitReturnedAt[created[batchCount-1]].Sub(submitReturnedAt[created[0]]).Seconds()
	}
	t.Logf("standard MySQL business-key batch diagnostic: count=%d spacing=%s submit_return_rate_per_sec=%.3f manifest_sha256=%x unique_event_ids=%d nsq_delivery=%d nsq_fin_samples=%d nsq_e2e_percentiles_ns=%v nsq_requeue=%d nsq_timeout=%d execution_stub_calls=%d", batchCount, spacing, submitRate, manifest.Sum(nil), len(seenEvents), stats.MessageCount, stats.E2EProcessingLatency.Count, stats.E2EProcessingLatency.Percentiles, stats.RequeueCount, stats.TimeoutCount, expectedDeliveries)
}

type m407ObservedSDKPublisher struct {
	transport.Publisher
	mu                sync.Mutex
	confirmed         map[string][]time.Time
	injectLostConfirm bool
	lostOnce          sync.Once
	lostAccepted      chan string
}

func (p *m407ObservedSDKPublisher) Publish(ctx context.Context, item message.Message) transport.Result {
	result := p.Publisher.Publish(ctx, item)
	if result.Outcome == transport.Confirmed {
		p.mu.Lock()
		id := item.Input().ID
		p.confirmed[id] = append(p.confirmed[id], time.Now())
		p.mu.Unlock()
		if p.injectLostConfirm {
			injected := false
			p.lostOnce.Do(func() {
				injected = true
				p.lostAccepted <- id
			})
			if injected {
				return transport.Result{Outcome: transport.Unknown}
			}
		}
	}
	return result
}

func (p *m407ObservedSDKPublisher) times(id string) []time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]time.Time(nil), p.confirmed[id]...)
}

type m407AssessmentHandled struct {
	eventID       string
	assessmentID  uint64
	sheetID       uint64
	brokerAt      time.Time
	handlerDoneAt time.Time
}

type m407EvaluationRecorder struct {
	expected map[uint64]uint64
	calls    chan uint64
}

func (r m407EvaluationRecorder) ExecuteEvaluation(_ context.Context, id uint64) (*pb.ExecuteEvaluationResponse, error) {
	if _, ok := r.expected[id]; !ok {
		return nil, fmt.Errorf("unexpected assessment %d", id)
	}
	select {
	case r.calls <- id:
	default:
		return nil, fmt.Errorf("evaluation recorder is full")
	}
	return &pb.ExecuteEvaluationResponse{Status: "already_evaluated"}, nil
}
