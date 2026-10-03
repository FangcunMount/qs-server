// effect_gap_recover applies one original, unconsumed business authorization.
// It is an operator command, not a scheduler, and never publishes to NSQ.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	baselog "github.com/FangcunMount/component-base/pkg/log"
	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	reportpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/originaleffect"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/recoveryjournal"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type config struct {
	mode, auditDir, requestID, operator, reason, fingerprint string
	request                                                  originaleffect.Request
	reviewed                                                 bool
	timeout                                                  time.Duration
}

func main() {
	opts := baselog.NewOptions()
	opts.OutputPaths = []string{"stderr"}
	baselog.Init(opts)
	os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func parseConfig(args []string, stderr io.Writer, now time.Time) (config, error) {
	var cfg config
	var cutoff string
	flags := flag.NewFlagSet("effect_gap_recover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.mode, "mode", "inspect", "inspect, apply, reconcile (offline journal)")
	flags.StringVar(&cfg.request.EventID, "event-id", "", "exact original event ID; not necessarily UUID")
	flags.Uint64Var(&cfg.request.AssessmentID, "assessment-id", 0, "one original assessment ID")
	flags.Uint64Var(&cfg.request.OrgID, "org-id", 0, "reviewed organization scope")
	flags.StringVar(&cutoff, "accepted-before", "", "fixed reviewed RFC3339 acceptance cutoff")
	flags.StringVar(&cfg.fingerprint, "source-fingerprint", "", "exact inspect fingerprint for apply")
	flags.StringVar(&cfg.auditDir, "audit-dir", "", "designated existing private durable journal")
	flags.StringVar(&cfg.requestID, "request-id", "", "canonical recovery operation UUID")
	flags.StringVar(&cfg.operator, "operator", "", "accountable operator identity")
	flags.StringVar(&cfg.reason, "reason", "", "reviewed delivery loss and external result check")
	flags.BoolVar(&cfg.reviewed, "external-result-reviewed", false, "original acceptance and unknown external effects reviewed")
	flags.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "overall bounded deadline (1s-2m)")
	if err := flags.Parse(args); err != nil {
		return cfg, err
	}
	if flags.NArg() != 0 || cfg.timeout < time.Second || cfg.timeout > 2*time.Minute {
		return cfg, errors.New("invalid bounded options")
	}
	if cfg.mode == "reconcile" {
		id, err := uuid.Parse(cfg.requestID)
		if err != nil || id.String() != cfg.requestID || cfg.auditDir == "" {
			return cfg, errors.New("original journal operation required")
		}
		return cfg, nil
	}
	var err error
	cfg.request.AcceptedBefore, err = time.Parse(time.RFC3339Nano, cutoff)
	if err != nil || cfg.request.AcceptedBefore.After(now) || cfg.mode != "inspect" && cfg.mode != "apply" || strings.TrimSpace(cfg.request.EventID) == "" || len(cfg.request.EventID) > 128 || cfg.request.AssessmentID == 0 || cfg.request.AssessmentID > math.MaxInt64 || cfg.request.OrgID == 0 || cfg.request.OrgID > math.MaxInt64 {
		return cfg, errors.New("fixed original identity, scope and past cutoff required")
	}
	if cfg.mode == "apply" {
		hash, err := hex.DecodeString(cfg.fingerprint)
		id, idErr := uuid.Parse(cfg.requestID)
		if err != nil || len(hash) != 32 || cfg.fingerprint != strings.ToLower(cfg.fingerprint) || idErr != nil || id.String() != cfg.requestID || cfg.auditDir == "" || strings.TrimSpace(cfg.operator) == "" || strings.TrimSpace(cfg.reason) == "" || !cfg.reviewed {
			return cfg, errors.New("explicit reviewed source and durable journal required")
		}
	}
	return cfg, nil
}

func runCLI(parent context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args, stderr, time.Now())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "original effect recovery: invalid reviewed input")
		return 1
	}
	if cfg.mode == "reconcile" {
		intent, receipt, err := recoveryjournal.Read[originaleffect.Plan](cfg.auditDir, cfg.requestID)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "original effect recovery: journal unavailable; do not repeat")
			return 1
		}
		if err := json.NewEncoder(stdout).Encode(struct {
			Intent         recoveryjournal.Intent[originaleffect.Plan] `json:"intent"`
			Receipt        *recoveryjournal.Receipt                    `json:"receipt"`
			NoRepeatEffect bool                                        `json:"no_repeat_effect"`
		}{intent, receipt, true}); err != nil {
			return 1
		}
		if receipt == nil || receipt.EffectOutcome != "accepted" {
			return 2
		}
		return 0
	}
	dsn, uri, dbName := os.Getenv("MYSQL_DSN"), os.Getenv("MONGO_URI"), os.Getenv("MONGO_DB")
	if dsn == "" || uri == "" || dbName == "" {
		_, _ = fmt.Fprintln(stderr, "original effect recovery: explicit host database configuration required")
		return 1
	}
	ctx, cancel := context.WithTimeout(parent, cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetMaxPoolSize(1))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "original effect recovery: Mongo connection unavailable")
		return 1
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = client.Disconnect(cleanup)
	}()
	// Keep the initial handshake inside the operation's context-bound reads.
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{DSN: dsn, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "original effect recovery: MySQL connection unavailable")
		return 1
	}
	sqlDB, err := db.DB()
	if err != nil {
		return 1
	}
	defer func() { _ = sqlDB.Close() }()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	source := originaleffect.Reader{SQL: db, Mongo: client.Database(dbName)}
	plan, err := source.Capture(ctx, cfg.request, time.Now())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "original effect recovery: original authority rejected or unavailable")
		return 1
	}
	if cfg.mode == "inspect" {
		if err := json.NewEncoder(stdout).Encode(plan); err != nil {
			return 1
		}
		return 0
	}
	if plan.SourceFingerprint != cfg.fingerprint {
		_, _ = fmt.Fprintln(stderr, "original effect recovery: reviewed source changed")
		return 1
	}
	endpoint, ca, cert, key := os.Getenv("M6_EFFECT_GRPC_ENDPOINT"), os.Getenv("M6_EFFECT_CA_FILE"), os.Getenv("M6_EFFECT_CERT_FILE"), os.Getenv("M6_EFFECT_KEY_FILE")
	if endpoint == "" || ca == "" || cert == "" || key == "" {
		_, _ = fmt.Fprintln(stderr, "original effect recovery: explicit Worker mTLS configuration required")
		return 1
	}
	manager, err := grpcclient.NewManager(&grpcclient.ManagerConfig{Endpoint: endpoint, Timeout: cfg.timeout, TLS: grpcclient.TLSConfig{CAFile: ca, CertFile: cert, KeyFile: key, ServerName: os.Getenv("M6_EFFECT_SERVER_NAME")}})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "original effect recovery: secure client unavailable")
		return 1
	}
	defer func() { _ = manager.Close() }()
	effect := workerEffect{evaluation: grpcclient.NewEvaluationWorkerClient(manager), report: grpcclient.NewInterpretationAutomationClient(manager)}
	return applyOriginal(ctx, source, effect, cfg, plan, stdout, stderr)
}

type originalSource interface {
	Capture(context.Context, originaleffect.Request, time.Time) (originaleffect.Plan, error)
}
type originalEffect interface {
	Apply(context.Context, originaleffect.Plan) (effectResult, error)
}
type effectResult struct {
	RunID, GenerationID, OutcomeID string
	Accepted                       bool
}

func applyOriginal(ctx context.Context, source originalSource, effect originalEffect, cfg config, plan originaleffect.Plan, stdout, stderr io.Writer) int {
	intent := recoveryjournal.Intent[originaleffect.Plan]{Plan: plan, RequestID: cfg.requestID, Operator: cfg.operator, Reason: cfg.reason, ExternalResultReviewed: cfg.reviewed}
	if err := recoveryjournal.Reserve(cfg.auditDir, intent); err != nil {
		_, _ = fmt.Fprintln(stderr, "original effect recovery: reservation failed; do not bypass with a new request or directory")
		return 1
	}
	current, err := source.Capture(ctx, cfg.request, time.Now())
	if err != nil || current.SourceFingerprint != plan.SourceFingerprint {
		_, _ = fmt.Fprintln(stderr, "original effect recovery: source changed; reconcile reserved operation")
		return 2
	}
	result, callErr := effect.Apply(ctx, current)
	outcome := "unknown"
	if callErr == nil && result.Accepted {
		outcome = "accepted"
	}
	receipt := recoveryjournal.Receipt{RequestID: cfg.requestID, EventID: plan.EventID, TransportOutcome: "not_sent", EffectOutcome: outcome, AssessmentID: plan.AssessmentID, RunID: result.RunID, GenerationID: result.GenerationID, OutcomeID: result.OutcomeID, RecordedAt: time.Now().In(time.FixedZone("UTC+8", 8*3600))}
	if err := recoveryjournal.PersistExclusive(cfg.auditDir, cfg.requestID+".receipt.json", receipt); err != nil {
		_, _ = fmt.Fprintln(stderr, "original effect recovery: receipt uncertain; do not repeat")
		return 2
	}
	if err := json.NewEncoder(stdout).Encode(receipt); err != nil || outcome != "accepted" {
		return 2
	}
	return 0
}

type observedEvaluation struct {
	handlers.EvaluationWorkerClient
	response *evalpb.ExecuteEvaluationResponse
}

func (c *observedEvaluation) ExecuteEvaluation(ctx context.Context, id uint64) (*evalpb.ExecuteEvaluationResponse, error) {
	response, err := c.EvaluationWorkerClient.ExecuteEvaluation(ctx, id)
	c.response = response
	return response, err
}

type observedReport struct {
	handlers.InterpretationAutomationClient
	response *reportpb.GenerateReportFromAssessmentResponse
}

func (c *observedReport) GenerateReportFromOutcome(ctx context.Context, id string) (*reportpb.GenerateReportFromAssessmentResponse, error) {
	response, err := c.InterpretationAutomationClient.GenerateReportFromOutcome(ctx, id)
	c.response = response
	return response, err
}

type workerEffect struct {
	evaluation handlers.EvaluationWorkerClient
	report     handlers.InterpretationAutomationClient
}

func (e workerEffect) Apply(ctx context.Context, plan originaleffect.Plan) (effectResult, error) {
	wire, recognized, err := legacy.Decode(plan.Message().Input().Payload)
	if err != nil || !recognized || wire.UUID != plan.EventID {
		return effectResult{}, errors.New("original wire unavailable")
	}
	return e.applyWire(ctx, plan, wire)
}

func (e workerEffect) applyWire(ctx context.Context, plan originaleffect.Plan, wire legacy.Envelope) (effectResult, error) {
	deps := &handlers.Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var name string
	eval := &observedEvaluation{EvaluationWorkerClient: e.evaluation}
	report := &observedReport{InterpretationAutomationClient: e.report}
	switch plan.EventType {
	case originaleffect.EvaluationRetry:
		if e.evaluation == nil {
			return effectResult{}, errors.New("normal evaluation client unavailable")
		}
		name, deps.EvaluationWorkerClient = "evaluation_requested_handler", eval
	case originaleffect.ReportInitial:
		if e.report == nil {
			return effectResult{}, errors.New("normal report client unavailable")
		}
		name, deps.InterpretationAutomationClient = "evaluation_outcome_committed_handler", report
	default:
		return effectResult{}, errors.New("unsupported original effect")
	}
	handler, ok := handlers.NewRegistry().Create(name, deps)
	if !ok {
		return effectResult{}, errors.New("normal Worker handler unavailable")
	}
	err := handler(ctx, plan.EventType, wire.Payload)
	var result effectResult
	if plan.EventType == originaleffect.EvaluationRetry && eval.response != nil {
		result.RunID, result.OutcomeID = eval.response.RunId, eval.response.OutcomeId
		// The original failed Run returned by a blocked authorization is not
		// a new acceptance. Require the actual one-step successor identity.
		wantRun := strconv.FormatUint(plan.AssessmentID, 10) + ":" + strconv.Itoa(plan.ExpectedAttempt+1)
		result.Accepted = err == nil && plan.ExpectedAttempt > 0 && result.RunID == wantRun && result.RunID != plan.PreviousRunID && int(eval.response.CurrentAttempt) == plan.ExpectedAttempt+1
	}
	if plan.EventType == originaleffect.ReportInitial && report.response != nil {
		result.RunID, result.GenerationID, result.OutcomeID = report.response.RunId, report.response.GenerationId, plan.OutcomeID
		generation, genErr := strconv.ParseUint(result.GenerationID, 10, 64)
		run, runErr := strconv.ParseUint(result.RunID, 10, 64)
		status := report.response.Status
		result.Accepted = err == nil && genErr == nil && runErr == nil && generation > 0 && run > 0 && (status == "generated" || status == "processing" || status == "already_generated" || status == "failed")
	}
	return result, err
}
