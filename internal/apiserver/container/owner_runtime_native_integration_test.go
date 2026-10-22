//go:build integration && m3_host_runtime

package container

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	generic "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/message"
	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type nativeOwnerData struct {
	sql       *gorm.DB
	pool      *sql.DB
	documents *mongo.Database
	close     func()
}

func ownedDefaultOwnerData(t *testing.T) nativeOwnerData {
	t.Helper()
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	cfg, err := driver.ParseDSN(os.Getenv("QS_M3_HOST_DSN"))
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
	name := fmt.Sprintf("qs_m3_host_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`")
	require.NoError(t, err)
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(4)
	pool.SetMaxIdleConns(2)
	uri := os.Getenv("QS_M3_HOST_MONGO_URI")
	parsed, err := url.Parse(uri)
	require.NoError(t, err)
	require.Equal(t, "mongodb", parsed.Scheme)
	require.Nil(t, parsed.User)
	require.True(t, net.ParseIP(parsed.Hostname()).IsLoopback())
	require.Equal(t, "sdk-proof", parsed.Query().Get("replicaSet"))
	require.Equal(t, "true", parsed.Query().Get("directConnection"))
	client, err := mongo.Connect(t.Context(), mongooptions.Client().ApplyURI(uri).SetMaxPoolSize(4).SetServerSelectionTimeout(2*time.Second))
	require.NoError(t, err)
	require.NoError(t, client.Ping(t.Context(), readpref.Primary()))
	documents := client.Database(name)
	var once sync.Once
	closeData := func() {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			require.NoError(t, pool.Close())
			require.NoError(t, documents.Drop(ctx))
			_, e := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`")
			require.NoError(t, e)
			require.NoError(t, client.Disconnect(ctx))
			require.NoError(t, admin.Close())
		})
	}
	t.Cleanup(closeData)
	return nativeOwnerData{db, pool, documents, closeData}
}

func ownedHostBroker(t *testing.T, action string) string {
	t.Helper()
	project, broker := os.Getenv("QS_M3_FIXTURE_PROJECT"), os.Getenv("QS_M3_HOST_BROKER")
	require.True(t, strings.HasPrefix(project, "rocketmq-m2-"))
	require.True(t, strings.HasPrefix(broker, project+"-"))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	label, err := exec.CommandContext(ctx, "docker", "inspect", "--format", `{{index .Config.Labels "com.docker.compose.project"}}`, broker).Output()
	require.NoError(t, err)
	require.Equal(t, project, strings.TrimSpace(string(label)))
	switch action {
	case "id", "pid", "exit", "restart-policy":
		field := map[string]string{"id": "{{.Id}}", "pid": "{{.State.Pid}}", "exit": "{{.State.ExitCode}}", "restart-policy": "{{.HostConfig.RestartPolicy.Name}}"}[action]
		out, e := exec.CommandContext(ctx, "docker", "inspect", "--format", field, broker).Output()
		require.NoError(t, e)
		return strings.TrimSpace(string(out))
	case "kill", "start":
		_, err = exec.CommandContext(ctx, "docker", action, broker).Output()
		require.NoError(t, err)
	default:
		t.Fatal("unsupported owned Broker action")
	}
	return ""
}

func waitOwnedHostTLS(t *testing.T, o *generic.RocketMQOptions) {
	t.Helper()
	body, err := os.ReadFile(o.CAFile)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(body))
	host, _, err := net.SplitHostPort(o.Endpoint)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback())
	require.Eventually(t, func() bool {
		conn, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", o.Endpoint, &tls.Config{RootCAs: roots, ServerName: host, MinVersion: tls.VersionTLS12, NextProtos: []string{"h2"}})
		if e != nil {
			return false
		}
		return conn.Close() == nil
	}, 45*time.Second, 250*time.Millisecond, "restarted owned verified TLS endpoint unavailable")
}

// Real default subsystem and both original stores. The frozen wire and fact
// rows below are protocol fixtures, not domain/API/provider/system acceptance.
func TestM3DefaultOwnerRuntime(t *testing.T) {
	data := ownedDefaultOwnerData(t)
	load := func(key string) generic.MessagingOptions {
		t.Helper()
		body, err := os.ReadFile(os.Getenv(key))
		require.NoError(t, err)
		var opts generic.MessagingOptions
		require.NoError(t, json.Unmarshal(body, &opts))
		require.NotNil(t, opts.RocketMQ)
		host, _, e := net.SplitHostPort(opts.RocketMQ.Endpoint)
		require.NoError(t, e)
		require.True(t, net.ParseIP(host).IsLoopback())
		return opts
	}
	opts, observer := load("QS_M3_HOST_OPTIONS"), load("QS_M3_HOST_OBSERVER_OPTIONS")
	source, err := catalog.Load("../../../configs/events.yaml")
	require.NoError(t, err)
	routes := catalog.NewCatalog(source)
	cases := map[string]bool{}
	// Construction is inert; the fixture installs schema explicitly afterward.
	type instance struct {
		events *eventsubsystem.Subsystem
		normal *messagingruntime.NormalWirePublisher
		host   *Container
	}
	newInstance := func(paused bool) *instance {
		t.Helper()
		options := opts
		mq := *opts.RocketMQ
		admitted := !paused
		mq.PublishEnabled = &admitted
		options.RocketMQ = &mq
		normal, e := messagingruntime.NewNormalWirePublisher(&options, []string{"qs.survey.lifecycle", "qs.plan.task"}, 4)
		require.NoError(t, e)
		events, e := eventsubsystem.NewWithTransactionOwners(eventsubsystem.Options{MySQLDB: data.sql, MongoDB: data.documents, Catalog: routes, WirePublisher: normal, PublisherMode: eventruntime.PublishModeMQ}, &options)
		require.NoError(t, e)
		host := NewContainerWithOptions(data.sql, data.documents, nil, ContainerOptions{EventSubsystem: events, Transactions: &EventTransactionBindings{MySQL: events.MySQLTransactionOwner(), Mongo: events.MongoTransactionOwner(), Routes: routes}, Silent: true})
		result := &instance{events, normal, host}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			e := events.CloseContext(ctx)
			if e != nil {
				e = events.Wait(ctx)
			}
			require.NoError(t, e)
			require.NoError(t, normal.Close(ctx))
			require.NoError(t, host.Cleanup())
		})
		return result
	}
	first := newInstance(false)
	var tables int
	require.NoError(t, data.pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&tables))
	require.Zero(t, tables)
	names, err := data.documents.ListCollectionNames(t.Context(), bson.D{})
	require.NoError(t, err)
	require.Empty(t, names)
	for _, file := range []string{"000106_transaction_messaging.up.sql", "000107_business_retry_schedule.up.sql"} {
		body, e := os.ReadFile(filepath.Join("..", "..", "pkg", "migration", "migrations", "mysql", file))
		require.NoError(t, e)
		_, e = data.pool.ExecContext(t.Context(), string(body))
		require.NoError(t, e)
	}
	for _, file := range []string{"000041_transaction_messaging.up.json", "000042_business_retry_schedule.up.json"} {
		body, e := os.ReadFile(filepath.Join("..", "..", "pkg", "migration", "migrations", "mongodb", file))
		require.NoError(t, e)
		var commands []json.RawMessage
		require.NoError(t, json.Unmarshal(body, &commands))
		for _, command := range commands {
			var doc bson.D
			require.NoError(t, bson.UnmarshalExtJSON(command, false, &doc))
			require.NoError(t, data.documents.RunCommand(t.Context(), doc).Err())
		}
	}
	my, mg := first.events.MySQLTransactionOwner(), first.events.MongoTransactionOwner()
	sqlBinding, mongoBinding := my.Binding(), mg.Binding()
	_, err = data.pool.ExecContext(t.Context(), "INSERT INTO rm_tx_binding VALUES(?,?,?,1)", []byte(sqlBinding.Owner), []byte(sqlBinding.StoreID), []byte(sqlBinding.WriterEpoch))
	require.NoError(t, err)
	_, err = data.documents.Collection("rm_tx_binding").InsertOne(t.Context(), bson.M{"_id": mongoBinding.Owner, "store_id": mongoBinding.StoreID, "writer_epoch": mongoBinding.WriterEpoch, "schema_version": 1})
	require.NoError(t, err)
	_, err = data.pool.ExecContext(t.Context(), "CREATE TABLE m3_owner_runtime_facts(id VARBINARY(128) PRIMARY KEY,wire_hash BINARY(32) NOT NULL)")
	require.NoError(t, err)
	require.NoError(t, data.documents.CreateCollection(t.Context(), "m3_owner_runtime_facts"))
	cases["explicit_original_schema_no_constructor_ddl"] = true
	factory, err := first.host.BuildRetryDispatchers(true)
	require.NoError(t, err)
	require.NoError(t, factory.Validate(t.Context()))
	require.NoError(t, first.normal.Start())
	require.NoError(t, first.events.Start(t.Context()))
	for _, topic := range []string{"qs.evaluation.lifecycle", "qs.plan.task", "qs.ai.commands.v1", "qs.ai.acks.v1"} {
		require.True(t, my.HasTopic(topic))
	}
	require.True(t, mg.HasTopic("qs.evaluation.lifecycle"))
	require.True(t, my.Runtime().Stats().Ready)
	require.True(t, mg.Runtime().Stats().Ready)
	require.Same(t, my, first.host.EvaluationTransactionOwner())
	require.Same(t, mg, first.host.ReportTransactionOwner())
	cases["default_dual_owner_start_full_idle_topics"] = true
	storeSQL, err := txmysql.New(data.pool, sqlBinding)
	require.NoError(t, err)
	storeMongo, err := txmongo.New(data.documents, mongoBinding)
	require.NoError(t, err)
	type branch struct {
		owner   string
		runtime *messagingruntime.TransactionRuntime
		plan    transaction.FrozenPlan
		owned   transaction.Execution
		calls   atomic.Int32
	}
	branches := make([]*branch, 0, 2)
	for _, spec := range []struct {
		owner   string
		runtime *messagingruntime.TransactionRuntime
	}{{"qs.mysql", my.Runtime()}, {"qs.mongo", mg.Runtime()}} {
		id := fmt.Sprintf("owner-runtime-%s-%d", spec.owner, time.Now().UnixNano())
		payload := []byte("protocol-fixture:" + id)
		wire, e := legacy.Encode(legacy.Envelope{UUID: id, Payload: payload, Metadata: map[string]string{"fixture": "m3-owner-runtime"}}, legacy.Revision2)
		require.NoError(t, e)
		wire = append(append([]byte{'\n'}, wire...), '\n')
		msg, e := message.New(message.Input{Producer: "qs-server", ID: id, Destination: "qs.evaluation.lifecycle", EventType: "m3.runtime.proof", SchemaVersion: "1", Scope: "fixture", ContentType: "application/octet-stream", OccurredAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: payload})
		require.NoError(t, e)
		physical, e := transport.PhysicalTopicV2(msg.Input().Destination, transport.TransactionMessageType, spec.owner, "m3-1")
		require.NoError(t, e)
		plan, e := transaction.NewPlan(transaction.PlanInput{Owner: spec.owner, Scope: "fixture", OperationID: id, IdempotencyKey: id, InputHash: sha256.Sum256(payload), Branches: []transaction.BranchInput{{ID: "event", PhysicalTopic: physical, RouteRevision: "m3-1", Message: msg, Wire: wire}}})
		require.NoError(t, e)
		branches = append(branches, &branch{owner: spec.owner, runtime: spec.runtime, plan: plan})
	}
	cases["frozen_protocol_inputs_before_roots"] = true
	startObserver := func() *rocketmq.Subscriber {
		t.Helper()
		config, e := observer.RocketMQ.Load([]string{"qs.evaluation.lifecycle"}, 1, 5*time.Second)
		require.NoError(t, e)
		sub, e := rocketmq.NewSubscriber(rocketmq.SubscriberConfig{Config: config, MaxAttempts: 3, InvisibleDuration: 15 * time.Second, HandlerTimeout: 5 * time.Second, RetryDelay: time.Second})
		require.NoError(t, e)
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			require.NoError(t, sub.Close(ctx))
		})
		return sub
	}
	received := make(chan transport.Received, 8)
	var deliveryMu sync.Mutex
	deliveryIDs := map[string]bool{}
	var duplicateDeliveries atomic.Int32
	subscribe := func(sub *rocketmq.Subscriber) {
		t.Helper()
		require.NoError(t, sub.Subscribe(t.Context(), "qs.evaluation.lifecycle", "qs-worker", func(_ context.Context, delivery transport.Delivery) error {
			if e := delivery.Ack(); e != nil {
				return e
			}
			value := delivery.Message()
			deliveryMu.Lock()
			duplicate := deliveryIDs[value.ID]
			deliveryIDs[value.ID] = true
			deliveryMu.Unlock()
			if duplicate {
				duplicateDeliveries.Add(1)
				return nil
			}
			select {
			case received <- value:
				return nil
			default:
				return errors.New("bounded native observer full")
			}
		}, func(context.Context, transport.Failure) error {
			return errors.New("native protocol observer audit refused")
		}))
	}
	before := startObserver()
	subscribe(before)
	require.Eventually(t, func() bool { return before.Ready(10 * time.Second) }, 30*time.Second, 50*time.Millisecond)
	require.Equal(t, "no", ownedHostBroker(t, "restart-policy"))
	// Start execution leases only after observer warm-up. All IDs/wire remain
	// frozen before Reserve and before either database callback.
	for _, branch := range branches {
		input := branch.plan.Input()
		reserved, e := branch.runtime.Reserve(t.Context(), transaction.Request{Owner: branch.owner, Scope: input.Scope, Key: input.IdempotencyKey, InputHash: input.InputHash}, branch.plan)
		require.NoError(t, e)
		require.True(t, reserved.Acquired)
		branch.owned = reserved.Execution
	}
	type outcome struct {
		index int
		value transaction.Result
		err   error
	}
	entered, committed := make(chan int, 2), make(chan int, 2)
	results := make(chan outcome, 2)
	allowRoots, brokerDown := make(chan struct{}), make(chan struct{})
	var allowOnce, downOnce sync.Once
	allow := func() { allowOnce.Do(func() { close(allowRoots) }) }
	release := func() { downOnce.Do(func() { close(brokerDown) }) }
	ctx, cancelWork := context.WithCancel(t.Context())
	var group sync.WaitGroup
	var mongoCallbacks atomic.Int32
	for index, b := range branches {
		group.Add(1)
		go func(index int, b *branch) {
			defer group.Done()
			value, e := b.runtime.Execute(ctx, b.owned, func(ctx context.Context, execution transaction.Execution) (transaction.LocalOutcome, error) {
				b.calls.Add(1)
				entered <- index
				select {
				case <-allowRoots:
				case <-ctx.Done():
					return transaction.LocalUnknown, ctx.Err()
				}
				input := execution.Plan.Input().Branches[0]
				hash := sha256.Sum256(input.Wire)
				var e error
				if index == 0 {
					e = data.sql.WithContext(ctx).Transaction(func(original *gorm.DB) error {
						guard, e := storeSQL.BindGORM(ctx, original, execution)
						if e != nil {
							return e
						}
						if e = original.Exec("INSERT INTO m3_owner_runtime_facts VALUES(?,?)", []byte(input.Message.Input().ID), hash[:]).Error; e != nil {
							return e
						}
						return guard.CommitWitness(ctx)
					})
				} else {
					session, sessionErr := data.documents.Client().StartSession()
					if sessionErr != nil {
						return transaction.LocalUnknown, sessionErr
					}
					defer session.EndSession(context.Background())
					_, e = session.WithTransaction(ctx, func(original mongo.SessionContext) (any, error) {
						mongoCallbacks.Add(1)
						guard, e := storeMongo.Bind(original, execution)
						if e != nil {
							return nil, e
						}
						if _, e = data.documents.Collection("m3_owner_runtime_facts").InsertOne(original, bson.M{"_id": input.Message.Input().ID, "wire_hash": hash[:]}); e != nil {
							return nil, e
						}
						return nil, guard.CommitWitness(original)
					}, mongooptions.Transaction().SetWriteConcern(writeconcern.Majority()))
				}
				if e != nil {
					return transaction.LocalUnknown, e
				}
				// No Broker operation or non-deterministic input occurs inside either root.
				committed <- index
				select {
				case <-brokerDown:
				case <-ctx.Done():
				}
				return transaction.LocalCommitted, nil
			})
			results <- outcome{index, value, e}
		}(index, b)
	}
	joined := make(chan struct{})
	go func() { group.Wait(); close(joined) }()
	t.Cleanup(func() {
		allow()
		release()
		cancelWork()
		select {
		case <-joined:
		case <-time.After(30 * time.Second):
			t.Error("owned original SDK executions did not join")
		}
	})
	await := func(signal <-chan int) {
		t.Helper()
		for range 2 {
			select {
			case <-signal:
			case result := <-results:
				t.Fatalf("owned execution ended before barrier: index=%d local=%s error=%T", result.index, result.value.Local, result.err)
			case <-time.After(35 * time.Second):
				t.Fatal("owned execution barrier unavailable")
			}
		}
	}
	await(entered)
	select {
	case <-received:
		t.Fatal("uncommitted half became visible")
	case <-time.After(time.Second):
	}
	require.NoError(t, before.Close(t.Context()))
	cases["prepared_halves_hidden_before_original_roots"] = true
	closeStarted := time.Now()
	closeCtx, closeCancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	closeErr := first.events.CloseContext(closeCtx)
	closeCancel()
	var incomplete *eventsubsystem.ShutdownIncomplete
	require.ErrorAs(t, closeErr, &incomplete)
	require.Equal(t, 1, incomplete.PendingClosures)
	closeWait := time.Since(closeStarted)
	require.Less(t, closeWait, 2*time.Second)
	require.NoError(t, data.pool.PingContext(t.Context()))
	require.NoError(t, data.documents.Client().Ping(t.Context(), readpref.Primary()))
	select {
	case <-joined:
		t.Fatal("close released still-owned callbacks")
	default:
	}
	cases["real_inflight_close_retains_original_dependencies"] = true
	allow()
	await(committed)
	for _, b := range branches {
		require.Zero(t, b.runtime.Stats().CheckCommit, "fixture pre-check window was not respected")
	}
	oldID, oldPID := ownedHostBroker(t, "id"), ownedHostBroker(t, "pid")
	ownedHostBroker(t, "kill")
	require.Equal(t, "137", ownedHostBroker(t, "exit"))
	release()
	cases["real_broker_sigkill_after_commit_before_final_rpc"] = true
	for range 2 {
		select {
		case result := <-results:
			require.Equal(t, transaction.LocalCommitted, result.value.Local)
			require.Len(t, result.value.Branches, 1)
			require.Equal(t, transaction.HalfPrepared, result.value.Branches[0].Half)
			require.Equal(t, transaction.FinalUnknown, result.value.Branches[0].Final)
		case <-time.After(25 * time.Second):
			t.Fatal("real final RPC did not end within its bound")
		}
	}
	<-joined
	waitCtx, waitCancel := context.WithTimeout(t.Context(), 30*time.Second)
	require.NoError(t, first.events.Wait(waitCtx))
	require.NoError(t, first.normal.Close(waitCtx))
	waitCancel()
	for _, b := range branches {
		require.Equal(t, int32(1), b.calls.Load())
		require.Zero(t, b.runtime.Stats().CheckCommit)
		require.True(t, b.runtime.Stats().Stopped)
	}
	cases["local_committed_final_unknown_both_owners"] = true
	cases["original_mysql_and_mongo_facts_witness_atomic"] = true
	require.NoError(t, data.pool.PingContext(t.Context()))
	require.NoError(t, data.documents.Client().Ping(t.Context(), readpref.Primary()))
	ownedHostBroker(t, "start")
	waitOwnedHostTLS(t, opts.RocketMQ)
	require.Equal(t, oldID, ownedHostBroker(t, "id"))
	require.NotEqual(t, oldPID, ownedHostBroker(t, "pid"))
	next := newInstance(true)
	require.NoError(t, next.events.Start(t.Context()))
	mysqlNext, mongoNext := next.events.MySQLTransactionOwner().Runtime(), next.events.MongoTransactionOwner().Runtime()
	after := startObserver()
	subscribe(after)
	maxima := map[string]int{"running": 0, "queued": 0}
	require.Eventually(t, func() bool {
		left, right := mysqlNext.Stats(), mongoNext.Stats()
		if left.RunningChecks+right.RunningChecks > maxima["running"] {
			maxima["running"] = left.RunningChecks + right.RunningChecks
		}
		if left.QueuedChecks+right.QueuedChecks > maxima["queued"] {
			maxima["queued"] = left.QueuedChecks + right.QueuedChecks
		}
		return left.CheckCommit > 0 && right.CheckCommit > 0
	}, 90*time.Second, 50*time.Millisecond, "fresh paused owners did not execute real Broker checks")
	require.False(t, mysqlNext.Stats().Ready)
	require.False(t, mongoNext.Stats().Ready)
	for index, runtime := range []*messagingruntime.TransactionRuntime{mysqlNext, mongoNext} {
		require.Zero(t, runtime.Stats().HalfPreparedResults, "Checker prepared a new message")
		input := branches[index].plan.Input()
		_, reserveErr := runtime.Reserve(t.Context(), transaction.Request{Owner: input.Owner, Scope: input.Scope, Key: input.IdempotencyKey, InputHash: input.InputHash}, branches[index].plan)
		require.ErrorIs(t, reserveErr, messagingruntime.ErrTransactionAdmissionClosed)
	}
	require.LessOrEqual(t, maxima["running"], 4)
	require.LessOrEqual(t, maxima["queued"], 32)
	cases["fresh_paused_instance_actual_broker_checker_commits"] = true
	for owner, binding := range opts.RocketMQ.Transactions {
		limits, limitErr := binding.ProtocolLimits(owner)
		require.NoError(t, limitErr)
		require.Equal(t, 4, limits.Sends)
		require.Equal(t, 2, limits.CheckWorkers)
		require.Equal(t, 16, limits.CheckQueue)
		require.Equal(t, 50, limits.ReconcileBatch)
		require.Equal(t, 2*time.Second, limits.CheckTimeout)
		require.Equal(t, 5*time.Second, limits.RPCTimeout)
	}
	cases["owner_budget_and_callback_observation"] = true
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case value := <-received:
			var expected *branch
			for _, b := range branches {
				if b.plan.Input().Branches[0].Message.Input().ID == value.ID {
					expected = b
				}
			}
			require.NotNil(t, expected)
			require.False(t, seen[value.ID], "unexpected duplicate native receipt")
			require.True(t, bytes.Equal(expected.plan.Input().Branches[0].Wire, value.Wire))
			seen[value.ID] = true
		case <-time.After(45 * time.Second):
			t.Fatal("Checker-committed original wire not consumed")
		}
	}
	cases["idempotent_observer_preserves_first_wire"] = true
	checkers := []transaction.StoreChecker{{Store: storeSQL}, {Store: storeMongo}}
	for i, b := range branches {
		reference := b.owned.References()[0]
		require.Equal(t, transaction.Commit, checkers[i].Check(t.Context(), reference))
		reference.WireHash[0] ^= 0xff
		require.Equal(t, transaction.Unknown, checkers[i].Check(t.Context(), reference))
		message := b.plan.Input().Branches[0]
		hash := sha256.Sum256(message.Wire)
		if i == 0 {
			var actual []byte
			require.NoError(t, data.pool.QueryRowContext(t.Context(), "SELECT wire_hash FROM m3_owner_runtime_facts WHERE id=?", []byte(message.Message.Input().ID)).Scan(&actual))
			require.Equal(t, hash[:], actual)
		} else {
			var fact struct {
				Hash []byte `bson:"wire_hash"`
			}
			require.NoError(t, data.documents.Collection("m3_owner_runtime_facts").FindOne(t.Context(), bson.M{"_id": message.Message.Input().ID}).Decode(&fact))
			require.Equal(t, hash[:], fact.Hash)
		}
	}
	cases["wrong_reference_unknown_without_changed_facts"] = true
	require.NoError(t, after.Close(t.Context()))
	require.NoError(t, next.events.CloseContext(t.Context()))
	require.NoError(t, next.normal.Close(t.Context()))
	require.NoError(t, data.pool.PingContext(t.Context()))
	require.NoError(t, data.documents.Client().Ping(t.Context(), readpref.Primary()))
	require.NoError(t, first.host.Cleanup())
	require.NoError(t, next.host.Cleanup())
	data.close()
	cases["actual_shutdown_and_owned_cleanup"] = true
	sourceSHA, err := exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	receipt, err := json.Marshal(map[string]any{"source_sha": strings.TrimSpace(string(sourceSHA)), "passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "scope": "default QS dual-owner lifecycle, actual Broker restart and next-instance Checker; protocol fixture facts", "domain_api_provider_system_not_claimed": true, "producer_process_exit_not_claimed": true, "owned_databases_dropped": true, "owned_runtimes_drained": true, "same_broker_container_restarted": true, "local_executor_calls": []int32{branches[0].calls.Load(), branches[1].calls.Load()}, "mongo_database_callbacks": mongoCallbacks.Load(), "new_checker_commits": []int64{mysqlNext.Stats().CheckCommit, mongoNext.Stats().CheckCommit}, "observed_callbacks": maxima, "duplicate_deliveries": duplicateDeliveries.Load(), "close_wait_ms": closeWait.Milliseconds(), "new_half_prepared_results": []int64{mysqlNext.Stats().HalfPreparedResults, mongoNext.Stats().HalfPreparedResults}})
	require.NoError(t, err)
	fmt.Println("M3_QS_DEFAULT_OWNER_RECEIPT " + string(receipt))
}
