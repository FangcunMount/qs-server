//go:build integration && reliable_messaging_m4

package runtimeclosure_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	automation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	execution "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	interp "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	stage "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	evaluationfact "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/outbox"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	"github.com/FangcunMount/reliable-messaging/transport"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/google/uuid"
	driver "github.com/nsqio/go-nsq"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc/metadata"
)

// Real original transaction, Relay, SIGKILL and operator binary are used.
// The read-only Outcome port and report content builder are isolated fixtures;
// the normal Worker handler, frozen-input adapter, Starter and success commit
// are real. This is not a production MySQL/mTLS/model-quality proof.
func TestOriginalReportRetryRecoveryAfterActualNSQConfirmationLoss(t *testing.T) {
	uri := os.Getenv("M6_HOTRANK_MONGO_URI")
	if uri == "" {
		t.Skip("owned disposable Mongo/NSQ proof requires explicit environment")
	}
	address, httpAddress := os.Getenv("M6_HOTRANK_NSQ_ADDRESS"), os.Getenv("M6_HOTRANK_NSQ_HTTP")
	container, owner, tool := os.Getenv("M6_HOTRANK_NSQ_CONTAINER"), os.Getenv("M6_HOTRANK_OWNER"), os.Getenv("M6_RETRY_RECOVERY_TOOL")
	if !strings.HasPrefix(uri, "mongodb://127.0.0.1:") || !strings.Contains(uri, "directConnection=true") || !strings.HasPrefix(address, "127.0.0.1:") || !strings.HasPrefix(httpAddress, "http://127.0.0.1:") || !strings.HasPrefix(container, "m6-hotrank-") || owner == "" || !filepath.IsAbs(tool) || filepath.Base(tool) != "interpretation-retry-recover" {
		t.Fatal("only owned loopback resources and operator binary allowed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	db := client.Database("m6_retry_" + strings.ReplaceAll(owner, "-", ""))
	defer db.Drop(context.Background())
	generations, err := interp.NewGenerationRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := interp.NewRunRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := interp.NewReportRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := interp.NewReportCatalogProjector(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateCollection(ctx, "rm_outbox"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("rm_outbox").Indexes().CreateMany(ctx, sdkmongo.Indexes()); err != nil {
		t.Fatal(err)
	}
	wireConfig, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stager, err := stage.NewStager(db.Collection("rm_outbox"), eventcatalog.NewCatalog(wireConfig), "qs-apiserver")
	if err != nil {
		t.Fatal(err)
	}
	runner := apptransaction.RunnerFunc(func(ctx context.Context, callback func(context.Context) error) error {
		session, err := client.StartSession()
		if err != nil {
			return err
		}
		defer session.EndSession(ctx)
		_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (any, error) { return nil, callback(sc) })
		return err
	})
	starter, err := execution.NewStarter(runner, generations, runs, reports, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	key := domaingeneration.Key{OutcomeID: meta.New(), ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("v1")}
	started, err := starter.Start(ctx, execution.StartRequest{Key: key, TraceID: "m6-original-retry"})
	if err != nil {
		t.Fatal(err)
	}
	committer, err := execution.NewInterpretationCommitter(runner, generations, runs, reports, stager, nil, projection)
	if err != nil {
		t.Fatal(err)
	}
	// A terminal original failure needs one explicit business authorization.
	// The subsequent transport repair must never call Authorize again.
	assessmentID := meta.New()
	failed, err := committer.CommitFailure(ctx, execution.CommitFailureRequest{Generation: started.Generation, Run: started.Run, OutcomeID: key.OutcomeID, Association: domainreport.Association{OrgID: 1, AssessmentID: assessmentID, TesteeID: 8}, Failure: interpretationrun.Failure{Kind: interpretationrun.FailureKindBuild, Code: "controlled_build_failure", SafeMessage: "isolated pre-model failure", Retryable: false}, FailedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	outcome := originalRetryOutcome(t, key.OutcomeID, assessmentID)
	repo := originalRetryOutcomeRepo{record: outcome}
	governance := automation.NewGovernedRetryService(generations, runs, repo, runner, stager)
	authority := automation.GovernedRetryCommand{OrgID: 1, GenerationID: started.Generation.ID(), ExpectedAttempt: 1, Origin: retrygovernance.AttemptOriginForce, RequestID: "original-business-authority", Reason: "isolated confirmed pre-model failure"}
	authorized, err := governance.Authorize(ctx, authority)
	if err != nil {
		t.Fatal(err)
	}
	decision := authorized.RetryDecision()
	if decision == nil || decision.RetryEventID == "" {
		t.Fatal("original authority missing")
	}
	var original struct {
		Payload []byte `bson:"payload"`
		State   string `bson:"state"`
	}
	if err := db.Collection("rm_outbox").FindOne(ctx, bson.M{"message_id": decision.RetryEventID}).Decode(&original); err != nil {
		t.Fatal(err)
	}
	store, err := sdkmongo.New(db.Collection("rm_outbox"))
	if err != nil {
		t.Fatal(err)
	}
	producer, err := driver.NewProducer(address, driver.NewConfig())
	if err != nil {
		t.Fatal(err)
	}
	producer.SetLogger(nil, driver.LogLevelError)
	publisher, err := sdknsq.New(producer, map[string]string{"qs.evaluation.lifecycle": "qs.evaluation.lifecycle"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	r, err := relay.New(store, publisher, relay.Config{Concurrency: 2, PollInterval: 10 * time.Millisecond, Lease: 10 * time.Second, PublishTimeout: 2 * time.Second, WriteTimeout: time.Second, Retry: func(outbox.Claim, transport.Outcome) relay.RetryDecision {
		return relay.RetryDecision{Quarantine: true}
	}, Observe: func(relay.Event) {}})
	if err != nil {
		t.Fatal(err)
	}
	rctx, stopRelay := context.WithCancel(ctx)
	rdone := make(chan error, 1)
	go func() { rdone <- r.Run(rctx) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		n, err := db.Collection("rm_outbox").CountDocuments(ctx, bson.M{"state": "published"})
		if err != nil {
			t.Fatal(err)
		}
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("original failure/retry intents did not receive real NSQ confirmation")
		}
		time.Sleep(20 * time.Millisecond)
	}
	stopRelay()
	if err := <-rdone; err != nil {
		t.Fatal(err)
	}
	drain, stop := context.WithTimeout(context.Background(), 3*time.Second)
	if err := publisher.Drain(drain); err != nil {
		t.Fatal(err)
	}
	stop()
	producer.Stop()
	if depth := hotProofNSQDepth(t, httpAddress); depth != 2 {
		t.Fatalf("confirmed original depth=%d want2", depth)
	}
	httpAddress = hotProofRestartOwnedNSQ(t, container, owner)
	if depth := hotProofNSQDepth(t, httpAddress); depth != 0 {
		t.Fatalf("confirmed original messages survived configured volatile fault: depth=%d", depth)
	}
	rawInspect, err := exec.Command("docker", "inspect", container).Output()
	if err != nil {
		t.Fatal(err)
	}
	var state []hotProofContainer
	if err := json.Unmarshal(rawInspect, &state); err != nil || len(state) != 1 {
		t.Fatal("owned restarted broker identity unavailable", err)
	}
	bindings := state[0].NetworkSettings.Ports["4150/tcp"]
	if len(bindings) != 1 || bindings[0].HostIP != "127.0.0.1" {
		t.Fatal("loopback TCP mapping unavailable")
	}
	address = "127.0.0.1:" + bindings[0].HostPort
	auditDir := t.TempDir()
	if err := os.Chmod(auditDir, 0700); err != nil {
		t.Fatal(err)
	}
	runTool := func(want int, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, tool, args...)
		cmd.Env = append(os.Environ(), "MONGO_URI="+uri, "MONGO_DB="+db.Name(), "M6_RETRY_NSQ_ADDRESS="+address)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		raw, err := cmd.Output()
		code := 0
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				code = exit.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		if code != want {
			t.Fatalf("operator exit=%d want=%d safe-error=%s", code, want, stderr.String())
		}
		return raw
	}
	inspectArgs := []string{"--mode=inspect", "--run-id=" + failed.Run.ID().String(), "--org-id=1"}
	var plan struct {
		EventID     string `json:"event_id"`
		Fingerprint string `json:"source_fingerprint"`
	}
	if err := json.Unmarshal(runTool(0, inspectArgs...), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.EventID != decision.RetryEventID {
		t.Fatal("operator invented a new retry identity")
	}
	requestID := uuid.NewString()
	applyArgs := []string{"--mode=apply", "--run-id=" + failed.Run.ID().String(), "--org-id=1", "--source-fingerprint=" + plan.Fingerprint, "--audit-dir=" + auditDir, "--request-id=" + requestID, "--operator=isolated-m6-operator", "--reason=original-authority-confirmed-broker-loss-no-model-call", "--external-result-reviewed"}
	wrong := append([]string(nil), applyArgs...)
	wrong[3] = "--source-fingerprint=" + strings.Repeat("0", 64)
	runTool(1, wrong...)
	runTool(1, "--mode=inspect", "--run-id="+failed.Run.ID().String(), "--org-id=2")
	if depth := hotProofNSQDepth(t, httpAddress); depth != 0 {
		t.Fatal("rejected operator attempt published a message")
	}
	runTool(0, applyArgs...)
	runTool(0, "--mode=reconcile", "--audit-dir="+auditDir, "--request-id="+requestID)
	if depth := hotProofNSQDepth(t, httpAddress); depth != 1 {
		t.Fatalf("original-only operator PUB depth=%d want1", depth)
	}
	// Same operator request cannot publish again before consumption either.
	runTool(1, applyArgs...)
	if depth := hotProofNSQDepth(t, httpAddress); depth != 1 {
		t.Fatal("repeated operator request duplicated PUB")
	}
	builder := &originalRetryBuilder{}
	builders, err := rendering.NewRegistry(builder)
	if err != nil {
		t.Fatal(err)
	}
	executor, err := execution.NewExecutor(starter, builders, committer)
	if err != nil {
		t.Fatal(err)
	}
	service, err := automation.NewService(repo, executor)
	if err != nil {
		t.Fatal(err)
	}
	boundary := &originalRetryCallBoundary{service: service, eventID: decision.RetryEventID, actionID: authority.RequestID, outcome: outcome}
	deps := &handlers.Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), InterpretationAutomationClient: boundary}
	handler, ok := handlers.NewRegistry().Create("interpretation_retry_requested_handler", deps)
	if !ok {
		t.Fatal("normal retry handler unavailable")
	}
	cfg := driver.NewConfig()
	cfg.HeartbeatInterval = time.Second
	cfg.ReadTimeout = 3 * time.Second
	cfg.WriteTimeout = time.Second
	consumer, err := driver.NewConsumer("qs.evaluation.lifecycle", "m6-original-retry", cfg)
	if err != nil {
		t.Fatal(err)
	}
	consumer.SetLogger(nil, driver.LogLevelError)
	consumer.AddHandler(driver.HandlerFunc(func(raw *driver.Message) error {
		if !bytes.Equal(raw.Body, original.Payload) {
			return fmt.Errorf("recovery wire changed")
		}
		decoded, recognized, err := legacy.Decode(raw.Body)
		if err != nil || !recognized {
			return fmt.Errorf("unrecognized original wire")
		}
		return handler(ctx, "interpretation.retry.requested", decoded.Payload)
	}))
	if err := consumer.ConnectToNSQD(address); err != nil {
		t.Fatal(err)
	}
	defer func() {
		consumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("owned consumer failed to stop")
		}
	}()
	late, err := driver.NewProducer(address, cfg)
	if err != nil {
		t.Fatal(err)
	}
	late.SetLogger(nil, driver.LogLevelError)
	defer late.Stop()
	for range 2 {
		if err := late.Publish("qs.evaluation.lifecycle", original.Payload); err != nil {
			t.Fatal(err)
		}
	}
	deadline = time.Now().Add(15 * time.Second)
	for {
		if consumer.Stats().MessagesFinished == 3 && originalRetryChannelSettled(t, httpAddress) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("original and late duplicates did not FIN with settled broker channel")
		}
		time.Sleep(20 * time.Millisecond)
	}
	latest, err := runs.FindLatestByGenerationID(ctx, started.Generation.ID())
	if err != nil || latest.Attempt() != 2 || latest.Status() != interpretationrun.StatusSucceeded {
		t.Fatalf("unique original successor missing: %+v err=%v", latest, err)
	}
	if builder.calls.Load() != 1 || boundary.calls.Load() != 3 {
		t.Fatalf("actual builder=%d handler calls=%d", builder.calls.Load(), boundary.calls.Load())
	}
	for name, want := range map[string]int64{"interpretation_runs": 2, "report_generations": 1, "interpret_report_artifacts": 1} {
		n, err := db.Collection(name).CountDocuments(ctx, bson.M{})
		if err != nil || n != want {
			t.Fatalf("%s=%d want%d err=%v", name, n, want, err)
		}
	}
	generated, err := generations.FindByID(ctx, started.Generation.ID())
	if err != nil || generated.Key() != key {
		t.Fatal("original frozen report identity changed", err)
	}
	var after struct {
		Payload []byte `bson:"payload"`
		State   string `bson:"state"`
	}
	if err := db.Collection("rm_outbox").FindOne(ctx, bson.M{"message_id": decision.RetryEventID}).Decode(&after); err != nil || after.State != "published" || !bytes.Equal(after.Payload, original.Payload) {
		t.Fatal("recovery changed original durable intent", err)
	}
	runTool(1, applyArgs...)
	if _, err := governance.Authorize(ctx, authority); err == nil {
		t.Fatal("consumed original authorization accepted a new authorization")
	}
	t.Logf("confirmed original depth2->SIGKILL137->depth0; actual CLI original PUB1; normal Worker FIN3; unique successor attempt2/artifact1; builder1; original action/config/payload unchanged; no model/WeChat/production calls")
}

type originalRetryOutcomeRepo struct{ record *evaluationfact.Record }

func (r originalRetryOutcomeRepo) FindByID(_ context.Context, id meta.ID) (*evaluationfact.Record, error) {
	if r.record == nil || r.record.ID() != id {
		return nil, evaluationfact.ErrNotFound
	}
	return r.record, nil
}
func (r originalRetryOutcomeRepo) FindByAssessmentID(_ context.Context, id meta.ID) (*evaluationfact.Record, error) {
	if r.record == nil || r.record.AssessmentID() != id {
		return nil, evaluationfact.ErrNotFound
	}
	return r.record, nil
}

func originalRetryOutcome(t *testing.T, outcomeID, assessmentID meta.ID) *evaluationfact.Record {
	t.Helper()
	input, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{Assets: &interpretationassets.Assets{ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{Code: "standard", Kind: "factor_scoring", TemplateID: "standard", TemplateVersion: "v1"}}}}, ModelRef: evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindScale, Algorithm: string(modelcatalog.AlgorithmScaleDefault), Code: "SCALE-1", Version: "v1", Title: "Scale"}, DecisionKind: modelcatalog.DecisionKindScoreRange, FactorCatalog: []evaluationinput.FactorCatalogEntry{{Code: "TOTAL", Title: "总分", IsTotalScore: true}}})
	if err != nil {
		t.Fatal(err)
	}
	return evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: outcomeID, OrgID: 1, AssessmentID: assessmentID, TesteeID: 8, Model: evaluationfact.ModelIdentity{Kind: modelcatalog.KindScale, Algorithm: modelcatalog.AlgorithmScaleDefault, Code: "SCALE-1", Version: "v1", Title: "Scale"}, Runtime: evaluationfact.RuntimeIdentity{DecisionKind: modelcatalog.DecisionKindScoreRange}, SchemaVersion: 2, ReportInput: input, EvaluatedAt: time.Now(), Payload: []byte(`{"Primary":{"Kind":"raw_total","Value":12},"Level":{"Code":"low"},"Dimensions":[{"Code":"TOTAL","Role":"total","Score":{"Kind":"raw_total","Value":12},"Level":{"Code":"low"}}]}`)})
}

type originalRetryBuilder struct{ calls atomic.Int32 }

func (*originalRetryBuilder) ReportType() policy.ReportType { return policy.ReportTypeStandard }
func (*originalRetryBuilder) TemplateVersion() policy.TemplateVersion {
	return policy.TemplateVersion("v1")
}
func (*originalRetryBuilder) BuilderIdentity() string {
	return domainreport.BuilderIdentityFactorScoring
}
func (*originalRetryBuilder) ContentSchemaVersion() string { return "report-content/v1" }
func (*originalRetryBuilder) MechanismKey() rendering.Key {
	return rendering.Key{DecisionKind: modelcatalog.DecisionKindScoreRange, ReportType: policy.ReportTypeStandard}
}
func (b *originalRetryBuilder) Build(_ context.Context, input interpinput.InterpretationInput) (*domainreport.Draft, error) {
	b.calls.Add(1)
	if input.Report.TemplateVersion != "v1" || input.Model.Version != "v1" {
		return nil, fmt.Errorf("frozen input changed")
	}
	return domainreport.NewDraft(domainreport.Content{Model: domainreport.ModelIdentity{Kind: "scale", Code: "SCALE-1", Version: "v1", Title: "Scale"}, PrimaryScore: domainreport.NewRawTotalScore(12, nil), Level: domainreport.LevelFromRisk(domainreport.RiskLevelLow), Conclusion: "ok", Dimensions: []domainreport.DimensionInterpret{domainreport.NewDimensionInterpret(domainreport.NewFactorCode("TOTAL"), "总分", 12, nil, domainreport.RiskLevelLow, "ok", "ok")}}), nil
}

// This fixture only bridges the existing client's outgoing metadata to the
// real application service. No RPC/network or production identity is claimed.
type originalRetryCallBoundary struct {
	service           automation.Service
	eventID, actionID string
	outcome           *evaluationfact.Record
	calls             atomic.Int32
}

func (b *originalRetryCallBoundary) GenerateReportFromOutcome(ctx context.Context, id string) (*interpretationpb.GenerateReportFromAssessmentResponse, error) {
	md, _ := metadata.FromOutgoingContext(ctx)
	get := func(key string) string {
		values := md.Get(key)
		if len(values) != 1 {
			return ""
		}
		return values[0]
	}
	attempt, err := strconv.Atoi(get("x-retry-expected-attempt"))
	if err != nil || attempt != 1 || id != b.outcome.ID().String() || get("x-event-id") != b.eventID || get("x-retry-event-id") != b.eventID || get("x-retry-action-request-id") != b.actionID || get("x-retry-origin") != "force" || get("x-retry-mode") != "next_attempt" {
		return nil, fmt.Errorf("original authority metadata changed")
	}
	b.calls.Add(1)
	ctx = retrygovernance.WithAuthorization(ctx, retrygovernance.Authorization{EventID: b.eventID, ExpectedAttempt: attempt, Origin: retrygovernance.AttemptOriginForce, ActionRequestID: b.actionID, Mode: "next_attempt"})
	result, err := b.service.Generate(ctx, automation.GenerateCommand{Actor: automation.TrustedServiceActor("m6-original-retry-proof"), OutcomeID: b.outcome.ID()})
	if err != nil {
		return nil, err
	}
	return &interpretationpb.GenerateReportFromAssessmentResponse{Success: result.Status == automation.StatusGenerated, Status: string(result.Status), RunId: result.RunID.String(), ReportId: result.ReportID.String()}, nil
}

func originalRetryChannelSettled(t *testing.T, address string) bool {
	t.Helper()
	client := http.Client{Timeout: time.Second}
	resp, err := client.Get(address + "/stats?format=json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("owned NSQ stats unavailable")
	}
	var stats struct {
		Topics []struct {
			Name     string `json:"topic_name"`
			Channels []struct {
				Name     string `json:"channel_name"`
				Depth    int64  `json:"depth"`
				InFlight int64  `json:"in_flight_count"`
				Deferred int64  `json:"deferred_count"`
				Count    uint64 `json:"message_count"`
				Requeue  uint64 `json:"requeue_count"`
			} `json:"channels"`
		} `json:"topics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	for _, topic := range stats.Topics {
		if topic.Name != "qs.evaluation.lifecycle" {
			continue
		}
		for _, c := range topic.Channels {
			if c.Name == "m6-original-retry" {
				return c.Depth == 0 && c.InFlight == 0 && c.Deferred == 0 && c.Count == 3 && c.Requeue == 0
			}
		}
	}
	return false
}
