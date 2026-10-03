// answersheet_gap_recover restores the missing Assessment effect of one original event.
// It never republishes to the shared topic or invokes a model directly.
// It is an explicit operator action, never a retry scheduler or model retry.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	baselog "github.com/FangcunMount/component-base/pkg/log"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"io"
	"log/slog"
	"math"
	"os"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/answersheetgap"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/recoveryjournal"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type config struct {
	mode, auditDir, requestID, operator, reason, fingerprint string
	sheetID, orgID                                           uint64
	cutoff                                                   time.Time
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
	flags := flag.NewFlagSet("answersheet_gap_recover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.mode, "mode", "inspect", "inspect, apply, or reconcile (read-only journal)")
	flags.Uint64Var(&cfg.sheetID, "answersheet-id", 0, "one original accepted answersheet ID")
	flags.Uint64Var(&cfg.orgID, "org-id", 0, "reviewed organization scope")
	flags.StringVar(&cutoff, "accepted-before", "", "fixed reviewed RFC3339 acceptance cutoff")
	flags.StringVar(&cfg.fingerprint, "source-fingerprint", "", "exact inspect fingerprint for apply")
	flags.StringVar(&cfg.auditDir, "audit-dir", "", "designated existing private durable journal directory")
	flags.StringVar(&cfg.requestID, "request-id", "", "canonical recovery operation UUID")
	flags.StringVar(&cfg.operator, "operator", "", "accountable operator identity")
	flags.StringVar(&cfg.reason, "reason", "", "reviewed delivery loss and in-flight assessment verification")
	flags.BoolVar(&cfg.reviewed, "external-result-reviewed", false, "original acceptance and absence of unknown external effects reviewed")
	flags.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "overall bounded deadline (1s-2m)")
	if err := flags.Parse(args); err != nil {
		return cfg, err
	}
	if flags.NArg() != 0 || cfg.timeout < time.Second || cfg.timeout > 2*time.Minute {
		return cfg, errors.New("invalid bounded options")
	}
	if cfg.mode == "reconcile" {
		if cfg.auditDir == "" || cfg.requestID == "" {
			return cfg, errors.New("original journal operation required")
		}
		return cfg, nil
	}
	var err error
	cfg.cutoff, err = time.Parse(time.RFC3339Nano, cutoff)
	if err != nil || cfg.cutoff.After(now) || cfg.mode != "inspect" && cfg.mode != "apply" || cfg.sheetID == 0 || cfg.sheetID > math.MaxInt64 || cfg.orgID == 0 || cfg.orgID > math.MaxInt64 {
		return cfg, errors.New("fixed original answersheet, scope and past cutoff required")
	}
	if cfg.mode == "apply" {
		hash, err := hex.DecodeString(cfg.fingerprint)
		if err != nil || len(hash) != 32 || cfg.auditDir == "" || cfg.requestID == "" || strings.TrimSpace(cfg.operator) == "" || strings.TrimSpace(cfg.reason) == "" || !cfg.reviewed {
			return cfg, errors.New("explicit reviewed source and durable journal required")
		}
	}
	return cfg, nil
}

func runCLI(parent context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args, stderr, time.Now())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "answersheet recovery: invalid reviewed input")
		return 1
	}
	if cfg.mode == "reconcile" {
		intent, receipt, err := recoveryjournal.Read[answersheetgap.RecoveryPlan](cfg.auditDir, cfg.requestID)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "answersheet recovery: journal unavailable; do not repeat the effect")
			return 1
		}
		if err := json.NewEncoder(stdout).Encode(struct {
			Intent         recoveryjournal.Intent[answersheetgap.RecoveryPlan] `json:"intent"`
			Receipt        *recoveryjournal.Receipt                            `json:"receipt"`
			NoRepeatEffect bool                                                `json:"no_repeat_effect"`
		}{intent, receipt, true}); err != nil {
			return 1
		}
		if receipt == nil || receipt.EffectOutcome != "accepted" {
			return 2
		}
		return 0
	}
	uri, dbName, dsn := os.Getenv("MONGO_URI"), os.Getenv("MONGO_DB"), os.Getenv("MYSQL_DSN")
	if uri == "" || dbName == "" || dsn == "" {
		_, _ = fmt.Fprintln(stderr, "answersheet recovery: explicit host database configuration required")
		return 1
	}
	ctx, cancel := context.WithTimeout(parent, cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetMaxPoolSize(1))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "answersheet recovery: Mongo connection unavailable")
		return 1
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = client.Disconnect(cleanup)
	}()
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "answersheet recovery: MySQL connection unavailable")
		return 1
	}
	sqlDB, err := db.DB()
	if err != nil {
		return 1
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	scanner, err := answersheetgap.New(client.Database(dbName), db)
	if err != nil {
		return 1
	}
	plan, err := scanner.CaptureOriginalRecovery(ctx, cfg.sheetID, cfg.orgID, cfg.cutoff, time.Now())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "answersheet recovery: original source rejected or unavailable")
		return 1
	}
	if cfg.mode == "inspect" {
		if err := json.NewEncoder(stdout).Encode(plan); err != nil {
			return 1
		}
		return 0
	}
	if plan.SourceFingerprint != cfg.fingerprint {
		_, _ = fmt.Fprintln(stderr, "answersheet recovery: reviewed source changed")
		return 1
	}
	endpoint, ca, cert, key := os.Getenv("M6_QS03_GRPC_ENDPOINT"), os.Getenv("M6_QS03_CA_FILE"), os.Getenv("M6_QS03_CERT_FILE"), os.Getenv("M6_QS03_KEY_FILE")
	if endpoint == "" || ca == "" || cert == "" || key == "" {
		_, _ = fmt.Fprintln(stderr, "answersheet recovery: explicit reviewed Worker mTLS configuration required")
		return 1
	}
	manager, err := grpcclient.NewManager(&grpcclient.ManagerConfig{Endpoint: endpoint, Timeout: cfg.timeout, TLS: grpcclient.TLSConfig{CAFile: ca, CertFile: cert, KeyFile: key, ServerName: os.Getenv("M6_QS03_SERVER_NAME")}})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "answersheet recovery: secure client unavailable")
		return 1
	}
	defer manager.Close()
	return applyOriginal(ctx, scanner, workerEffect{grpcclient.NewAssessmentIntakeClient(manager)}, cfg, plan, stdout, stderr)
}

type originalSource interface {
	CaptureOriginalRecovery(context.Context, uint64, uint64, time.Time, time.Time) (answersheetgap.RecoveryPlan, error)
}

func applyOriginal(ctx context.Context, source originalSource, effect originalEffect, cfg config, plan answersheetgap.RecoveryPlan, stdout, stderr io.Writer) int {
	intent := recoveryjournal.Intent[answersheetgap.RecoveryPlan]{Plan: plan, RequestID: cfg.requestID, Operator: cfg.operator, Reason: cfg.reason, ExternalResultReviewed: cfg.reviewed}
	if err := recoveryjournal.Reserve(cfg.auditDir, intent); err != nil {
		_, _ = fmt.Fprintln(stderr, "answersheet recovery: journal reservation failed; do not bypass with a new request or directory")
		return 1
	}
	current, err := source.CaptureOriginalRecovery(ctx, cfg.sheetID, cfg.orgID, cfg.cutoff, time.Now())
	if err != nil || current.SourceFingerprint != plan.SourceFingerprint {
		_, _ = fmt.Fprintln(stderr, "answersheet recovery: source changed; reconcile reserved operation")
		return 2
	}
	response, callErr := effect.EnsureOriginal(ctx, current.Message())
	outcome := "unknown"
	if callErr == nil && response != nil && response.AssessmentId != 0 {
		outcome = "accepted"
	}
	receipt := recoveryjournal.Receipt{RequestID: cfg.requestID, EventID: plan.EventID, TransportOutcome: "not_sent", EffectOutcome: outcome, RecordedAt: time.Now().In(time.FixedZone("UTC+8", 8*3600))}
	if response != nil {
		receipt.AssessmentID = response.AssessmentId
	}
	if err := recoveryjournal.PersistExclusive(cfg.auditDir, cfg.requestID+".receipt.json", receipt); err != nil {
		_, _ = fmt.Fprintln(stderr, "answersheet recovery: receipt uncertain; do not repeat the effect")
		return 2
	}
	if err := json.NewEncoder(stdout).Encode(receipt); err != nil || outcome != "accepted" {
		return 2
	}
	return 0
}

// An RPC error or nil response is uncertain, never authority to retry.
type observedIntake struct {
	handlers.AssessmentIntakeClient
	response *pb.EnsureAssessmentResponse
}

func (c *observedIntake) EnsureAssessment(ctx context.Context, req *pb.EnsureAssessmentRequest) (*pb.EnsureAssessmentResponse, error) {
	response, err := c.AssessmentIntakeClient.EnsureAssessment(ctx, req)
	c.response = response
	return response, err
}

type originalEffect interface {
	EnsureOriginal(context.Context, message.Message) (*pb.EnsureAssessmentResponse, error)
}
type workerEffect struct {
	client handlers.AssessmentIntakeClient
}

func (e workerEffect) EnsureOriginal(ctx context.Context, original message.Message) (*pb.EnsureAssessmentResponse, error) {
	wire, recognized, err := legacy.Decode(original.Input().Payload)
	if err != nil || !recognized {
		return nil, errors.New("original wire unavailable")
	}
	tracked := &observedIntake{AssessmentIntakeClient: e.client}
	handler, ok := handlers.NewRegistry().Create("answersheet_submitted_handler", &handlers.Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AssessmentIntakeClient: tracked})
	if !ok {
		return nil, errors.New("normal Worker handler unavailable")
	}
	err = handler(ctx, "answersheet.submitted", wire.Payload)
	return tracked.response, err
}
