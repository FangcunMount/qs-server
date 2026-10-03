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
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	workereventing "github.com/FangcunMount/qs-server/internal/worker/integration/eventing"
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

// This gateway only accepts the invocation's disposable MySQL endpoint. It
// reads actual protocol packets: either drops the first Confirm COM_QUERY
// before forwarding, or forwards it and drops the server's actual OK packet.
// interpolateParams is enabled only on the test Relay connection so this
// observer can identify the command; the SDK Store and SQL are unchanged.
type m4ConfirmGateway struct {
	listener net.Listener
	mode     string
	consumed atomic.Bool
	packet   chan string
	mu       sync.Mutex
	pairs    map[net.Conn]net.Conn
}

func newM4ConfirmGateway(t *testing.T, mode string) *m4ConfirmGateway {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := &m4ConfirmGateway{listener: l, mode: mode, packet: make(chan string, 1), pairs: make(map[net.Conn]net.Conn)}
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
			go p.forward(down, up)
		}
	}()
	t.Cleanup(func() {
		l.Close()
		p.mu.Lock()
		defer p.mu.Unlock()
		for down, up := range p.pairs {
			down.Close()
			up.Close()
		}
	})
	return p
}
func m4ReadPacket(c net.Conn) ([]byte, error) {
	h := make([]byte, 4)
	if _, err := io.ReadFull(c, h); err != nil {
		return nil, err
	}
	n := int(h[0]) | int(h[1])<<8 | int(h[2])<<16
	if n > 16*1024*1024-1 {
		return nil, fmt.Errorf("packet bound")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(c, b); err != nil {
		return nil, err
	}
	return append(h, b...), nil
}
func (p *m4ConfirmGateway) forward(down, up net.Conn) {
	defer func() { down.Close(); up.Close(); p.mu.Lock(); delete(p.pairs, down); p.mu.Unlock() }()
	var dropReply atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer down.Close()
		defer up.Close()
		for {
			packet, err := m4ReadPacket(up)
			if err != nil {
				return
			}
			if dropReply.Swap(false) {
				if len(packet) > 4 && packet[4] == 0 {
					p.packet <- "server_OK_dropped_after_real_commit"
				} else {
					p.packet <- "unexpected_server_packet"
				}
				return
			}
			if _, err = down.Write(packet); err != nil {
				return
			}
		}
	}()
	for {
		packet, err := m4ReadPacket(down)
		if err != nil {
			break
		}
		matched := len(packet) > 5 && packet[4] == 3 && strings.Contains(string(packet[5:]), "UPDATE rm_outbox") && strings.Contains(string(packet[5:]), "state='published'")
		if matched && p.consumed.CompareAndSwap(false, true) {
			if p.mode == "before" {
				p.packet <- "actual_Confirm_COM_QUERY_dropped_before_forward"
				break
			}
			dropReply.Store(true)
		}
		if _, err = up.Write(packet); err != nil {
			break
		}
	}
	down.Close()
	up.Close()
	<-done
}

type m4ConfirmInput struct{ calls atomic.Int32 }

func (r *m4ConfirmInput) Resolve(ctx context.Context, ref evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	r.calls.Add(1)
	scale := &scalesnapshot.ScaleSnapshot{Code: ref.ModelRef.Code, ScaleVersion: ref.ModelRef.Version, QuestionnaireCode: ref.QuestionnaireCode, QuestionnaireVersion: ref.QuestionnaireVersion, Status: "published", Factors: []scalesnapshot.FactorSnapshot{{Code: "total", Title: "总分", IsTotalScore: true, QuestionCodes: []string{"q1", "q2"}, ScoringStrategy: "sum", InterpretRules: []scalesnapshot.InterpretRuleSnapshot{{Min: 0, Max: 10, RiskLevel: "low", Conclusion: "low"}}}}}
	return &evaluationinput.InputSnapshot{
		Model:         &evaluationinput.ModelSnapshot{Kind: evaluationinput.EvaluationModelKindScale, Algorithm: string(modelcatalog.AlgorithmScaleDefault), DecisionKind: string(modelcatalog.DecisionKindScoreRange), Code: ref.ModelRef.Code, Version: ref.ModelRef.Version, Title: "isolated native scale"},
		ModelPayload:  evaluationinput.ScaleModelPayload{Scale: scale},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: ref.QuestionnaireCode, Version: ref.QuestionnaireVersion},
		AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: ref.AnswerSheetID, QuestionnaireCode: ref.QuestionnaireCode, QuestionnaireVersion: ref.QuestionnaireVersion, Answers: []evaluationinput.AnswerSnapshot{{QuestionCode: "q1", Score: 3}, {QuestionCode: "q2", Score: 4}}},
		DefinitionV2:  &definition.Definition{Measure: definition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Title: "总分", Role: factor.FactorRoleTotal}}, FactorGraph: factor.FactorGraph{Roots: []string{"total"}}, Scoring: []factor.Scoring{{FactorCode: "total", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q1", ScoringMode: factor.QuestionScoringModeQuestionScore}, {Kind: factor.ScoringSourceQuestion, Code: "q2", ScoringMode: factor.QuestionScoringModeQuestionScore}}}}}, Conclusions: []conclusion.Conclusion{conclusion.RiskConclusion{FactorCode: "total", Rules: []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, MaxInclusive: true, OutcomeCode: "low", Level: "low"}}}}, Outcomes: []conclusion.Outcome{{Code: "low", Title: "低风险"}}, InterpretationAssets: interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "low", Title: "低风险"}}}},
	}, nil
}

type m4ConfirmDelivery struct {
	ID, PhysicalID, EventType string
	Payload                   []byte
}

// Both SQL uncertainty windows run the actual SDK Relay/Store, native SDK
// subscriber, original Worker dispatcher, real gRPC and original durable
// Assessment/Run/Outcome success transaction. No external model is invoked. This is
// a successful native factor-scoring idempotency proof, not an external
// model/report or sustained-pressure proof.
func TestM4ConfirmWritebackActualSQLAndSuccessfulOutcome(t *testing.T) {
	dsn := os.Getenv("RM_M4_CONFIRM_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m4_qs_confirm" || os.Getenv("RM_QS_M5_NSQ_TCP") != "nsqd:4150" {
		t.Fatal("disposable original SQL/NSQ endpoints required")
	}
	all := make(map[string]any)
	for _, mode := range []string{"before", "after"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			base, err := sql.Open("mysql", dsn)
			require.NoError(t, err)
			defer base.Close()
			name := "m4_qs_confirm_" + mode
			_, err = base.ExecContext(ctx, "CREATE DATABASE "+name)
			require.NoError(t, err)
			direct := *parsed
			direct.DBName = name
			db, err := gorm.Open(gormmysql.Open(direct.FormatDSN()), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			defer sqlDB.Close()
			require.NoError(t, db.AutoMigrate(&assessmentmysql.AssessmentPO{}, &checkpoint.RuntimeCheckpointPO{}, &assessmentmysql.EvaluationOutcomePO{}, &assessmentmysql.AssessmentScorePO{}))
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
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			server := grpc.NewServer()
			grpcservice.NewEvaluationWorkerService(evalworker.NewService(engine, assessmentRepo, assessmentmysql.NewOutcomeRepository(db), runRepo)).RegisterService(server)
			serverDone := make(chan error, 1)
			go func() { serverDone <- server.Serve(listener) }()
			defer func() { server.Stop(); require.NoError(t, <-serverDone) }()
			conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
			require.NoError(t, err)
			defer conn.Close()
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			dispatcher := workereventing.NewDispatcher(logger, &workereventing.HandlerDependencies{Logger: logger, EvaluationWorkerClient: m5EvaluationGRPCClient{client: evalpb.NewEvaluationWorkerServiceClient(conn)}}, handlers.NewRegistry())
			require.NoError(t, dispatcher.Initialize(catalog))
			received := make(chan m4ConfirmDelivery, 128)
			subscriber, err := eventtransport.NewSDKDeliverySubscriber(eventtransport.SubscriberConfig{Provider: "nsq", NSQLookupdAddr: "nsqlookupd:4161", NSQMessageTimeout: 12 * time.Second}, 1, 8, func(context.Context, legacywire.FailedHandoff) error { return fmt.Errorf("unexpected failure handoff") })
			require.NoError(t, err)
			defer func() { require.NoError(t, subscriber.Close()) }()
			const topic = "qs.evaluation.lifecycle"
			channel := "m4-confirm-" + mode
			require.NoError(t, subscriber.Subscribe(topic, channel, func(ctx context.Context, delivery rmtransport.Delivery) error {
				msg := delivery.Message()
				if msg.Metadata["event_type"] == eventcatalog.EvaluationRequested {
					_, err := dispatcher.DispatchEvent(ctx, msg.Metadata["event_type"], msg.Payload)
					if err != nil {
						return err
					}
				} else if msg.Metadata["event_type"] != eventcatalog.EvaluationOutcomeCommitted {
					return fmt.Errorf("unexpected event type: %s", msg.Metadata["event_type"])
				}
				received <- m4ConfirmDelivery{ID: msg.ID, PhysicalID: msg.TransportID, EventType: msg.Metadata["event_type"], Payload: append([]byte(nil), msg.Payload...)}
				return delivery.Ack()
			}))
			producer, err := driver.NewProducer("nsqd:4150", driver.NewConfig())
			require.NoError(t, err)
			producer.SetLogger(nil, driver.LogLevelError)
			publisher, err := sdknsq.New(producer, map[string]string{topic: topic}, 1)
			require.NoError(t, err)
			defer func() {
				drain, finish := context.WithTimeout(context.Background(), 5*time.Second)
				defer finish()
				require.NoError(t, publisher.Drain(drain))
				producer.Stop()
			}()
			gateway := newM4ConfirmGateway(t, mode)
			relayDSN := direct
			relayDSN.Addr = gateway.listener.Addr().String()
			relayDSN.InterpolateParams = true
			relaySQL, err := sql.Open("mysql", relayDSN.FormatDSN())
			require.NoError(t, err)
			defer relaySQL.Close()
			require.NoError(t, relaySQL.PingContext(ctx))
			store, err := sdkmysql.New(relaySQL)
			require.NoError(t, err)
			writes := make(chan relay.Event, 128)
			r, err := relay.New(store, publisher, relay.Config{Concurrency: 1, PollInterval: 30 * time.Millisecond, Lease: 30 * time.Second, PublishTimeout: 2 * time.Second, WriteTimeout: time.Second, Retry: standardoutbox.SDKRetryPolicy(), Observe: func(e relay.Event) {
				if e.Kind == "write_failed" || e.Kind == "write_succeeded" {
					writes <- e
				}
			}})
			require.NoError(t, err)
			kind, code, version := "scale", "MODEL-1", "1.0.0"
			created, err := intake.CreateForAnswerSheet(ctx, appintake.CreateCommand{OrgID: 1, TesteeID: 2, AnswerSheetID: 1705, QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", OriginType: "adhoc", ModelKind: &kind, ModelCode: &code, ModelVersion: &version})
			require.NoError(t, err)
			_, err = intake.SubmitForEvaluation(ctx, created.ID)
			require.NoError(t, err)
			var original struct {
				MessageID, State string
				Payload          []byte
			}
			require.NoError(t, db.Table("rm_outbox").Select("message_id,state,payload").Where("event_type=?", eventcatalog.EvaluationRequested).Take(&original).Error)
			originalWire := append([]byte(nil), original.Payload...)
			relayCtx, stopRelay := context.WithCancel(ctx)
			relayDone := make(chan error, 1)
			go func() { relayDone <- r.Run(relayCtx) }()
			defer func() { stopRelay(); require.NoError(t, <-relayDone) }()
			select {
			case observed := <-gateway.packet:
				require.Equal(t, map[string]string{"before": "actual_Confirm_COM_QUERY_dropped_before_forward", "after": "server_OK_dropped_after_real_commit"}[mode], observed)
			case <-ctx.Done():
				t.Fatal("no actual SQL Confirm interception", ctx.Err())
			}
			select {
			case write := <-writes:
				require.Equal(t, "write_failed", write.Kind)
				require.Equal(t, rmtransport.Confirmed, write.Outcome)
				require.Error(t, write.Err)
			case <-ctx.Done():
				t.Fatal("no original Relay write failure")
			}
			expected := 1
			if mode == "before" {
				expected = 2
			}
			deliveries := make([]m4ConfirmDelivery, 0, expected)
			for len(deliveries) < expected {
				select {
				case record := <-received:
					if record.EventType == eventcatalog.EvaluationRequested {
						deliveries = append(deliveries, record)
					}
				case <-ctx.Done():
					t.Fatal("original request not recovered", ctx.Err())
				}
			}
			require.Eventually(t, func() bool {
				var n int64
				return db.Table("rm_outbox").Where("state != ?", "published").Count(&n).Error == nil && n == 0
			}, 8*time.Second, 30*time.Millisecond)
			for _, delivery := range deliveries {
				require.Equal(t, original.MessageID, delivery.ID)
			}
			if mode == "before" {
				require.NotEqual(t, deliveries[0].PhysicalID, deliveries[1].PhysicalID)
				require.Equal(t, deliveries[0].Payload, deliveries[1].Payload)
			}
			require.EqualValues(t, 1, input.calls.Load(), "duplicate publish must not resolve or execute another attempt")
			finalRun, err := runRepo.FindLatestByAssessmentID(ctx, created.ID)
			require.NoError(t, err)
			require.Equal(t, 1, finalRun.Attempt().Number)
			finalAssessment, err := assessmentRepo.FindByID(ctx, meta.FromUint64(created.ID))
			require.NoError(t, err)
			require.True(t, finalAssessment.Status().IsEvaluated())
			require.True(t, evaluationinput.IsIdentityRef(finalRun.InputSnapshotRef()))
			var outcomeCount int64
			require.NoError(t, db.Table("evaluation_outcome").Where("assessment_id=?", created.ID).Count(&outcomeCount).Error)
			require.EqualValues(t, 1, outcomeCount)
			committed, err := outcomes.FindByAssessmentID(ctx, meta.FromUint64(created.ID))
			require.NoError(t, err)
			require.NotNil(t, committed)
			require.Equal(t, "succeeded", finalRun.Attempt().Status.String())
			require.NotNil(t, finalAssessment.TotalScore())
			require.Equal(t, float64(7), *finalAssessment.TotalScore())
			var runs, failures, retries int64
			require.NoError(t, db.Table("runtime_checkpoint").Where("assessment_id=?", created.ID).Count(&runs).Error)
			require.EqualValues(t, 1, runs)
			require.NoError(t, db.Table("rm_outbox").Where("event_type=?", eventcatalog.EvaluationOutcomeCommitted).Count(&failures).Error)
			require.EqualValues(t, 1, failures)
			var failedIntents int64
			require.NoError(t, db.Table("rm_outbox").Where("event_type=?", eventcatalog.EvaluationFailed).Count(&failedIntents).Error)
			require.Zero(t, failedIntents)
			require.NoError(t, db.Table("rm_outbox").Where("event_type=?", eventcatalog.EvaluationRetryRequested).Count(&retries).Error)
			require.Zero(t, retries)
			var after []byte
			require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT payload FROM rm_outbox WHERE message_id=?", original.MessageID).Scan(&after))
			require.Equal(t, originalWire, after)
			waitM5ProcessNSQDrain(t, topic, channel)
			all[mode] = map[string]any{"event_id": original.MessageID, "assessment_id": created.ID, "run_id": finalRun.ID().String(), "attempt": finalRun.Attempt().Number, "runs": runs, "outcome_intents": failures, "outcomes": outcomeCount, "frozen_input_ref": finalRun.InputSnapshotRef(), "retry_intents": retries, "input_resolver_calls": input.calls.Load(), "wire_sha256": fmt.Sprintf("%x", sha256.Sum256(originalWire)), "physical_request_deliveries": deliveries, "lease_seconds": 30, "scope": "successful native factor_scoring Outcome; no external model/report or sustained pressure"}
			t.Logf("actual SQL %s: original event=%s; request deliveries=%d; Run=%s attempt=1; wire unchanged", mode, original.MessageID, len(deliveries), finalRun.ID().String())
		})
	}
	if !t.Failed() {
		encoded, err := json.MarshalIndent(all, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(os.Getenv("RM_M4_CONFIRM_RESULT"), encoded, 0600))
	}
}
