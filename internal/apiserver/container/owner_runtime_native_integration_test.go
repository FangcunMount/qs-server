//go:build integration && m3_host_runtime

package container

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	case "hold-checks", "release-checks":
		value := "600000"
		if action == "release-checks" {
			value = "6000"
		}
		// Each JVM/admin operation has its own bounded fixture budget. A
		// successful update must not consume the readback's remaining time.
		adminCall := func(arguments ...string) ([]byte, error) {
			operation, stop := context.WithTimeout(t.Context(), 45*time.Second)
			defer stop()
			command := []string{"exec", broker, "java", "-Xms32m", "-Xmx128m", "-XX:MaxDirectMemorySize=32m", "-cp", "/opt/rocketmq/conf:/opt/rocketmq/lib/*", "org.apache.rocketmq.tools.command.MQAdminStartup"}
			return exec.CommandContext(operation, "docker", append(command, arguments...)...).Output()
		}
		out, e := adminCall("updateBrokerConfig", "-b", "127.0.0.1:10911", "-k", "transactionTimeOut", "-v", value)
		require.NoError(t, e)
		require.NotEmpty(t, out)
		actual, e := adminCall("getBrokerConfig", "-b", "127.0.0.1:10911")
		require.NoError(t, e)
		found := false
		for line := range strings.SplitSeq(string(actual), "\n") {
			fields := strings.Fields(strings.ReplaceAll(line, "=", " "))
			if len(fields) == 2 && fields[0] == "transactionTimeOut" {
				require.Equal(t, value, fields[1])
				found = true
			}
		}
		require.True(t, found, "owned check hold/release not read back")

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

// Export only fixed classifications/numeric driver codes, never driver text,
// DSNs, execution tokens, references, payloads or SQL statements.
func ownerNativeError(err error) map[string]any {
	kind, code := "none", 0
	switch {
	case err == nil:
	case strings.Contains(err.Error(), "initial settings deadline exceeded"):
		kind = "deadline"
	case errors.Is(err, context.DeadlineExceeded):
		kind = "deadline"
	case errors.Is(err, context.Canceled):
		kind = "canceled"
	case errors.Is(err, transaction.ErrStaleExecution):
		kind = "stale_execution"
	case errors.Is(err, messagingruntime.ErrProtocolProtection):
		kind = "protocol_protection"
	case errors.Is(err, messagingruntime.ErrTransactionAdmissionClosed):
		kind = "host_admission"
	case errors.Is(err, transaction.ErrBinding):
		kind = "binding"
	case errors.Is(err, transaction.ErrTransactionRequired):
		kind = "transaction_required"
	default:
		kind = "other"
		var protocolStatus interface {
			error
			GetCode() int32
		}
		var registrationError prometheus.AlreadyRegisteredError
		var sqlError *driver.MySQLError
		var mongoError mongo.CommandError
		if errors.As(err, &registrationError) {
			kind = "metrics_registration"
		} else if errors.As(err, &sqlError) {
			kind, code = "mysql", int(sqlError.Number)
		} else if errors.As(err, &mongoError) {
			kind, code = "mongo", int(mongoError.Code)
		} else if errors.As(err, &protocolStatus) {
			if n := protocolStatus.GetCode(); n >= 0 && n <= 1000000000 {
				code = int(n)
			}
		} else if rpc, ok := status.FromError(err); ok {
			code = int(rpc.Code())
			if rpc.Code() == codes.DeadlineExceeded {
				kind = "deadline"
			}
		}
	}
	return map[string]any{"kind": kind, "code": code}
}

// Real default subsystem and both original stores. The frozen wire and fact
// rows below are protocol fixtures, not domain/API/provider/system acceptance.
func TestM3DefaultOwnerRuntime(t *testing.T) { runDefaultOwnerRuntime(t, false) }

// Original QS owner/root component proof; not full domain/API or system acceptance.
func TestM3DefaultOwnerNativeRecovery(t *testing.T) { runDefaultOwnerRuntime(t, true) }

func runDefaultOwnerRuntime(t *testing.T, recovery bool) {
	directory := os.Getenv("QS_M3_NATIVE_RECOVERY_DIRECTORY")
	if recovery {
		require.Equal(t, "true", os.Getenv("GITHUB_ACTIONS"))
		require.Equal(t, "Linux", os.Getenv("RUNNER_OS"))
		require.Equal(t, "github-hosted", os.Getenv("RUNNER_ENVIRONMENT"))
		require.True(t, filepath.IsAbs(directory))
		info, e := os.Lstat(directory)
		require.NoError(t, e)
		require.True(t, info.IsDir())
		require.Zero(t, info.Mode().Perm()&0077)
	}
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
	// This protocol fixture has no domain projections. Explicitly disable
	// their catalog consumers instead of silently starting an unbound handler;
	// the real SDK observer below remains enabled with consume-only credentials.
	registry, err := catalog.NewEffectiveRegistry(routes, catalog.DefaultSpecs())
	require.NoError(t, err)
	consumers := map[string]eventsubsystem.ConsumerOptions{}
	for _, spec := range registry.Snapshot() {
		for _, consumer := range spec.AdditionalConsumers {
			consumers[consumer.ID] = eventsubsystem.ConsumerOptions{Enabled: false}
		}
	}
	require.NotEmpty(t, consumers)
	cases := map[string]bool{}
	verifyProtection := os.Getenv("QS_M3_VERIFY_PROTOCOL_PROTECTION") == "1"
	protectionCases := map[string]bool{}
	verifyNative := os.Getenv("QS_M3_VERIFY_NATIVE_LIMIT") == "1"
	require.False(t, verifyNative && verifyProtection, "native evidence and timestamp-aging profiles are distinct")
	nativeCases := map[string]bool{}
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
		events, e := eventsubsystem.NewWithTransactionOwners(eventsubsystem.Options{MySQLDB: data.sql, MongoDB: data.documents, Catalog: routes, WirePublisher: normal, PublisherMode: eventruntime.PublishModeMQ, Consumers: consumers}, &options)
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
	for _, file := range []string{"000106_transaction_messaging.up.sql", "000107_business_retry_schedule.up.sql", "000110_native_transaction_limit_notices.up.sql"} {
		body, e := os.ReadFile(filepath.Join("..", "..", "pkg", "migration", "migrations", "mysql", file))
		require.NoError(t, e)
		_, e = data.pool.ExecContext(t.Context(), string(body))
		require.NoError(t, e)
	}
	for _, file := range []string{"000041_transaction_messaging.up.json", "000042_business_retry_schedule.up.json", "000043_native_transaction_limit_notices.up.json"} {
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
		phase   atomic.Int32
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
	reservedAt := time.Now()
	diagnostic := func(stage string, index int, local transaction.LocalOutcome, err error) {
		t.Helper()
		phases := make([]int32, 0, len(branches))
		leases := make([]int64, 0, len(branches))
		for _, branch := range branches {
			phases = append(phases, branch.phase.Load())
			leases = append(leases, time.Until(branch.owned.LeaseUntil).Milliseconds())
		}
		raw, e := json.Marshal(map[string]any{"stage": stage, "owner_index": index,
			"local": local, "error": ownerNativeError(err), "root_phases": phases,
			"lease_remaining_ms": leases, "elapsed_ms": time.Since(reservedAt).Milliseconds()})
		require.NoError(t, e)
		fmt.Println("M3_QS_OWNER_DIAGNOSTIC " + string(raw))
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
				b.phase.Store(1)
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
					b.phase.Store(2)
					e = data.sql.WithContext(ctx).Transaction(func(original *gorm.DB) error {
						guard, e := storeSQL.BindGORM(ctx, original, execution)
						if e != nil {
							return e
						}
						b.phase.Store(3)
						if e = original.Exec("INSERT INTO m3_owner_runtime_facts(id,wire_hash) VALUES(?,?)", input.Message.Input().ID, hash[:]).Error; e != nil {
							return e
						}
						b.phase.Store(4)
						return guard.CommitWitness(ctx)
					})
				} else {
					b.phase.Store(2)
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
						b.phase.Store(3)
						if _, e = data.documents.Collection("m3_owner_runtime_facts").InsertOne(original, bson.M{"_id": input.Message.Input().ID, "wire_hash": hash[:]}); e != nil {
							return nil, e
						}
						b.phase.Store(4)
						return nil, guard.CommitWitness(original)
					}, mongooptions.Transaction().SetWriteConcern(writeconcern.Majority()))
				}
				if e != nil {
					return transaction.LocalUnknown, e
				}
				// No Broker operation or non-deterministic input occurs inside either root.
				b.phase.Store(5)
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
	await := func(signal <-chan int, stage string) {
		t.Helper()
		for range 2 {
			select {
			case <-signal:
			case result := <-results:
				diagnostic(stage, result.index, result.value.Local, result.err)
				t.Fatalf("owned execution ended before barrier: index=%d local=%s error=%T", result.index, result.value.Local, result.err)
			case <-time.After(35 * time.Second):
				diagnostic(stage, -1, transaction.LocalUnknown, nil)
				t.Fatal("owned execution barrier unavailable")
			}
		}
	}
	await(entered, "local_entered")
	select {
	case <-received:
		t.Fatal("uncommitted half became visible")
	case <-time.After(time.Second):
	}
	diagnostic("before_observer_close", -1, transaction.LocalNotStarted, nil)
	require.NoError(t, before.Close(t.Context()))
	diagnostic("after_observer_close", -1, transaction.LocalNotStarted, nil)
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
	await(committed, "local_committed")
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

	if recovery {
		require.False(t, verifyProtection)
		require.False(t, verifyNative)
		write := func(name string, value any) {
			t.Helper()
			raw, e := json.Marshal(value)
			require.NoError(t, e)
			f, e := os.OpenFile(filepath.Join(directory, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			require.NoError(t, e)
			_, e = f.Write(raw)
			require.NoError(t, e)
			require.NoError(t, f.Close())
		}
		wait := func(name string) {
			t.Helper()
			deadline := time.Now().Add(360 * time.Second)
			for time.Now().Before(deadline) {
				if info, e := os.Lstat(filepath.Join(directory, name)); e == nil {
					require.True(t, info.Mode().IsRegular())
					require.Zero(t, info.Mode().Perm()&0077)
					return
				}
				select {
				case <-t.Context().Done():
					t.Fatal("owned QS recovery control cancelled")
				case <-time.After(50 * time.Millisecond):
				}
			}
			t.Fatal("owned QS recovery control timeout")
		}
		ownedHostBroker(t, "start")
		waitOwnedHostTLS(t, opts.RocketMQ)
		ownedHostBroker(t, "release-checks")
		records := make([]map[string]any, 0, 2)
		for i, b := range branches {
			ref := b.owned.References()[0]
			binding := sqlBinding
			if i == 1 {
				binding = mongoBinding
			}
			records = append(records, map[string]any{"owner": binding.Owner, "store_id": binding.StoreID, "writer_epoch": binding.WriterEpoch, "operation_id": ref.OperationID, "branch_id": ref.BranchID, "fence": ref.Fence, "message_id": ref.MessageID, "wire_hash": hex.EncodeToString(ref.WireHash[:])})
		}
		write("original-roots.json", map[string]any{"database": data.documents.Name(), "branches": records, "original_root_calls": 2, "original_owners_stopped": true, "broker_sigkill_after_commit": true})
		wait("record-notices")
		for i, b := range branches {
			raw, e := os.ReadFile(filepath.Join(directory, "native-"+b.owner+".json"))
			require.NoError(t, e)
			expected := sha256.Sum256(raw)
			evidence, e := messagingruntime.ParseNativeLimitEvidence(raw, expected)
			require.NoError(t, e)
			require.Equal(t, 1, evidence.IndexCount())
			if i == 0 {
				notices, e := messagingruntime.NewSQLNativeLimits(data.pool, sqlBinding, "m3-1")
				require.NoError(t, e)
				require.NoError(t, notices.RecordNativeLimit(t.Context(), evidence))
			} else {
				notices, e := messagingruntime.NewMongoNativeLimits(data.documents, mongoBinding, "m3-1")
				require.NoError(t, e)
				require.NoError(t, notices.RecordNativeLimit(t.Context(), evidence))
			}
		}
		write("notices.json", map[string]bool{"both_original_stores_recorded": true})
		wait("verify")
		for i, b := range branches {
			ref := b.owned.References()[0]
			if i == 0 {
				proof, e := storeSQL.Evidence(t.Context(), ref)
				require.NoError(t, e)
				require.Equal(t, transaction.Commit, transaction.Resolve(ref, proof))
				var n int
				require.NoError(t, data.pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM m3_owner_runtime_facts").Scan(&n))
				require.Equal(t, 1, n)
				require.NoError(t, data.pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_tx_witness WHERE final_outcome='commit_confirmed' AND final_recorded_at IS NOT NULL").Scan(&n))
				require.Equal(t, 1, n)
				require.NoError(t, data.pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM messaging_native_limit_notices WHERE status='resolved' AND resolved_at IS NOT NULL AND resolution_receipt_sha256 IS NOT NULL").Scan(&n))
				require.Equal(t, 1, n)
			} else {
				proof, e := storeMongo.Evidence(t.Context(), ref)
				require.NoError(t, e)
				require.Equal(t, transaction.Commit, transaction.Resolve(ref, proof))
				n, e := data.documents.Collection("m3_owner_runtime_facts").CountDocuments(t.Context(), bson.M{})
				require.NoError(t, e)
				require.EqualValues(t, 1, n)
				n, e = data.documents.Collection("rm_tx_execution").CountDocuments(t.Context(), bson.M{"state": "committed", "witnesses.0.final_outcome": "commit_confirmed", "witnesses.0.final_recorded_at": bson.M{"$type": "date"}})
				require.NoError(t, e)
				require.EqualValues(t, 1, n)
				n, e = data.documents.Collection("messaging_native_limit_notices").CountDocuments(t.Context(), bson.M{"status": "resolved", "resolved_at": bson.M{"$type": "date"}})
				require.NoError(t, e)
				require.EqualValues(t, 1, n)
			}
			require.Equal(t, int32(1), b.calls.Load())
		}
		next := newInstance(true)
		require.NoError(t, next.normal.Start())
		require.NoError(t, next.events.Start(t.Context()))
		require.True(t, next.events.MySQLTransactionOwner().Runtime().Stats().Ready)
		require.True(t, next.events.MongoTransactionOwner().Runtime().Stats().Ready)
		closeCtx, closeCancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer closeCancel()
		require.NoError(t, next.events.CloseContext(closeCtx))
		require.NoError(t, next.normal.Close(closeCtx))
		require.NoError(t, next.host.Cleanup())
		require.NoError(t, data.pool.PingContext(t.Context()))
		require.NoError(t, data.documents.Client().Ping(t.Context(), readpref.Primary()))
		data.close()
		fmt.Println(`M3_QS_NATIVE_RECOVERY {"passed":7,"failed":0,"skipped":0,"original_roots":2,"both_owners_stopped":true,"both_stores_resolved":true,"paused_replacement_drained":true,"owned_databases_removed":true,"whole_api":false,"system_acceptance":false}`)
		return
	}
	if verifyProtection {
		// Fixture-only audit clock aging in the two owned temporary databases.
		// Do not mutate business facts, terminal witnesses, IDs, wire or RPC results.
		for i, b := range branches {
			ref := b.owned.References()[0]
			key := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence)
			if i == 0 {
				result, e := data.pool.ExecContext(t.Context(), "UPDATE rm_tx_witness SET half_recorded_at=UTC_TIMESTAMP(6)-INTERVAL 10 MINUTE WHERE execution_key=? AND state='committed' AND half_outcome='prepared' AND final_outcome='unknown'", key[:])
				require.NoError(t, e)
				n, e := result.RowsAffected()
				require.NoError(t, e)
				require.EqualValues(t, 1, n)
			} else {
				result, e := data.documents.Collection("rm_tx_execution").UpdateOne(t.Context(), bson.M{"_id": key[:], "state": transaction.Committed, "witnesses.0.half_outcome": transaction.HalfPrepared, "witnesses.0.final_outcome": transaction.FinalUnknown}, bson.M{"$set": bson.M{"witnesses.0.half_recorded_at": time.Now().UTC().Add(-10 * time.Minute)}})
				require.NoError(t, e)
				require.EqualValues(t, 1, result.MatchedCount)
			}
		}
		protectionCases["exact_owned_audit_timestamp_fixture_only"] = true
	}
	ownedHostBroker(t, "start")
	waitOwnedHostTLS(t, opts.RocketMQ)
	if verifyNative {
		ownedHostBroker(t, "hold-checks")
	}
	require.Equal(t, oldID, ownedHostBroker(t, "id"))
	require.NotEqual(t, oldPID, ownedHostBroker(t, "pid"))
	// Verify actual signed QueryRoute for both owners and the consume-only
	// account before creating new clients. Retry only read-side startup probes;
	// never an executor, half message, model call or failed SDK runtime.
	index, routeErr := waitOwnedProxyRoutes(t.Context(), opts.RocketMQ, observer.RocketMQ)
	if routeErr != nil {
		diagnostic("next_owner_start", index, transaction.LocalUnknown, routeErr)
	}
	require.NoError(t, routeErr)
	// A listening TLS socket does not prove the restarted Proxy can serve the
	// restored routes/authentication. Warm the real consume-only observer before
	// starting fresh owners; do not relax the production SDK's five-second RPC.
	after := startObserver()
	subscribe(after)
	require.Eventually(t, func() bool { return after.Ready(10 * time.Second) }, 30*time.Second, 50*time.Millisecond)
	// With protection enabled, reproduce the normal configured-open production
	// startup. Registration must finish while the observed latch keeps writes
	// closed; a manual-open attempt is not the startup barrier.
	next := newInstance(!verifyProtection && !verifyNative)
	startErr := next.events.Start(t.Context())
	if startErr != nil {
		diagnostic("next_owner_start", -1, transaction.LocalUnknown, startErr)
	}
	require.NoError(t, startErr)
	mysqlNext, mongoNext := next.events.MySQLTransactionOwner().Runtime(), next.events.MongoTransactionOwner().Runtime()
	if verifyNative {
		// This file and its independent digest come from the infra-owned real
		// SYS index exporter; never synthesize a positive native fault here.
		path := os.Getenv("QS_M3_NATIVE_LIMIT_AUDIT")
		require.True(t, filepath.IsAbs(path))
		info, e := os.Lstat(path)
		require.NoError(t, e)
		require.True(t, info.Mode().IsRegular())
		require.Zero(t, info.Mode().Perm()&0077)
		require.LessOrEqual(t, info.Size(), int64(65536))
		raw, e := os.ReadFile(path)
		require.NoError(t, e)
		digest, e := hex.DecodeString(os.Getenv("QS_M3_NATIVE_LIMIT_SHA256"))
		require.NoError(t, e)
		require.Len(t, digest, 32)
		var expected [32]byte
		copy(expected[:], digest)
		evidence, e := messagingruntime.ParseNativeLimitEvidence(raw, expected)
		require.NoError(t, e)
		require.Equal(t, 1, evidence.IndexCount())
		sqlNotices, e := messagingruntime.NewSQLNativeLimits(data.pool, sqlBinding, "m3-1")
		require.NoError(t, e)
		mongoNotices, e := messagingruntime.NewMongoNativeLimits(data.documents, mongoBinding, "m3-1")
		require.NoError(t, e)
		for i, runtime := range []*messagingruntime.TransactionRuntime{mysqlNext, mongoNext} {
			require.True(t, runtime.Stats().Ready, "no native governance existed before ingestion")
			input := branches[i].plan.Input()
			duplicate, e := runtime.Reserve(t.Context(), transaction.Request{Owner: input.Owner, Scope: input.Scope, Key: input.IdempotencyKey, InputHash: input.InputHash}, branches[i].plan)
			require.NoError(t, e)
			require.False(t, duplicate.Acquired)
			require.Zero(t, runtime.Stats().CheckCommit, "held Broker checks ran before native protection")
		}
		nativeCases["live_configured_open_before_private_native_ingest"] = true
		for range 2 {
			require.NoError(t, sqlNotices.RecordNativeLimit(t.Context(), evidence))
			require.NoError(t, mongoNotices.RecordNativeLimit(t.Context(), evidence))
		}
		var count int
		require.NoError(t, data.pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM messaging_native_limit_notices").Scan(&count))
		require.Equal(t, evidence.IndexCount(), count)
		n, e := data.documents.Collection("messaging_native_limit_notices").CountDocuments(t.Context(), bson.D{})
		require.NoError(t, e)
		require.EqualValues(t, evidence.IndexCount(), n)
		nativeCases["actual_sys_index_recorded_idempotently_on_both_original_primaries"] = true
		verifyBlocked := func(runtimes ...*messagingruntime.TransactionRuntime) {
			t.Helper()
			for i, runtime := range runtimes {
				require.Eventually(t, func() bool {
					status := runtime.ProtectionStats()
					return status.Fresh && status.NativeLimitActive && status.Protected && status.Reason == "native_transaction_limit"
				}, 15*time.Second, 50*time.Millisecond)
				require.True(t, runtime.ProtectionStats().SamplerRunning)
				require.False(t, runtime.Stats().Stopped)
				require.ErrorIs(t, runtime.SetAdmission(true), messagingruntime.ErrTransactionAdmissionClosed)
				input := branches[i].plan.Input()
				_, e := runtime.Reserve(t.Context(), transaction.Request{Owner: input.Owner, Scope: input.Scope, Key: input.IdempotencyKey, InputHash: input.InputHash}, branches[i].plan)
				require.ErrorIs(t, e, messagingruntime.ErrTransactionAdmissionClosed)
				_, e = runtime.Execute(t.Context(), branches[i].owned, nil)
				require.ErrorIs(t, e, messagingruntime.ErrTransactionAdmissionClosed)
				status := runtime.ProtectionStats()
				require.Error(t, runtime.ReviewProtection(status.Generation, "native-limit-no-resolution"))
				require.Zero(t, runtime.Stats().HalfPreparedResults)
			}
		}
		verifyBlocked(mysqlNext, mongoNext)
		nativeCases["live_sampler_seals_reserve_execute_plain_open_and_review"] = true
		require.NoError(t, next.events.CloseContext(t.Context()))
		require.NoError(t, next.normal.Close(t.Context()))
		require.NoError(t, next.host.Cleanup())
		next = newInstance(false)
		require.NoError(t, next.events.Start(t.Context()))
		mysqlNext, mongoNext = next.events.MySQLTransactionOwner().Runtime(), next.events.MongoTransactionOwner().Runtime()
		verifyBlocked(mysqlNext, mongoNext)
		nativeCases["new_configured_open_instance_retains_durable_deny_before_registration"] = true
		ownedHostBroker(t, "release-checks")
	}

	if verifyProtection {
		for _, runtime := range []*messagingruntime.TransactionRuntime{mysqlNext, mongoNext} {
			status := runtime.ProtectionStats()
			require.True(t, status.Protected)
			require.Equal(t, "confirmation_gap", status.Reason)
			require.Positive(t, status.Generation)
			require.True(t, status.SamplerRunning)
			require.ErrorIs(t, runtime.SetAdmission(true), messagingruntime.ErrTransactionAdmissionClosed)
		}
		protectionCases["initial_authoritative_observation_latches_before_registration"] = true
		protectionCases["plain_admission_cannot_bypass_latch"] = true
	}
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
	if verifyNative {
		for _, runtime := range []*messagingruntime.TransactionRuntime{mysqlNext, mongoNext} {
			require.Eventually(t, func() bool {
				stats := runtime.Stats()
				return stats.RecoveryCommitConfirmed > 0 && stats.RecoveryAuditRecorded > 0 && runtime.ProtectionStats().ConfirmationGapBranches == 0
			}, 20*time.Second, 50*time.Millisecond)
			require.True(t, runtime.ProtectionStats().NativeLimitActive)
			require.True(t, runtime.ProtectionStats().Protected)
			require.ErrorIs(t, runtime.SetAdmission(true), messagingruntime.ErrTransactionAdmissionClosed)
		}
		nativeCases["real_original_checker_commit_rpc_and_positive_audit_while_native_blocked"] = true
		nativeCases["first_wire_facts_and_single_executor_preserved_without_new_half"] = true
		nativeCases["no_automatic_or_unaudited_governance_resolution"] = true
	}

	if verifyProtection {
		for i, runtime := range []*messagingruntime.TransactionRuntime{mysqlNext, mongoNext} {
			require.Eventually(t, func() bool {
				status, stats := runtime.ProtectionStats(), runtime.Stats()
				return status.Fresh && status.ConfirmationGapBranches == 0 && stats.RecoveryCommitConfirmed > 0 && stats.RecoveryAuditRecorded > 0
			}, 20*time.Second, 50*time.Millisecond, "real positive recovery RPC/audit did not clear observed gap")
			status := runtime.ProtectionStats()
			require.True(t, status.Protected, "healthy sample automatically reopened admission")
			require.ErrorIs(t, runtime.SetAdmission(true), messagingruntime.ErrTransactionAdmissionClosed)
			require.NoError(t, runtime.SetAdmission(false), "pause explicitly before named review")
			require.ErrorIs(t, runtime.ReviewProtection(status.Generation+1, "native-owned-recovery-review"), messagingruntime.ErrProtocolProtection)
			require.NoError(t, runtime.ReviewProtection(status.Generation, "native-owned-recovery-review"))
			require.False(t, runtime.Stats().Ready, "named review bypassed explicitly paused host gate")
			require.NoError(t, runtime.SetAdmission(true))
			input := branches[i].plan.Input()
			duplicate, e := runtime.Reserve(t.Context(), transaction.Request{Owner: input.Owner, Scope: input.Scope, Key: input.IdempotencyKey, InputHash: input.InputHash}, branches[i].plan)
			require.NoError(t, e)
			require.False(t, duplicate.Acquired)
			require.Equal(t, branches[i].plan.Digest(), duplicate.Execution.Plan.Digest())
			require.Zero(t, runtime.Stats().HalfPreparedResults)
			require.Equal(t, int32(1), branches[i].calls.Load())
			require.NoError(t, runtime.SetAdmission(false))
		}
		protectionCases["real_broker_recovery_rpc_then_positive_audit_clears_gap"] = true
		protectionCases["healthy_sampling_does_not_unlatch"] = true
		protectionCases["generation_bound_named_review_then_explicit_admission"] = true
		protectionCases["duplicate_plan_no_new_half_or_business_executor"] = true
	}
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
	receipt, err := json.Marshal(map[string]any{"source_sha": strings.TrimSpace(string(sourceSHA)), "passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "scope": "default QS dual-owner lifecycle, actual Broker restart and next-instance Checker; protocol fixture facts", "domain_api_provider_system_not_claimed": true, "domain_consumers_explicitly_disabled": true, "producer_process_exit_not_claimed": true, "owned_databases_dropped": true, "owned_runtimes_drained": true, "same_broker_container_restarted": true, "local_executor_calls": []int32{branches[0].calls.Load(), branches[1].calls.Load()}, "mongo_database_callbacks": mongoCallbacks.Load(), "new_checker_commits": []int64{mysqlNext.Stats().CheckCommit, mongoNext.Stats().CheckCommit}, "observed_callbacks": maxima, "duplicate_deliveries": duplicateDeliveries.Load(), "close_wait_ms": closeWait.Milliseconds(), "new_half_prepared_results": []int64{mysqlNext.Stats().HalfPreparedResults, mongoNext.Stats().HalfPreparedResults}})
	require.NoError(t, err)
	fmt.Println("M3_QS_DEFAULT_OWNER_RECEIPT " + string(receipt))
	if verifyNative {
		nativeCases["actual_sampler_sdk_shutdown_joined_original_pools_owned_cleanup"] = !mysqlNext.ProtectionStats().SamplerRunning && !mongoNext.ProtectionStats().SamplerRunning && mysqlNext.Stats().Stopped && mongoNext.Stats().Stopped
		require.True(t, nativeCases["actual_sampler_sdk_shutdown_joined_original_pools_owned_cleanup"])
		receipt, e := json.Marshal(map[string]any{"source_sha": strings.TrimSpace(string(sourceSHA)), "passed": len(nativeCases), "failed": 0, "skipped": 0, "cases": nativeCases, "owner_count": 2, "real_sys_index_required": true, "timestamp_aging_not_used": true, "governance_resolution_not_claimed": true, "domain_api_provider_system_not_claimed": true, "owned_databases_dropped": true})
		require.NoError(t, e)
		fmt.Println("M3_QS_NATIVE_LIMIT_RECEIPT " + string(receipt))
	}

	if verifyProtection {
		protectionCases["actual_sampler_and_sdk_shutdown_joined"] = !mysqlNext.ProtectionStats().SamplerRunning && !mongoNext.ProtectionStats().SamplerRunning && mysqlNext.Stats().Stopped && mongoNext.Stats().Stopped
		require.True(t, protectionCases["actual_sampler_and_sdk_shutdown_joined"])
		receipt, err := json.Marshal(map[string]any{"source_sha": strings.TrimSpace(string(sourceSHA)), "passed": len(protectionCases), "failed": 0, "skipped": 0, "cases": protectionCases, "fixture_timestamp_aging_seconds": 600, "real_ten_minute_elapsed_not_claimed": true, "owner_count": 2, "domain_api_provider_system_not_claimed": true, "owned_databases_dropped": true})
		require.NoError(t, err)
		fmt.Println("M3_QS_PROTOCOL_PROTECTION_RECEIPT " + string(receipt))
	}
}
