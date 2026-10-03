//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m5

package transaction

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	appexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	evalworker "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/worker"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	workereventing "github.com/FangcunMount/qs-server/internal/worker/integration/eventing"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	legacywire "github.com/FangcunMount/reliable-messaging/wire/legacy"
	mysqldriver "github.com/go-sql-driver/mysql"
	driver "github.com/nsqio/go-nsq"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Sustained fault affects real Confirm COM_QUERY packets only. Intake and
// successful Outcome transactions retain their original direct DB connection.
// No Store/Publisher method is mocked, and no lease or SQL row is repaired.
type m4PressureGateway struct {
	l      net.Listener
	active atomic.Bool
	mu     sync.Mutex
	pairs  map[net.Conn]net.Conn
	drops  []map[string]any
}

func newM4PressureGateway(t *testing.T) *m4PressureGateway {
	l, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err)
	p := &m4PressureGateway{l: l, pairs: make(map[net.Conn]net.Conn)}
	go func() {
		for {
			down, err := l.Accept()
			if err != nil {
				return
			}
			up, err := net.DialTimeout("tcp", "mysql:3306", time.Second)
			if err != nil {
				down.Close()
				continue
			}
			p.mu.Lock()
			p.pairs[down] = up
			p.mu.Unlock()
			go func() {
				defer func() { down.Close(); up.Close(); p.mu.Lock(); delete(p.pairs, down); p.mu.Unlock() }()
				go func() { _, _ = io.Copy(down, up); down.Close(); up.Close() }()
				for {
					packet, err := m4ReadPacket(down)
					if err != nil {
						return
					}
					matched := len(packet) > 5 && packet[4] == 3 && strings.Contains(string(packet[5:]), "UPDATE rm_outbox") && strings.Contains(string(packet[5:]), "state='published'")
					if matched {
						p.mu.Lock()
						if p.active.Load() {
							p.drops = append(p.drops, map[string]any{"at": time.Now(), "actual_sql": string(packet[5:])})
							p.mu.Unlock()
							return
						}
						p.mu.Unlock()
					}
					if _, err = up.Write(packet); err != nil {
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() {
		l.Close()
		p.mu.Lock()
		defer p.mu.Unlock()
		for d, u := range p.pairs {
			d.Close()
			u.Close()
		}
	})
	return p
}

type m4PressurePublisher struct {
	rmtransport.Publisher
	mu       sync.Mutex
	accepted map[string][][]byte
}

func (p *m4PressurePublisher) Publish(ctx context.Context, m message.Message) rmtransport.Result {
	r := p.Publisher.Publish(ctx, m)
	if r.Outcome == rmtransport.Confirmed {
		p.mu.Lock()
		p.accepted[m.Input().ID] = append(p.accepted[m.Input().ID], m.Input().Payload)
		p.mu.Unlock()
	}
	return r
}

func TestM4ConfirmSustainedSQLAndSuccessfulOutcomes(t *testing.T) {
	const total = 2600
	const spacing = 120 * time.Millisecond
	const prefix = 1300
	ctx, cancel := context.WithTimeout(t.Context(), 9*time.Minute)
	defer cancel()
	dsn := os.Getenv("RM_M4_CONFIRM_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m4_qs_confirm" || os.Getenv("RM_QS_M5_NSQ_TCP") != "nsqd:4150" {
		t.Fatal("disposable endpoints required")
	}
	base, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer base.Close()
	_, err = base.ExecContext(ctx, "CREATE DATABASE m4_qs_confirm_pressure")
	require.NoError(t, err)
	direct := *parsed
	direct.DBName = "m4_qs_confirm_pressure"
	db, err := gorm.Open(gormmysql.Open(direct.FormatDSN()), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	// RuntimeCheckpointPO's AutoMigrate index tags are not the production
	// migration indexes. Use the original schema rather than tune locking,
	// transaction isolation or consumer concurrency for the pressure fixture.
	for _, migration := range []struct{ name, statement string }{
		{"000040_merge_runtime_checkpoint.up.sql", "CREATE TABLE IF NOT EXISTS `runtime_checkpoint`"},
		{"000046_add_evaluation_run_claim_lease.up.sql", "ALTER TABLE `runtime_checkpoint`"},
		{"000049_add_retry_governance.up.sql", "ALTER TABLE `runtime_checkpoint`"},
	} {
		source, e := os.ReadFile("../../../../pkg/migration/migrations/mysql/" + migration.name)
		require.NoError(t, e)
		start := strings.Index(string(source), migration.statement)
		require.GreaterOrEqual(t, start, 0)
		statement := string(source)[start:]
		end := strings.Index(statement, ";")
		require.GreaterOrEqual(t, end, 0)
		require.NoError(t, db.Exec(statement[:end+1]).Error)
	}
	var indexes []struct {
		Name    string `gorm:"column:name"`
		Columns string `gorm:"column:columns"`
	}
	require.NoError(t, db.Raw("SELECT INDEX_NAME AS name, GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) AS columns FROM information_schema.statistics WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='runtime_checkpoint' GROUP BY INDEX_NAME ORDER BY INDEX_NAME").Scan(&indexes).Error)
	indexColumns := make(map[string]string)
	for _, index := range indexes {
		indexColumns[index.Name] = index.Columns
	}
	require.Equal(t, "scope,status", indexColumns["idx_runtime_checkpoint_scope_status"])
	require.Equal(t, "scope,status,lease_expires_at", indexColumns["idx_runtime_checkpoint_claim"])
	require.Equal(t, "scope,status,retry_disposition,next_attempt_at", indexColumns["idx_runtime_checkpoint_retry_due"])
	require.NoError(t, db.AutoMigrate(&assessmentmysql.AssessmentPO{}, &assessmentmysql.EvaluationOutcomePO{}, &assessmentmysql.AssessmentScorePO{}))
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)
	require.NoError(t, createEvaluationRequestRefTable(ctx, sqlDB))
	config, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  evaluation:
    name: qs.evaluation.lifecycle
events:
  evaluation.requested:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Evaluation
    domain: evaluation
    handler: evaluation_requested_handler
  evaluation.outcome.committed:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Evaluation
    domain: evaluation
    handler: evaluation_outcome_committed_handler
`))
	require.NoError(t, err)
	catalog := eventcatalog.NewCatalog(config)
	stager, err := mysqlstandard.NewStager(catalog, eventruntime.SourceAPIServer)
	require.NoError(t, err)
	assessmentRepo := assessmentmysql.NewAssessmentRepository(db)
	runRepo := checkpoint.NewRunRepository(db)
	runner := NewMySQLRunner(db)
	intake := appintake.NewService(assessmentRepo, proofModelValidator{}, runner, stager)
	input := &m4ConfirmInput{}
	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	require.NoError(t, err)
	require.NoError(t, evalruntime.AttachNativePipelines(registry, evalruntime.NativePipelineDeps{ScaleScorer: evalruntime.MaterializeFactorScoringPipelineComponents(evalruntime.WiringDeps{ScaleScorer: ruleengine.NewScaleFactorScorer()})}))
	outcomes := assessmentmysql.NewOutcomeRepository(db)
	committer := outcomecommit.NewCommitter(runner, assessmentRepo, outcomes, runRepo, outcomescoring.NewAssessmentScoreProjector(assessmentmysql.NewScoreRepository(db)), stager, nil)
	engine := appexecute.NewEngine(assessmentRepo, input, appexecute.WithRunRepository(runRepo), appexecute.WithTransactionalOutbox(runner, stager), appexecute.WithRuntimeDescriptorRegistry(registry), appexecute.WithEvaluationCommitter(committer))
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	grpcservice.NewEvaluationWorkerService(evalworker.NewService(engine, assessmentRepo, outcomes, runRepo)).RegisterService(server)
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(l) }()
	defer func() { server.Stop(); require.NoError(t, <-serverDone) }()
	conn, err := grpc.NewClient(l.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dispatcher := workereventing.NewDispatcher(logger, &workereventing.HandlerDependencies{Logger: logger, EvaluationWorkerClient: m5EvaluationGRPCClient{client: evalpb.NewEvaluationWorkerServiceClient(conn)}}, handlers.NewRegistry())
	require.NoError(t, dispatcher.Initialize(catalog))
	const topic, channel = "qs.evaluation.lifecycle", "m4-confirm-pressure"
	httpClient := &http.Client{Timeout: 2 * time.Second}
	for _, endpoint := range []string{"http://nsqd:4151/topic/create?topic=" + topic, "http://nsqd:4151/channel/create?topic=" + topic + "&channel=" + channel} {
		r, err := httpClient.Post(endpoint, "application/octet-stream", nil)
		require.NoError(t, err)
		r.Body.Close()
		require.Equal(t, 200, r.StatusCode)
	}
	require.Eventually(t, func() bool {
		r, e := httpClient.Get("http://nsqlookupd:4161/lookup?topic=" + topic)
		if e != nil {
			return false
		}
		defer r.Body.Close()
		var v struct {
			Producers []json.RawMessage `json:"producers"`
		}
		return r.StatusCode == 200 && json.NewDecoder(r.Body).Decode(&v) == nil && len(v.Producers) == 1
	}, 10*time.Second, 50*time.Millisecond)
	var deliveryMu sync.Mutex
	deliveries := make(map[string][]m4ConfirmDelivery)
	subscriber, err := eventtransport.NewSDKDeliverySubscriber(eventtransport.SubscriberConfig{Provider: "nsq", NSQLookupdAddr: "nsqlookupd:4161", NSQMessageTimeout: 12 * time.Second}, 16, 8, func(context.Context, legacywire.FailedHandoff) error { return fmt.Errorf("unexpected failure handoff") })
	require.NoError(t, err)
	defer func() { require.NoError(t, subscriber.Close()) }()
	require.NoError(t, subscriber.Subscribe(topic, channel, func(ctx context.Context, d rmtransport.Delivery) error {
		m := d.Message()
		if m.Metadata["event_type"] == eventcatalog.EvaluationRequested {
			if _, err := dispatcher.DispatchEvent(ctx, m.Metadata["event_type"], m.Payload); err != nil {
				return err
			}
		} else if m.Metadata["event_type"] != eventcatalog.EvaluationOutcomeCommitted {
			return fmt.Errorf("unexpected event")
		}
		if err := d.Ack(); err != nil {
			return err
		}
		deliveryMu.Lock()
		deliveries[m.ID] = append(deliveries[m.ID], m4ConfirmDelivery{ID: m.ID, PhysicalID: m.TransportID, EventType: m.Metadata["event_type"], Payload: append([]byte(nil), m.Payload...)})
		deliveryMu.Unlock()
		return nil
	}))
	producer, err := driver.NewProducer("nsqd:4150", driver.NewConfig())
	require.NoError(t, err)
	producer.SetLogger(nil, driver.LogLevelError)
	native, err := sdknsq.New(producer, map[string]string{topic: topic}, 16)
	require.NoError(t, err)
	defer func() {
		drain, c := context.WithTimeout(context.Background(), 15*time.Second)
		defer c()
		require.NoError(t, native.Drain(drain))
		producer.Stop()
	}()
	publisher := &m4PressurePublisher{Publisher: native, accepted: make(map[string][][]byte)}
	gateway := newM4PressureGateway(t)
	relayDSN := direct
	relayDSN.Addr = gateway.l.Addr().String()
	relayDSN.InterpolateParams = true
	relaySQL, err := sql.Open("mysql", relayDSN.FormatDSN())
	require.NoError(t, err)
	defer relaySQL.Close()
	store, err := sdkmysql.New(relaySQL)
	require.NoError(t, err)
	var confirmErrors atomic.Int64
	r, err := relay.New(store, publisher, relay.Config{Concurrency: 16, PollInterval: time.Second, Lease: 30 * time.Second, PublishTimeout: 10 * time.Second, WriteTimeout: 5 * time.Second, Retry: standardoutbox.SDKRetryPolicy(), Observe: func(e relay.Event) {
		if e.Kind == "write_failed" && e.Outcome == rmtransport.Confirmed {
			confirmErrors.Add(1)
		}
	}})
	require.NoError(t, err)
	relayCtx, stopRelay := context.WithCancel(ctx)
	relayDone := make(chan error, 1)
	go func() { relayDone <- r.Run(relayCtx) }()
	var stopOnce sync.Once
	finishRelay := func() { stopOnce.Do(func() { stopRelay(); require.NoError(t, <-relayDone) }) }
	defer finishRelay()
	kind, code, version, algorithm := "scale", "MODEL-1", "1.0.0", string(modelcatalog.AlgorithmScaleDefault)
	ids := make([]uint64, total)
	submitted := make([]time.Time, total)
	manifest := sha256.New()
	for i := range total {
		sheet := uint64(90010003 + i)
		a, e := intake.CreateForAnswerSheet(ctx, appintake.CreateCommand{OrgID: 501, TesteeID: 401, AnswerSheetID: sheet, QuestionnaireCode: "QNR-M4", QuestionnaireVersion: "1.0.0", OriginType: "adhoc", ModelKind: &kind, ModelCode: &code, ModelVersion: &version, ModelAlgorithm: &algorithm})
		require.NoError(t, e)
		ids[i] = a.ID
		_, e = fmt.Fprintf(manifest, "%d|501|401|QNR-M4|1.0.0|adhoc|scale|MODEL-1|1.0.0\n", sheet)
		require.NoError(t, e)
	}
	var faultAt time.Time
	for i, id := range ids {
		_, err = intake.SubmitForEvaluation(ctx, id)
		require.NoError(t, err)
		submitted[i] = time.Now()
		if i+1 == prefix {
			require.Eventually(t, func() bool {
				var n, outcomes int64
				return db.Table("rm_outbox").Where("state<>'published'").Count(&n).Error == nil && n == 0 && db.Table("evaluation_outcome").Count(&outcomes).Error == nil && outcomes == prefix
			}, 20*time.Second, 20*time.Millisecond)
			gateway.mu.Lock()
			faultAt = time.Now()
			gateway.active.Store(true)
			gateway.mu.Unlock()
			t.Logf("actual Confirm fault starts=%s original_prefix=%d", faultAt.Format(time.RFC3339Nano), prefix)
		}
		if i+1 < total {
			time.Sleep(spacing)
		}
	}
	time.Sleep(2 * time.Second)
	var unfinished int64
	require.NoError(t, db.Table("rm_outbox").Where("state<>'published'").Count(&unfinished).Error)
	require.Positive(t, unfinished)
	gateway.mu.Lock()
	gateway.active.Store(false)
	restoredAt := time.Now()
	gateway.mu.Unlock()
	require.GreaterOrEqual(t, restoredAt.Sub(faultAt), 156*time.Second)
	// All 2600 original submissions already exist at restoration. Recovery is
	// measured from this fixed point, not reset after collection or assertions.
	recovered := false
	deadline := restoredAt.Add(120 * time.Second)
	for time.Now().Before(deadline) {
		var n, pending int64
		require.NoError(t, db.Table("rm_outbox").Where("state<>'published'").Count(&pending).Error)
		require.NoError(t, db.Table("evaluation_outcome").Count(&n).Error)
		deliveryMu.Lock()
		unique := len(deliveries)
		deliveryMu.Unlock()
		if pending == 0 && n == total && unique == 2*total {
			recovered = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.True(t, recovered, "original responsibilities not recovered within120s")
	waitM5ProcessNSQDrain(t, topic, channel)
	recovery := time.Since(restoredAt)
	require.LessOrEqual(t, recovery, 120*time.Second)
	finishRelay()
	span := submitted[total-1].Sub(submitted[0])
	rate := float64(total-1) / span.Seconds()
	require.GreaterOrEqual(t, span.Seconds(), 300.0)
	require.GreaterOrEqual(t, rate, 7.0)
	require.EqualValues(t, total, input.calls.Load())
	for table, want := range map[string]int64{"runtime_checkpoint": total, "evaluation_outcome": total, "rm_outbox": 2 * total, "assessment": total} {
		var n int64
		require.NoError(t, db.Table(table).Count(&n).Error)
		require.Equal(t, want, n, table)
	}
	var correct int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM assessment a JOIN runtime_checkpoint r ON r.assessment_id=a.id JOIN evaluation_outcome o ON o.assessment_id=a.id WHERE a.status='evaluated' AND a.total_score=7 AND r.attempt_no=1 AND r.status='succeeded' AND r.input_snapshot_ref LIKE 'isn:v2:%'").Scan(&correct).Error)
	require.EqualValues(t, total, correct)
	var forbidden int64
	require.NoError(t, db.Table("rm_outbox").Where("event_type NOT IN ?", []string{eventcatalog.EvaluationRequested, eventcatalog.EvaluationOutcomeCommitted}).Count(&forbidden).Error)
	require.Zero(t, forbidden)
	var rows []struct {
		MessageID, EventType string
		Payload              []byte
	}
	require.NoError(t, db.Table("rm_outbox").Select("message_id,event_type,payload").Find(&rows).Error)
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	deliveryMu.Lock()
	defer deliveryMu.Unlock()
	physical, duplicates := 0, 0
	for _, row := range rows {
		pubs, ds := publisher.accepted[row.MessageID], deliveries[row.MessageID]
		require.NotEmpty(t, pubs)
		require.Len(t, ds, len(pubs))
		physical += len(pubs)
		if len(pubs) > 1 {
			duplicates++
		}
		decoded, recognized, e := legacywire.Decode(row.Payload)
		require.NoError(t, e)
		require.True(t, recognized)
		require.Equal(t, row.MessageID, decoded.UUID)
		physicalIDs := make(map[string]bool)
		for i, payload := range pubs {
			require.Equal(t, row.Payload, payload)
			require.Equal(t, row.MessageID, ds[i].ID)
			require.Equal(t, row.EventType, ds[i].EventType)
			require.Equal(t, decoded.Payload, ds[i].Payload)
			require.False(t, physicalIDs[ds[i].PhysicalID])
			physicalIDs[ds[i].PhysicalID] = true
		}
	}
	require.Positive(t, duplicates)
	require.Positive(t, confirmErrors.Load())
	response, err := httpClient.Get("http://nsqd:4151/stats?format=json")
	require.NoError(t, err)
	var broker map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&broker))
	response.Body.Close()
	var finalChannel map[string]any
	for _, rawTopic := range broker["topics"].([]any) {
		topicStats := rawTopic.(map[string]any)
		if topicStats["topic_name"] != topic {
			continue
		}
		for _, rawChannel := range topicStats["channels"].([]any) {
			ch := rawChannel.(map[string]any)
			if ch["channel_name"] == channel {
				finalChannel = ch
			}
		}
	}
	require.NotNil(t, finalChannel)
	require.EqualValues(t, physical, finalChannel["message_count"])
	for _, name := range []string{"depth", "in_flight_count", "deferred_count", "requeue_count", "timeout_count"} {
		require.EqualValues(t, 0, finalChannel[name], name)
	}
	latency := finalChannel["e2e_processing_latency"].(map[string]any)
	require.EqualValues(t, physical, latency["count"])
	require.Len(t, latency["percentiles"], 3)
	recovery = time.Since(restoredAt)
	require.LessOrEqual(t, recovery, 120*time.Second)
	gateway.mu.Lock()
	drops := append([]map[string]any(nil), gateway.drops...)
	gateway.mu.Unlock()
	require.Greater(t, len(drops), 100)
	firstDrop := drops[0]["at"].(time.Time)
	lastDrop := drops[len(drops)-1]["at"].(time.Time)
	require.GreaterOrEqual(t, lastDrop.Sub(firstDrop).Seconds(), 150.0)
	result := map[string]any{"runtime": "308a0d966529ffab80f3b600f464a90c8d2a3bf1", "count": total, "spacing_ms": 120, "input_span_seconds": span.Seconds(), "input_rate": rate, "manifest_sha256": fmt.Sprintf("%x", manifest.Sum(nil)), "fault_at": faultAt, "restored_at": restoredAt, "fault_duration_seconds": restoredAt.Sub(faultAt).Seconds(), "recovery_ms": recovery.Milliseconds(), "unfinished_at_restore": unfinished, "actual_drops": drops, "confirm_write_errors": confirmErrors.Load(), "runs": total, "outcomes": total, "outcome_intents": total, "original_request_intents": total, "resolver_calls": input.calls.Load(), "physical_deliveries": physical, "duplicated_event_ids": duplicates, "original_ids": ids, "intent_rows": rows, "deliveries": deliveries, "scope": "sustained real Confirm COM_QUERY loss; native successful scoring; no entire-DB outage, external model/report or production SLA"}
	result["final_channel"] = finalChannel
	encoded, err := json.MarshalIndent(result, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(os.Getenv("RM_M4_CONFIRM_RESULT"), encoded, 0600))
	t.Logf("sustained actual Confirm success:2600 original requests; Run/Outcome1 each; physical=%d duplicate_events=%d recovery=%s input_span=%s rate=%.3f", physical, duplicates, recovery, span, rate)
}
