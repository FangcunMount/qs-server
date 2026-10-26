//go:build integration && m3_worker_runtime

package process

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	boundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	txport "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	generic "github.com/FangcunMount/qs-server/internal/pkg/options"
	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
	"github.com/FangcunMount/qs-server/internal/worker/container"
	grpcclient "github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	workeroptions "github.com/FangcunMount/qs-server/internal/worker/options"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	driver "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This is the real worker Container + initializeRuntime + lifecycle, with a
// loopback gRPC business fixture. It is not the complete apiserver or a provider.
type nativeWorkerRPC struct {
	evalpb.UnimplementedEvaluationWorkerServiceServer
	mu              sync.Mutex
	active, maximum int
	calls           map[uint64]int
	entered         chan uint64
	release         map[uint64]chan struct{}
	headers         map[uint64]metadata.MD
}

func (s *nativeWorkerRPC) ExecuteEvaluation(ctx context.Context, req *evalpb.ExecuteEvaluationRequest) (*evalpb.ExecuteEvaluationResponse, error) {
	id := req.GetAssessmentId()
	s.mu.Lock()
	s.active++
	s.maximum = max(s.maximum, s.active)
	s.calls[id]++
	s.headers[id], _ = metadata.FromIncomingContext(ctx)
	release := s.release[id]
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.active--; s.mu.Unlock() }()
	select {
	case s.entered <- id:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-release:
		return &evalpb.ExecuteEvaluationResponse{Status: "evaluated"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func nativeWorkerWait(t *testing.T, condition func() bool) {
	t.Helper()
	require.Eventually(t, condition, 60*time.Second, 25*time.Millisecond, "owned worker runtime condition did not converge")
}
func TestM3WorkerRocketMQRuntime(t *testing.T) {
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	cfg, err := driver.ParseDSN(os.Getenv("QS_M3_WORKER_DSN"))
	require.NoError(t, err)
	require.Equal(t, "tcp", cfg.Net)
	require.Empty(t, cfg.DBName)
	host, _, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback())
	cfg.ParseTime, cfg.MultiStatements, cfg.Loc = true, true, time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	name := fmt.Sprintf("qs_m3_worker_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`")
	require.NoError(t, err)
	var dropOnce sync.Once
	drop := func() {
		dropOnce.Do(func() {
			_, e := admin.ExecContext(context.Background(), "DROP DATABASE `"+name+"`")
			require.NoError(t, e)
		})
	}
	t.Cleanup(drop)
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(4)
	var poolOnce sync.Once
	closePool := func() { poolOnce.Do(func() { require.NoError(t, pool.Close()) }) }
	t.Cleanup(closePool)
	install := func(file string) {
		raw, e := os.ReadFile(filepath.Join("..", "..", "pkg", "migration", "migrations", "mysql", file))
		require.NoError(t, e)
		_, e = pool.ExecContext(t.Context(), string(raw))
		require.NoError(t, e)
	}
	for _, file := range []string{"000050_add_retry_event_hold.up.sql", "000104_rocketmq_transport_evidence.up.sql", "000106_transaction_messaging.up.sql", "000110_native_transaction_limit_notices.up.sql"} {
		install(file)
	}
	load := func(key string) *generic.RocketMQOptions {
		raw, e := os.ReadFile(os.Getenv(key))
		require.NoError(t, e)
		var o generic.RocketMQOptions
		require.NoError(t, json.Unmarshal(raw, &o))
		h, _, e := net.SplitHostPort(o.Endpoint)
		require.NoError(t, e)
		require.True(t, net.ParseIP(h).IsLoopback())
		return &o
	}
	workerMQ, producerMQ := load("QS_M3_WORKER_CONFIG"), load("QS_M3_WORKER_PRODUCER_CONFIG")
	require.False(t, workerMQ.PublishingEnabled())
	require.Empty(t, workerMQ.Transactions, "consumer does not own a producer or Checker")
	source, err := catalog.Load("../../../configs/events.yaml")
	require.NoError(t, err)
	routes := catalog.NewCatalog(source)
	binding := producerMQ.Transactions["qs.mysql"]
	_, err = pool.ExecContext(t.Context(), `INSERT INTO rm_tx_binding VALUES(?,?,?,1)`, []byte("qs.mysql"), []byte(binding.StoreID), []byte(binding.WriterEpoch))
	require.NoError(t, err)
	owner, err := boundary.New(db, &generic.MessagingOptions{Provider: "rocketmq", Enabled: true, RocketMQ: producerMQ}, routes, []string{"qs.evaluation.lifecycle"})
	require.NoError(t, err)
	require.NoError(t, owner.Runtime().Start(t.Context()))
	var ownerOnce sync.Once
	closeOwner := func() {
		ownerOnce.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			require.NoError(t, owner.Runtime().Close(ctx))
		})
	}
	t.Cleanup(closeOwner)
	rpc := &nativeWorkerRPC{calls: map[uint64]int{}, entered: make(chan uint64, 8), release: map[uint64]chan struct{}{1: make(chan struct{}), 2: make(chan struct{})}, headers: map[uint64]metadata.MD{}}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcServer := grpc.NewServer()
	evalpb.RegisterEvaluationWorkerServiceServer(grpcServer, rpc)
	serveDone := make(chan error, 1)
	go func() { serveDone <- grpcServer.Serve(listener) }()
	t.Cleanup(func() { grpcServer.Stop(); require.NoError(t, <-serveDone) })
	// Always unblock owned fixture RPCs before drain cleanup, even on assertion failure.
	var release1, release2 sync.Once
	unblock1 := func() { release1.Do(func() { close(rpc.release[1]) }) }
	unblock2 := func() { release2.Do(func() { close(rpc.release[2]) }) }
	type instance struct {
		runtime   runtimeOutput
		shutdown  *workerShutdown
		manager   *grpcclient.Manager
		container *container.Container
	}
	start := func(automatic bool) (*instance, error) {
		o := workeroptions.NewOptions()
		o.Metrics.Enable = false
		o.Worker.Concurrency = 1
		o.Worker.ServiceName = "qs-worker"
		o.RetryGovernance.AutomaticRetryEnabled = automatic
		o.Messaging.Provider = "rocketmq"
		o.Messaging.RocketMQ = workerMQ
		o.Messaging.NSQMessageTimeout = 3 * time.Minute
		o.MySQL.Host = cfg.Addr
		o.MySQL.Username = cfg.User
		o.MySQL.Password = cfg.Passwd
		o.MySQL.Database = name
		o.MySQL.Location = "UTC"
		o.MySQL.SessionTimeZone = "+00:00"
		manager, e := grpcclient.NewManager(&grpcclient.ManagerConfig{Endpoint: listener.Addr().String(), Insecure: true, Timeout: 3 * time.Minute})
		require.NoError(t, e)
		require.NoError(t, manager.RegisterClients())
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		c, e := container.NewContainer(o, log, nil, nil, routes)
		require.NoError(t, e)
		c.InitializeRuntimeClients(container.ClientBundle{AnswerSheet: manager.AnswerSheetClient(), Internal: manager.InternalClient(), AssessmentIntake: manager.AssessmentIntakeClient(), EvaluationWorker: manager.EvaluationWorkerClient(), InterpretationAutomation: manager.InterpretationAutomationClient()})
		require.NoError(t, c.Initialize())
		s := &server{config: &workerconfig.Config{Options: o}, logger: log}
		co := containerOutput{container: c}
		runtime, e := s.initializeRuntime(resourceOutput{}, co)
		shutdown := newWorkerShutdown(buildLifecycleDeps(resourceOutput{}, co, integrationOutput{grpcClients: grpcClientsOutput{grpcManager: manager}}, runtime))
		t.Cleanup(func() {
			unblock1()
			unblock2()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			require.NoError(t, shutdown.Close(ctx))
		})
		return &instance{runtime: runtime, shutdown: shutdown, manager: manager, container: c}, e
	}
	cases := map[string]bool{}
	missing, err := start(false)
	require.Error(t, err, "missing explicit SQL109 must fail before subscription")
	require.NoError(t, missing.shutdown.Close(t.Context()))
	require.Nil(t, missing.runtime.messaging.publisher)
	require.Nil(t, missing.runtime.messaging.holdReplayer)
	install("000109_rocketmq_deferred_delivery.up.sql")
	cases["explicit_schema_before_subscription"] = true
	paused, err := start(false)
	require.NoError(t, err)
	require.Nil(t, paused.runtime.messaging.publisher)
	require.Nil(t, paused.runtime.messaging.holdReplayer)
	send := func(id int64) transaction.Execution {
		e := event.New(catalog.EvaluationRetryRequested, "Assessment", fmt.Sprint(id), eventpayload.EvaluationRequestedData{OrgID: 7, AssessmentID: id, ExpectedAttempt: 2, AttemptOrigin: "automatic", ModelCode: "fixture", RequestedAt: time.Now().UTC()})
		r, er := owner.Reserve(t.Context(), txport.Request{Key: fmt.Sprintf("worker-fixture-%d", id), InputHash: sha256.Sum256([]byte(fmt.Sprint(id))), Artifact: []byte("deterministic-transport-fixture"), Events: []event.DomainEvent{e}})
		require.NoError(t, er)
		result, er := owner.Commit(t.Context(), r.Execution, func(context.Context) error { return nil })
		require.NoError(t, er)
		require.Equal(t, transaction.LocalCommitted, result.Local)
		for _, branch := range result.Branches {
			require.Equal(t, transaction.HalfPrepared, branch.Half)
			require.Equal(t, transaction.FinalCommitConfirmed, branch.Final, "healthy transport fixture must confirm its seed, independently of business success")
		}
		return r.Execution
	}
	observed, phase := paused, "paused_delivery"
	// Capture live failure facts before t.Cleanup drains the replacement runtime.
	defer func() {
		if !t.Failed() {
			return
		}
		stats := observed.container.DispatchBudgetSnapshot()
		counters := map[string]float64{}
		families, gatherErr := prometheus.DefaultGatherer.Gather()
		if gatherErr == nil {
			for _, family := range families {
				if family.GetName() != "qs_event_consume_total" {
					continue
				}
				for _, metric := range family.Metric {
					labels := map[string]string{}
					for _, label := range metric.Label {
						labels[label.GetName()] = label.GetValue()
					}
					if labels["service"] == "qs-worker" && labels["topic"] == "qs.evaluation.lifecycle" {
						switch labels["outcome"] {
						case "held", "hold_failed", "dispatch_failed", "decode_failed", "acked", "unknown_acked", "unknown_persist_failed":
							counters[labels["outcome"]] += metric.GetCounter().GetValue()
						}
					}
				}
			}
		}
		var holds, evidence int
		diagnosticContext, cancelDiagnostic := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelDiagnostic()
		holdErr := pool.QueryRowContext(diagnosticContext, "SELECT COUNT(*) FROM retry_event_hold").Scan(&holds)
		evidenceErr := pool.QueryRowContext(diagnosticContext, "SELECT COUNT(*) FROM event_delivery_transport_evidence").Scan(&evidence)
		stackBytes := make([]byte, 1<<20)
		stackSize := runtime.Stack(stackBytes, true)
		sites := map[string]int{}
		for _, frame := range regexp.MustCompile(`transport/rocketmq/(?:internal/driver/)?(subscriber|simple_consumer)\.go:([0-9]+)`).FindAllSubmatch(stackBytes[:stackSize], 128) {
			key := string(frame[1]) + ":" + string(frame[2])
			if sites[key] < 32 {
				sites[key]++
			}
		}
		raw, _ := json.Marshal(map[string]any{"sdk_stack_sites": sites, "sdk_stack_truncated": stackSize == len(stackBytes), "phase": phase, "active": stats.Active, "waiting": stats.Waiting, "holds": holds, "evidence": evidence, "query_failed": holdErr != nil || evidenceErr != nil, "consume_outcomes": counters, "transport_errors": func() map[string]int {
			result := map[string]int{}
			sub, ok := observed.runtime.messaging.subscriber.(*eventtransport.SDKDeliverySubscriber)
			if !ok {
				return result
			}
			for i := 0; i < 8; i++ {
				select {
				case err := <-sub.RocketMQErrors():
					if err == nil {
						return result
					}
					label := "other"
					raw := strings.ToLower(err.Error())
					switch {
					case strings.Contains(raw, "40100"):
						label = "auth_or_acl"
					case strings.Contains(raw, "deadline"):
						label = "deadline"
					case strings.Contains(raw, "tls") || strings.Contains(raw, "x509"):
						label = "tls"
					case strings.Contains(raw, "unavailable") || strings.Contains(raw, "connection"):
						label = "connection"
					}
					result[label]++
				default:
					return result
				}
			}
			return result
		}()})
		fmt.Println("M3_WORKER_DIAGNOSTIC " + string(raw))
	}()
	// New groups must complete real receives before the fixture publishes. This
	// barrier has no message side effects and does not weaken the hold assertion.
	pausedSubscriber, ok := paused.runtime.messaging.subscriber.(*eventtransport.SDKDeliverySubscriber)
	require.True(t, ok)
	require.Eventually(t, func() bool { return pausedSubscriber.RocketMQReady(time.Minute) }, 45*time.Second, 50*time.Millisecond)
	first := send(1)
	pausedObservationStarted := time.Now()
	envelope, recognized, decodeErr := legacy.Decode(first.Plan.Input().Branches[0].Wire)
	require.NoError(t, decodeErr)
	require.True(t, recognized)
	require.Equal(t, first.References()[0].MessageID, envelope.UUID)
	_, pauseErr := paused.container.DispatchEvent(t.Context(), envelope.Metadata["event_type"], envelope.Payload)
	require.True(t, errors.Is(pauseErr, eventruntime.ErrAutomaticRetryPaused), "frozen input must retain its original automatic authorization")
	nativeWorkerWait(t, func() bool {
		var n int
		return pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM retry_event_hold WHERE event_id=? AND protocol_version=2 AND status='blocked'", first.References()[0].MessageID).Scan(&n) == nil && n == 1
	})
	pausedObservationMS := time.Since(pausedObservationStarted).Milliseconds()
	var wire []byte
	var physical string
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT original_wire,transport_message_id FROM event_delivery_transport_evidence WHERE application_id=?", first.References()[0].MessageID).Scan(&wire, &physical))
	require.Equal(t, first.Plan.Input().Branches[0].Wire, wire)
	require.NotEmpty(t, physical)
	rpc.mu.Lock()
	pausedCalls := maps.Clone(rpc.calls)
	rpc.mu.Unlock()
	require.Empty(t, pausedCalls)
	require.NoError(t, paused.shutdown.Close(t.Context()))
	cases["actual_broker_paused_delivery_preserves_first_wire"] = true
	active, err := start(true)
	require.NoError(t, err)
	require.Nil(t, active.runtime.messaging.publisher)
	require.Nil(t, active.runtime.messaging.holdReplayer)
	observed, phase = active, "active_delivery"
	// A resumed deferred callback can enter gRPC before the replacement
	// consumer completes its first receives. Await the same real transport
	// readiness used for the paused instance before publishing the second seed.
	// Keep all routes, the shared limit of one and the 60-second assertion.
	activeSubscriber, ok := active.runtime.messaging.subscriber.(*eventtransport.SDKDeliverySubscriber)
	require.True(t, ok)
	require.Eventually(t, func() bool { return activeSubscriber.RocketMQReady(time.Minute) }, 45*time.Second, 50*time.Millisecond)
	select {
	case id := <-rpc.entered:
		require.EqualValues(t, 1, id)
	case <-time.After(60 * time.Second):
		t.Fatal("held delivery did not resume through original gRPC handler")
	}
	second := send(2)
	nativeWorkerWait(t, func() bool {
		stats := active.container.DispatchBudgetSnapshot()
		return stats.Limit == 1 && stats.Active == 1 && stats.Waiting == 1
	})
	select {
	case <-rpc.entered:
		t.Fatal("physical input exceeded original shared execution budget")
	default:
	}
	unblock1()
	select {
	case id := <-rpc.entered:
		require.EqualValues(t, 2, id)
	case <-time.After(60 * time.Second):
		t.Fatal("committed physical delivery did not follow held callback")
	}
	nativeWorkerWait(t, func() bool {
		var n int
		return pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM retry_event_hold WHERE event_id=? AND status='consumed' AND replay_attempt_count=1 AND replayed_at IS NULL", first.References()[0].MessageID).Scan(&n) == nil && n == 1
	})
	rpc.mu.Lock()
	maximum, calls, headers := rpc.maximum, maps.Clone(rpc.calls), maps.Clone(rpc.headers)
	rpc.mu.Unlock()
	require.Equal(t, 1, maximum)
	require.Equal(t, 1, calls[1])
	require.Equal(t, 1, calls[2])
	require.Equal(t, []string{first.References()[0].MessageID}, headers[1].Get("x-retry-event-id"))
	require.Equal(t, []string{"2"}, headers[1].Get("x-retry-expected-attempt"))
	require.Equal(t, []string{second.References()[0].MessageID}, headers[2].Get("x-retry-event-id"))
	cases["original_dispatcher_and_shared_execution_budget"] = true
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = active.shutdown.Close(ctx)
	cancel()
	require.ErrorIs(t, err, transaction.ErrShutdownIncomplete)
	require.NotEqual(t, connectivity.Shutdown, active.manager.Conn().GetState(), "active business RPC lost its dependency before real drain")
	unblock2()
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	require.NoError(t, active.shutdown.Wait(ctx))
	cancel()
	require.Equal(t, connectivity.Shutdown, active.manager.Conn().GetState())
	cases["real_broker_handler_drain_retains_grpc_until_return"] = true
	closeOwner()
	closePool()
	drop()
	sourceSHA, err := exec.Command("git", "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	raw, err := json.Marshal(map[string]any{"source_sha": strings.TrimSpace(string(sourceSHA)), "passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "scope": "actual worker container/runtime/Broker/MySQL with loopback business RPC fixture", "complete_apiserver_not_claimed": true, "provider_not_called": true, "system_login_not_claimed": true, "owned_database_dropped": true, "owned_runtime_drained": true, "paused_observation_ms": pausedObservationMS, "fixture_shared_limit": 1, "fixture_observation_budget_ms": 60000})
	require.NoError(t, err)
	fmt.Println("M3_QS_WORKER_RUNTIME_RECEIPT " + string(raw))
}
