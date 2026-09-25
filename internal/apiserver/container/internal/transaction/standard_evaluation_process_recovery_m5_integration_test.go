//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m5

package transaction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	appexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	evalworker "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/worker"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/nsqio/go-nsq"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type m5ProcessDelivery struct {
	EventID  string `json:"event_id"`
	BrokerID string `json:"broker_id"`
	Attempts uint16 `json:"attempts"`
}

// A separate Worker OS process dies after the original evaluation.requested
// handler commits a terminal Assessment/Run, but before it can FIN the NSQ
// message. A replacement process must receive that same broker message and
// preserve the one business attempt.
func TestM5EvaluationRequestedRecoversAfterWorkerProcessKill(t *testing.T) {
	dsn := os.Getenv("RM_QS_M5_PROCESS_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m5_qs_process" ||
		os.Getenv("RM_QS_M5_NSQ_TCP") != "nsqd:4150" {
		t.Fatal("disposable m5_qs_process MySQL and nsqd:4150 required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 75*time.Second)
	defer cancel()
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&assessmentmysql.AssessmentPO{}, &checkpoint.RuntimeCheckpointPO{}))
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)

	parsedCatalog, err := eventcatalog.Parse([]byte(`version: "1"
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
  evaluation.failed:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Evaluation
    domain: evaluation
    handler: evaluation_failed_handler
`))
	require.NoError(t, err)
	stager, err := mysqlstandard.NewStager(eventcatalog.NewCatalog(parsedCatalog), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	assessmentRepo := assessmentmysql.NewAssessmentRepository(db)
	runRepo := checkpoint.NewRunRepository(db)
	intake := appintake.NewService(assessmentRepo, proofModelValidator{}, NewMySQLRunner(db), stager)
	engine := appexecute.NewEngine(assessmentRepo, m5TerminalInput{},
		appexecute.WithRunRepository(runRepo), appexecute.WithTransactionalOutbox(NewMySQLRunner(db), stager))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	grpcservice.NewEvaluationWorkerService(evalworker.NewService(
		engine, assessmentRepo, assessmentmysql.NewOutcomeRepository(db), runRepo,
	)).RegisterService(server)
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		if err := <-serverDone; err != nil {
			t.Errorf("stop Evaluation gRPC server: %v", err)
		}
	})

	config := nsq.NewConfig()
	config.HeartbeatInterval, config.MsgTimeout = time.Second, 5*time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	producer, err := nsq.NewProducer("nsqd:4150", config)
	require.NoError(t, err)
	producer.SetLogger(nil, nsq.LogLevelError)
	require.NoError(t, producer.Ping())
	publisher, err := sdknsq.New(producer, map[string]string{"qs.evaluation.lifecycle": "qs.evaluation.lifecycle"}, 1)
	require.NoError(t, err)
	t.Cleanup(func() {
		drainCtx, finish := context.WithTimeout(context.Background(), 5*time.Second)
		defer finish()
		if err := publisher.Drain(drainCtx); err != nil {
			t.Errorf("drain NSQ publisher: %v", err)
		}
		producer.Stop()
	})
	store, err := sdkmysql.New(sqlDB)
	require.NoError(t, err)
	r, err := relay.New(store, publisher, relay.Config{
		Concurrency: 1, PollInterval: 30 * time.Millisecond, Lease: 5 * time.Second,
		PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
		Retry: standardoutbox.SDKRetryPolicy(), Observe: func(relay.Event) {},
	})
	require.NoError(t, err)
	relayCtx, stopRelay := context.WithCancel(ctx)
	relayDone := make(chan error, 1)
	go func() { relayDone <- r.Run(relayCtx) }()
	t.Cleanup(func() {
		stopRelay()
		if err := <-relayDone; err != nil {
			t.Errorf("standard MySQL Relay: %v", err)
		}
	})

	const topic, channel = "qs.evaluation.lifecycle", "rm-m5-process-recovery"
	markers := t.TempDir()
	readyPath := filepath.Join(markers, "ready")
	firstPath := filepath.Join(markers, "before-fin")
	secondPath := filepath.Join(markers, "after-restart")
	childEnv := append(os.Environ(),
		"RM_QS_M5_PROCESS_TOPIC="+topic, "RM_QS_M5_PROCESS_CHANNEL="+channel,
		"RM_QS_M5_PROCESS_GRPC="+listener.Addr().String(),
		"RM_QS_M5_PROCESS_READY="+readyPath,
	)
	crash := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestM5EvaluationRequestedProcessChild$", "-test.v")
	crash.Env = append(childEnv, "RM_QS_M5_PROCESS_MODE=crash", "RM_QS_M5_PROCESS_RESULT="+firstPath)
	var crashOutput bytes.Buffer
	crash.Stdout, crash.Stderr = &crashOutput, &crashOutput
	require.NoError(t, crash.Start())
	crashDone := make(chan error, 1)
	go func() { crashDone <- crash.Wait() }()
	t.Cleanup(func() { _ = crash.Process.Kill() })
	waitM5ProcessFile(t, readyPath, crashDone)

	kind, code, version := "scale", "MODEL-1", "1.0.0"
	created, err := intake.CreateForAnswerSheet(ctx, appintake.CreateCommand{
		OrgID: 1, TesteeID: 2, AnswerSheetID: 705,
		QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", OriginType: "adhoc",
		ModelKind: &kind, ModelCode: &code, ModelVersion: &version,
	})
	require.NoError(t, err)
	_, err = intake.SubmitForEvaluation(ctx, created.ID)
	require.NoError(t, err)
	var intent struct{ MessageID, State string }
	require.NoError(t, db.Table("rm_outbox").Select("message_id,state").
		Where("event_type = ?", eventcatalog.EvaluationRequested).Take(&intent).Error)
	waitM5ProcessFile(t, firstPath, crashDone)
	first := readM5ProcessDelivery(t, firstPath)
	require.Equal(t, intent.MessageID, first.EventID)
	require.EqualValues(t, 1, first.Attempts)
	failed, err := assessmentRepo.FindByID(ctx, meta.FromUint64(created.ID))
	require.NoError(t, err)
	require.True(t, failed.Status().IsFailed())
	run, err := runRepo.FindLatestByAssessmentID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, 1, run.Attempt().Number)
	require.Equal(t, retrygovernance.DispositionTerminal, run.RetryDecision().Disposition)
	require.Empty(t, run.RetryDecision().RetryEventID)
	var count int64
	require.NoError(t, db.Table("runtime_checkpoint").Where("scope = ? AND assessment_id = ?", "evaluation_run", created.ID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Table("rm_outbox").Where("event_type = ?", eventcatalog.EvaluationFailed).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Table("rm_outbox").Where("event_type = ?", eventcatalog.EvaluationRetryRequested).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, crash.Process.Kill())
	require.Error(t, <-crashDone, "first Worker OS process must die before FIN")

	recovery := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestM5EvaluationRequestedProcessChild$", "-test.v")
	recovery.Env = append(childEnv, "RM_QS_M5_PROCESS_MODE=recover", "RM_QS_M5_PROCESS_RESULT="+secondPath)
	output, err := recovery.CombinedOutput()
	require.NoError(t, err, "replacement Worker process failed: %s; first: %s", output, crashOutput.String())
	second := readM5ProcessDelivery(t, secondPath)
	require.Equal(t, first.EventID, second.EventID)
	require.Equal(t, first.BrokerID, second.BrokerID)
	require.Greater(t, second.Attempts, first.Attempts)
	require.NoError(t, db.Table("runtime_checkpoint").Where("scope = ? AND assessment_id = ?", "evaluation_run", created.ID).Count(&count).Error)
	require.EqualValues(t, 1, count, "process replacement must not create another Evaluation Run")
	require.NoError(t, db.Table("rm_outbox").Where("event_type = ?", eventcatalog.EvaluationFailed).Count(&count).Error)
	require.EqualValues(t, 1, count, "process replacement must not create another failure event")
	require.NoError(t, db.Table("rm_outbox").Where("event_type = ?", eventcatalog.EvaluationRetryRequested).Count(&count).Error)
	require.Zero(t, count)
	require.Eventually(t, func() bool {
		var state string
		return db.Table("rm_outbox").Select("state").Where("message_id = ?", intent.MessageID).Scan(&state).Error == nil && state == "published"
	}, 5*time.Second, 25*time.Millisecond)
	waitM5ProcessNSQDrain(t, topic, channel)
	t.Logf("recovered original Evaluation message: event_id=%s broker_id=%s attempts=%d/%d, one terminal Run", second.EventID, second.BrokerID, first.Attempts, second.Attempts)
}

func TestM5EvaluationRequestedProcessChild(t *testing.T) {
	mode := os.Getenv("RM_QS_M5_PROCESS_MODE")
	if mode == "" {
		t.Skip("invoked only by the disposable Worker process test")
	}
	if mode != "crash" && mode != "recover" ||
		os.Getenv("RM_QS_M5_NSQ_TCP") != "nsqd:4150" ||
		os.Getenv("RM_QS_M5_PROCESS_TOPIC") != "qs.evaluation.lifecycle" ||
		os.Getenv("RM_QS_M5_PROCESS_CHANNEL") != "rm-m5-process-recovery" ||
		!strings.HasPrefix(os.Getenv("RM_QS_M5_PROCESS_GRPC"), "127.0.0.1:") ||
		!strings.HasPrefix(os.Getenv("RM_QS_M5_PROCESS_READY"), "/tmp/") ||
		!strings.HasPrefix(os.Getenv("RM_QS_M5_PROCESS_RESULT"), "/tmp/") {
		t.Fatal("Worker child requires the invocation-owned disposable endpoints")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	connection, err := grpc.NewClient(os.Getenv("RM_QS_M5_PROCESS_GRPC"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer connection.Close()
	deps := &handlers.Dependencies{
		Logger:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
		EvaluationWorkerClient: m5EvaluationGRPCClient{client: evalpb.NewEvaluationWorkerServiceClient(connection)},
	}
	registry := handlers.NewRegistry()
	requested, ok := registry.Create("evaluation_requested_handler", deps)
	require.True(t, ok)
	failed, ok := registry.Create("evaluation_failed_handler", deps)
	require.True(t, ok)
	config := nsq.NewConfig()
	config.HeartbeatInterval, config.MsgTimeout = time.Second, 5*time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	consumer, err := nsq.NewConsumer(os.Getenv("RM_QS_M5_PROCESS_TOPIC"), os.Getenv("RM_QS_M5_PROCESS_CHANNEL"), config)
	require.NoError(t, err)
	consumer.SetLogger(nil, nsq.LogLevelError)
	consumer.AddHandler(nsq.HandlerFunc(func(raw *nsq.Message) error {
		decoded, recognized, decodeErr := messaging.DecodeMessagePayload(raw.Body)
		if decodeErr != nil || !recognized {
			return fmt.Errorf("decode original QS envelope: recognized=%t: %w", recognized, decodeErr)
		}
		eventType := decoded.Metadata["event_type"]
		switch eventType {
		case eventcatalog.EvaluationRequested:
			if err := requested(ctx, eventType, decoded.Payload); err != nil {
				return err
			}
			record, err := json.Marshal(m5ProcessDelivery{EventID: decoded.UUID, BrokerID: fmt.Sprintf("%s", raw.ID), Attempts: raw.Attempts})
			if err != nil {
				return err
			}
			if err := os.WriteFile(os.Getenv("RM_QS_M5_PROCESS_RESULT"), record, 0o600); err != nil {
				return err
			}
			if mode == "crash" {
				select {} // Parent kills this OS process before the original message can FIN.
			}
			return nil
		case eventcatalog.EvaluationFailed:
			return failed(ctx, eventType, decoded.Payload)
		default:
			return fmt.Errorf("unexpected event %q", eventType)
		}
	}))
	require.NoError(t, consumer.ConnectToNSQD("nsqd:4150"))
	defer consumer.Stop()
	require.NoError(t, os.WriteFile(os.Getenv("RM_QS_M5_PROCESS_READY"), []byte(mode), 0o600))
	if mode == "crash" {
		select {}
	}
	require.Eventually(t, func() bool { return consumer.Stats().MessagesFinished >= 2 }, 25*time.Second, 25*time.Millisecond,
		"replacement must FIN both the original request and its failed event")
	consumer.Stop()
	select {
	case <-consumer.StopChan:
	case <-time.After(5 * time.Second):
		t.Fatal("replacement Worker consumer did not stop")
	}
}

func waitM5ProcessFile(t *testing.T, path string, childDone <-chan error) {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case err := <-childDone:
			t.Fatalf("Worker process exited before %s: %v", filepath.Base(path), err)
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("Worker process did not reach %s", filepath.Base(path))
		}
	}
}

func readM5ProcessDelivery(t *testing.T, path string) m5ProcessDelivery {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var record m5ProcessDelivery
	require.NoError(t, json.Unmarshal(data, &record))
	require.NotEmpty(t, record.EventID)
	require.NotEmpty(t, record.BrokerID)
	return record
}

// Keep the channel empty for longer than the five-second NSQ message timeout;
// a missing FIN would otherwise briefly look drained while still in flight.
func waitM5ProcessNSQDrain(t *testing.T, topic, channel string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(18 * time.Second)
	var emptySince time.Time
	for time.Now().Before(deadline) {
		response, err := client.Get("http://nsqd:4151/stats?format=json")
		require.NoError(t, err)
		var stats struct {
			Topics []struct {
				Name     string `json:"topic_name"`
				Channels []struct {
					Name     string `json:"channel_name"`
					Depth    int64  `json:"depth"`
					InFlight int64  `json:"in_flight_count"`
					Deferred int64  `json:"deferred_count"`
				} `json:"channels"`
			} `json:"topics"`
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&stats)
		_ = response.Body.Close()
		require.NoError(t, decodeErr)
		found, empty := false, false
		for _, item := range stats.Topics {
			if item.Name != topic {
				continue
			}
			for _, ch := range item.Channels {
				if ch.Name == channel {
					found, empty = true, ch.Depth == 0 && ch.InFlight == 0 && ch.Deferred == 0
				}
			}
		}
		require.True(t, found, "NSQ process-recovery channel missing")
		if empty {
			if emptySince.IsZero() {
				emptySince = time.Now()
			} else if time.Since(emptySince) > 6*time.Second {
				return
			}
		} else {
			emptySince = time.Time{}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("NSQ process-recovery channel did not remain drained")
}
