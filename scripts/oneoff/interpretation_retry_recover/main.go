// interpretation_retry_recover restores only an already authorized original
// report retry notification. Inspect is read-only; apply is an explicit,
// audited, single-use operator action, never a background retry scheduler.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/FangcunMount/reliable-messaging/transport"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	driver "github.com/nsqio/go-nsq"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type config struct {
	mode, auditDir, requestID, operator, reason, fingerprint string
	runID                                                    uint64
	orgID                                                    int64
	reviewed                                                 bool
	timeout                                                  time.Duration
}

func main() { os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr)) }

func parseConfig(args []string, stderr io.Writer) (config, error) {
	var cfg config
	flags := flag.NewFlagSet("interpretation_retry_recover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.mode, "mode", "inspect", "inspect, apply, or reconcile (read-only)")
	flags.Uint64Var(&cfg.runID, "run-id", 0, "one original failed Run ID")
	flags.Int64Var(&cfg.orgID, "org-id", 0, "reviewed organization scope")
	flags.StringVar(&cfg.fingerprint, "source-fingerprint", "", "exact inspect fingerprint for apply")
	flags.StringVar(&cfg.auditDir, "audit-dir", "", "designated private durable audit directory")
	flags.StringVar(&cfg.requestID, "request-id", "", "canonical recovery operation UUID")
	flags.StringVar(&cfg.operator, "operator", "", "accountable operator identity")
	flags.StringVar(&cfg.reason, "reason", "", "reviewed delivery loss and external result resolution")
	flags.BoolVar(&cfg.reviewed, "external-result-reviewed", false, "operator has verified no unknown model call is being replayed")
	flags.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "overall bounded deadline (1s-2m)")
	if err := flags.Parse(args); err != nil {
		return cfg, err
	}
	if flags.NArg() != 0 || cfg.timeout < time.Second || cfg.timeout > 2*time.Minute {
		return cfg, errors.New("invalid bounded options")
	}
	if cfg.mode == "reconcile" {
		if cfg.auditDir == "" || cfg.requestID == "" {
			return cfg, errors.New("reconcile requires original operation")
		}
		return cfg, nil
	}
	if cfg.mode != "inspect" && cfg.mode != "apply" || cfg.runID == 0 || cfg.runID > math.MaxInt64 || cfg.orgID <= 0 {
		return cfg, errors.New("fixed Run and organization required")
	}
	if cfg.mode == "apply" {
		hash, err := hex.DecodeString(cfg.fingerprint)
		if err != nil || len(hash) != 32 || cfg.auditDir == "" || cfg.requestID == "" || strings.TrimSpace(cfg.operator) == "" || strings.TrimSpace(cfg.reason) == "" || !cfg.reviewed {
			return cfg, errors.New("explicit reviewed source and durable audit required")
		}
	}
	return cfg, nil
}

func runtimeMongo() (string, string) {
	if uri, db := os.Getenv("MONGO_URI"), os.Getenv("MONGO_DB"); uri != "" && db != "" {
		return uri, db
	}
	host, user, password, db := os.Getenv("QS_APISERVER_MONGODB_HOST"), os.Getenv("QS_APISERVER_MONGODB_USERNAME"), os.Getenv("QS_APISERVER_MONGODB_PASSWORD"), os.Getenv("QS_APISERVER_MONGODB_DATABASE")
	if host == "" || user == "" || password == "" || db == "" {
		return "", ""
	}
	uri := url.URL{Scheme: "mongodb", User: url.UserPassword(user, password), Host: host, Path: "/" + db}
	uri.RawQuery = url.Values{"authSource": {db}, "directConnection": {"true"}}.Encode()
	return uri.String(), db
}

func runCLI(parent context.Context, args []string, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "original retry recovery: invalid reviewed input")
		return 1
	}
	if cfg.mode == "reconcile" {
		intent, receipt, err := readOperation(cfg.auditDir, cfg.requestID)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "original retry recovery: audit read failed; do not republish")
			return 1
		}
		if err := json.NewEncoder(stdout).Encode(struct {
			Intent      operationIntent   `json:"intent"`
			Receipt     *operationReceipt `json:"receipt"`
			NoRepublish bool              `json:"no_republish"`
		}{intent, receipt, true}); err != nil {
			return 1
		}
		if receipt == nil || receipt.TransportOutcome != "confirmed" {
			return 2
		}
		return 0
	}
	uri, dbName := runtimeMongo()
	if uri == "" {
		_, _ = fmt.Fprintln(stderr, "original retry recovery: host Mongo configuration unavailable")
		return 1
	}
	ctx, cancel := context.WithTimeout(parent, cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "original retry recovery: Mongo connection failed")
		return 1
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = client.Disconnect(cleanup)
	}()
	plan, err := captureOriginal(ctx, client.Database(dbName), cfg.runID, cfg.orgID, time.Now())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "original retry recovery: source rejected or unavailable")
		return 1
	}
	if cfg.mode == "inspect" {
		if err := json.NewEncoder(stdout).Encode(plan); err != nil {
			return 1
		}
		return 0
	}
	if plan.SourceFingerprint != cfg.fingerprint {
		_, _ = fmt.Fprintln(stderr, "original retry recovery: reviewed source changed")
		return 1
	}
	address := os.Getenv("M6_RETRY_NSQ_ADDRESS")
	if address == "" {
		_, _ = fmt.Fprintln(stderr, "original retry recovery: explicit reviewed NSQ address required")
		return 1
	}
	producer, err := newRecoveryProducer(address)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "original retry recovery: NSQ producer configuration invalid")
		return 1
	}
	producer.SetLogger(nil, driver.LogLevelError)
	defer producer.Stop()
	publisher, err := sdknsq.New(producer, map[string]string{retryDestination: retryDestination}, 1)
	if err != nil {
		return 1
	}
	defer func() {
		drainCtx, stop := context.WithTimeout(context.Background(), 4*time.Second)
		err := publisher.Drain(drainCtx)
		stop()
		if err != nil {
			producer.Stop()
			finalCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = publisher.Drain(finalCtx)
		}
	}()
	intent := operationIntent{Plan: plan, RequestID: cfg.requestID, Operator: cfg.operator, Reason: cfg.reason, ExternalResultReviewed: cfg.reviewed}
	if err := reserveOperation(cfg.auditDir, intent); err != nil {
		_, _ = fmt.Fprintln(stderr, "original retry recovery: audit reservation failed; do not bypass with another request or directory")
		return 1
	}
	// Revalidate after the durable reservation and immediately before PUB.
	// If a consumer wins this final race, its existing generation CAS and
	// attempt unique index govern the original authorization at consumption.
	current, err := captureOriginal(ctx, client.Database(dbName), cfg.runID, cfg.orgID, time.Now())
	if err != nil || current.SourceFingerprint != plan.SourceFingerprint {
		_, _ = fmt.Fprintln(stderr, "original retry recovery: source changed; reserved operation requires reconciliation")
		return 2
	}
	result := publisher.Publish(ctx, plan.message)
	outcome := "unknown"
	if result.Outcome == transport.Confirmed {
		outcome = "confirmed"
	} else if result.Outcome == transport.Rejected {
		outcome = "rejected"
	}
	receipt := operationReceipt{RequestID: cfg.requestID, EventID: plan.EventID, TransportOutcome: outcome, RecordedAt: time.Now().In(time.FixedZone("UTC+8", 8*3600))}
	if err := persistExclusive(cfg.auditDir, cfg.requestID+".receipt.json", receipt); err != nil {
		_, _ = fmt.Fprintln(stderr, "original retry recovery: receipt persistence failed; result uncertain, do not republish")
		return 2
	}
	if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
		return 2
	}
	if outcome != "confirmed" {
		return 2
	}
	return 0
}

func newRecoveryProducer(address string) (*driver.Producer, error) {
	cfg := driver.NewConfig()
	cfg.DialTimeout = 2 * time.Second
	// go-nsq validates the shared config even for producers. Its default
	// heartbeat is 30s and would invalidate the bounded 3s read timeout.
	cfg.HeartbeatInterval = time.Second
	cfg.ReadTimeout = 3 * time.Second
	cfg.WriteTimeout = 3 * time.Second
	return driver.NewProducer(address, cfg)
}
