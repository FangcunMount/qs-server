//go:build integration && m3_ai_command_broker && m3_host_faults

package aibridge

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	boundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	qsoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	jose "github.com/go-jose/go-jose/v4"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The original command store and witness borrow the same proxy-backed root.
// Setup and fresh authoritative reads use only the same owned database directly.
func TestM3AICommandOriginalCommitPacketFault(t *testing.T) {
	fault := os.Getenv("QS_M3_FAULT_BOUNDARY")
	require.Contains(t, []string{"before", "after"}, fault)
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	cfg, err := driver.ParseDSN(os.Getenv("QS_M3_COMMAND_DSN"))
	require.NoError(t, err)
	require.Equal(t, "tcp", cfg.Net)
	require.Empty(t, cfg.DBName)
	host, _, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback())
	cfg.ParseTime, cfg.MultiStatements = true, true
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	name := fmt.Sprintf("qs_m3_ai_command_packet_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`")
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`")
		require.NoError(t, err)
	})
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	install := func(file string) {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "pkg", "migration", "migrations", "mysql", file))
		require.NoError(t, err)
		_, err = pool.ExecContext(t.Context(), string(raw))
		require.NoError(t, err)
	}
	for _, file := range []string{"000072_ai_bridge_delivery.up.sql", "000083_ai_runtime_index.up.sql", "000091_ai_messaging.up.sql", "000094_ai_messaging_admission.up.sql", "000097_ai_command_retirement.up.sql", "000106_transaction_messaging.up.sql", "000110_native_transaction_limit_notices.up.sql", "000108_ai_transaction_command_order.up.sql"} {
		install(file)
	}
	load := func(key string) qsoptions.MessagingOptions {
		raw, err := os.ReadFile(os.Getenv(key))
		require.NoError(t, err)
		var o qsoptions.MessagingOptions
		require.NoError(t, json.Unmarshal(raw, &o))
		require.NotNil(t, o.RocketMQ)
		host, _, err := net.SplitHostPort(o.RocketMQ.Endpoint)
		require.NoError(t, err)
		require.True(t, net.ParseIP(host).IsLoopback())
		return o
	}
	producerOptions, observerOptions := load("QS_M3_PRODUCER_CONFIG"), load("QS_M3_OBSERVER_CONFIG")
	binding := producerOptions.RocketMQ.Transactions["qs.mysql"]
	_, err = pool.ExecContext(t.Context(), "INSERT INTO rm_tx_binding VALUES(?,?,?,1)", []byte("qs.mysql"), []byte(binding.StoreID), []byte(binding.WriterEpoch))
	require.NoError(t, err)
	key := func(id string) jose.JSONWebKey {
		value, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		return jose.JSONWebKey{Key: value, KeyID: id}
	}
	signing, recipient := key("qs-sign"), key("ai-recipient")
	var seals atomic.Int32
	seal := func(kind pb.MessagingKind, id, aggregate, org, original string, body *pb.MessagingBody) (*app.PreparedMessaging, error) {
		seals.Add(1)
		return app.ProtectMessaging(kind, id, aggregate, "", org, original, body, signing, recipient.Public())
	}
	count := func(query string, args ...any) int {
		var n int
		require.NoError(t, pool.QueryRowContext(t.Context(), query, args...).Scan(&n))
		return n
	}
	proxy := os.Getenv("QS_M3_FAULT_MYSQL_PROXY")
	proxyHost, _, err := net.SplitHostPort(proxy)
	require.NoError(t, err)
	require.True(t, net.ParseIP(proxyHost).IsLoopback())
	cfg.Addr = proxy
	originalPool, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	originalPool.SetMaxOpenConns(4)
	t.Cleanup(func() { require.NoError(t, originalPool.Close()) })
	originalDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: originalPool}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	shared, err := boundary.New(originalDB, &producerOptions, commandNativeRoutes{}, []string{app.CommandsTopic})
	require.NoError(t, err)
	commands, err := NewTransactionCommands(originalDB, shared, seal)
	require.NoError(t, err)
	require.NoError(t, commands.Validate(t.Context()))
	stop := func(owner *boundary.Committer) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, owner.Runtime().Close(ctx))
	}
	t.Cleanup(func() { stop(shared) })
	require.NoError(t, shared.Runtime().Start(t.Context()))
	require.NoError(t, shared.Runtime().SetAdmission(true))
	transportConfig, err := observerOptions.RocketMQ.Load([]string{app.CommandsTopic}, 1, 5*time.Second)
	require.NoError(t, err)
	consumer, err := rocketmq.NewSubscriber(rocketmq.SubscriberConfig{Config: transportConfig, MaxAttempts: 3, InvisibleDuration: 15 * time.Second, HandlerTimeout: 5 * time.Second, RetryDelay: time.Second})
	require.NoError(t, err)
	closeConsumer := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, consumer.Close(ctx))
	}
	t.Cleanup(closeConsumer)
	received := make(chan transport.Received, 4)
	require.NoError(t, consumer.Subscribe(t.Context(), app.CommandsTopic, "qs-m3-ai-command-observer", func(_ context.Context, delivery transport.Delivery) error {
		if e := delivery.Ack(); e != nil {
			return e
		}
		select {
		case received <- delivery.Message():
			return nil
		default:
			return errors.New("bounded original command observer full")
		}
	}, func(context.Context, transport.Failure) error {
		return errors.New("original command failure audit refused")
	}))
	require.Eventually(t, func() bool { return consumer.Ready(10 * time.Second) }, 30*time.Second, 50*time.Millisecond)
	store := &MessagingCommandStore{Store: &Store{DB: originalPool}, Messaging: NewMessagingStore(), Seal: seal, Transactions: commands}
	request := app.Start{RequestID: uuid.NewString(), Actor: app.Actor{OrgID: "7", SubjectID: "9"}, TesteeID: "19", AssessmentIDs: []string{"21"}, Goal: "original packet root", Evidence: []app.EvidenceItem{{AssessmentID: "21", TesteeID: "19", ReportID: "31", SourceVersion: "frozen-v1", Facts: []app.Fact{{Ref: "standard-report", Value: "frozen owned fixture"}}}}}
	_, err = pool.ExecContext(t.Context(), "UPDATE ai_messaging_admission SET closed=FALSE WHERE singleton=1")
	require.NoError(t, err)
	applicationError := store.StageStart(t.Context(), request)
	command, err := app.NewTransactionCommand(pb.MessagingKind_START, request.RequestID, request.RequestID, app.OperationScope{OrganizationID: "7", SubjectID: "9", ResourceID: request.RequestID}, startMessagingBody(request))
	require.NoError(t, err)
	authority, err := txmysql.New(pool, transaction.Binding{Owner: "qs.mysql", StoreID: binding.StoreID, WriterEpoch: binding.WriterEpoch})
	require.NoError(t, err)
	lookup := transaction.Request{Owner: "qs.mysql", Scope: "org:7", Key: command.Key(), InputHash: command.InputHash()}
	winner, err := authority.Lookup(t.Context(), lookup)
	require.NoError(t, err)
	require.Len(t, winner.Execution.References(), 1)
	reference := winner.Execution.References()[0]
	evidence, err := authority.Evidence(t.Context(), reference)
	require.NoError(t, err)
	decision := transaction.Resolve(reference, evidence)
	facts := count("SELECT COUNT(*) FROM ai_bridge_requests WHERE request_id=?", request.RequestID)
	if fault == "after" {
		require.NoError(t, applicationError)
		require.Equal(t, 1, facts)
		require.Equal(t, transaction.Commit, decision)
	} else {
		require.Error(t, applicationError)
		require.Zero(t, facts)
		require.NotEqual(t, transaction.Commit, decision)
	}
	duplicateError := store.StageStart(t.Context(), request)
	if fault == "after" {
		require.NoError(t, duplicateError)
	} else {
		require.Error(t, duplicateError)
	}
	duplicate, err := authority.Lookup(t.Context(), lookup)
	require.NoError(t, err)
	require.Equal(t, winner.Execution.Plan.Digest(), duplicate.Execution.Plan.Digest())
	require.EqualValues(t, 1, seals.Load())
	require.Equal(t, int64(1), shared.Runtime().Stats().CompletedResults)
	require.Equal(t, int64(1), shared.Runtime().Stats().HalfPreparedResults)
	stop(shared)
	paused := producerOptions
	cloned := *producerOptions.RocketMQ
	publish := false
	cloned.PublishEnabled = &publish
	paused.RocketMQ = &cloned
	next, err := boundary.New(db, &paused, commandNativeRoutes{}, []string{app.CommandsTopic})
	require.NoError(t, err)
	t.Cleanup(func() { stop(next) })
	require.NoError(t, next.Runtime().Start(t.Context()))
	expected := transaction.Commit
	if fault == "before" {
		expected = transaction.Rollback
	}
	require.Eventually(t, func() bool {
		read, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		ev, e := authority.Evidence(read, reference)
		return e == nil && transaction.Resolve(reference, ev) == expected && (fault == "after" || next.Runtime().Stats().CheckRollback > 0)
	}, 100*time.Second, 100*time.Millisecond)
	if fault == "after" {
		select {
		case got := <-received:
			branch := winner.Execution.Plan.Input().Branches[0]
			require.Equal(t, branch.Message.Input().ID, got.ID)
			require.True(t, bytes.Equal(branch.Wire, got.Wire))
		case <-time.After(45 * time.Second):
			t.Fatal("committed original command missing")
		}
		committed, found, e := commands.Archive().ReadCommitted(t.Context(), request.RequestID)
		require.NoError(t, e)
		require.True(t, found)
		require.True(t, bytes.Equal(winner.Execution.Plan.Input().Branches[0].Wire, committed.Wire))
	} else {
		select {
		case <-received:
			t.Fatal("uncommitted original command visible")
		case <-time.After(2 * time.Second):
		}
	}
	require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_outbox"))
	nextStats := next.Runtime().Stats()
	require.Zero(t, nextStats.HalfPreparedResults)
	stop(next)
	closeConsumer()
	require.NoError(t, pool.PingContext(t.Context()))
	require.NoError(t, originalPool.PingContext(t.Context()))
	source, err := exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	cases := map[string]bool{"actual_command_original_root": true, "business_witness_match_boundary": true, "first_seal_plan_and_operation_reused": true, "broker_visibility_or_new_paused_checker": true, "new_publish_outbox_empty": true, "owned_runtimes_drained_primary_retained": true}
	raw, err := json.Marshal(map[string]any{"source_sha": strings.TrimSpace(string(source)), "boundary": fault, "scope": "actual QS command store original MySQL root with wire-level COMMIT faults; component only", "passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "seals": seals.Load(), "sdk_executions": shared.Runtime().Stats().CompletedResults, "half_prepared": shared.Runtime().Stats().HalfPreparedResults, "new_half_prepared": nextStats.HalfPreparedResults, "new_check_rollback": nextStats.CheckRollback, "business_rows": facts, "full_api_process_paired_provider_system_not_claimed": true})
	require.NoError(t, err)
	fmt.Println("M3_QS_COMMAND_FAULT_RECEIPT " + string(raw))
}
