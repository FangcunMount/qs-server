//go:build integration && m3_deferred_consumer

package messaging

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func nativeDeferredConfig(t *testing.T) *driver.Config {
	t.Helper()
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"), "owned isolated fixture required")
	cfg, err := driver.ParseDSN(os.Getenv("QS_M3_DEFERRED_DSN"))
	require.NoError(t, err)
	require.Equal(t, "tcp", cfg.Net)
	host, _, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback(), "native fixture must be loopback-only")
	cfg.ParseTime, cfg.MultiStatements, cfg.Loc = true, true, time.UTC
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["time_zone"] = "'+00:00'"
	return cfg
}

func nativeDeferredMessage(t *testing.T, routes []rmtransport.RouteBinding) rmtransport.Received {
	t.Helper()
	payload := []byte(`{"id":"event-native","eventType":"evaluation.retry.requested","data":{"org_id":7,"attempt_origin":"automatic","expected_attempt":2}}`)
	wire, err := legacy.Encode(legacy.Envelope{UUID: "event-native", Metadata: map[string]string{"event_type": eventcatalog.EvaluationRetryRequested}, Payload: payload}, legacy.Revision2)
	require.NoError(t, err)
	env, recognized, err := legacy.Decode(wire)
	require.NoError(t, err)
	require.True(t, recognized)
	return rmtransport.Received{Provider: "rocketmq", ID: env.UUID, TransportID: "broker-original", Topic: "qs.evaluation.lifecycle", Channel: "qs-worker", PhysicalTopic: routes[0].PhysicalTopic, Wire: wire, Metadata: env.Metadata, Payload: env.Payload, Attempts: 1, Timestamp: time.Now().UnixNano()}
}

type nativeDeferredFixture struct {
	pool    *sql.DB
	store   *DeferredDeliveryStore
	message rmtransport.Received
	config  *driver.Config
	cleanup func()
}

func nativeDeferredFixtureFor(t *testing.T, install bool) *nativeDeferredFixture {
	t.Helper()
	cfg := nativeDeferredConfig(t)
	cfg.DBName = ""
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	name := fmt.Sprintf("qs_m3_deferred_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4")
	require.NoError(t, err)
	cfg.DBName = name
	pool, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	pool.SetMaxOpenConns(4)
	pool.SetMaxIdleConns(2)
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			require.NoError(t, pool.Close())
			_, dropErr := admin.ExecContext(context.Background(), "DROP DATABASE `"+name+"`")
			require.NoError(t, dropErr)
			require.NoError(t, admin.Close())
		})
	}
	t.Cleanup(cleanup)
	files := []string{"000050_add_retry_event_hold.up.sql", "000104_rocketmq_transport_evidence.up.sql"}
	if install {
		files = append(files, "000109_rocketmq_deferred_delivery.up.sql")
	}
	for _, file := range files {
		body, readErr := os.ReadFile(filepath.Join("..", "..", "..", "pkg", "migration", "migrations", "mysql", file))
		require.NoError(t, readErr)
		_, err = pool.ExecContext(t.Context(), string(body))
		require.NoError(t, err)
	}
	routes := deferredRoutes(t)
	policy := retrygovernance.DefaultOutboxPolicy
	policy.BaseDelay, policy.MaxDelay, policy.JitterFraction = time.Millisecond, time.Millisecond, 0
	store, err := NewRocketMQDeferredStore(&mysqlRetryEventHoldStore{db: pool, provider: "rocketmq", policy: policy}, "qs-worker", 8, routes)
	require.NoError(t, err)
	return &nativeDeferredFixture{pool: pool, store: store, message: nativeDeferredMessage(t, routes), config: cfg, cleanup: cleanup}
}

func nativeDeferredHold(t *testing.T, f *nativeDeferredFixture) {
	t.Helper()
	require.NoError(t, f.store.HoldDelivery(t.Context(), f.message, eventcatalog.EvaluationRetryRequested, eventruntime.ErrAutomaticRetryPaused))
}

// This test uses the real original delivery row. It never publishes a new MQ
// message or treats a renamed Topic as permission to repeat business work.
func TestReadableDeferredHistoryNative(t *testing.T) {
	f := nativeDeferredFixtureFor(t, true)
	nativeDeferredHold(t, f)
	legacyRoutes := deferredRoutes(t)
	active := append([]rmtransport.RouteBinding(nil), legacyRoutes...)
	for i := range active {
		var err error
		active[i].PhysicalTopic, err = rmtransport.PhysicalTopicV3(active[i].Topic, active[i].MessageType, active[i].Owner, active[i].RouteRevision)
		require.NoError(t, err)
		active[i].ConsumerGroup, err = rmtransport.ConsumerGroupV3(active[i].Topic, active[i].Subscription)
		require.NoError(t, err)
		active[i].NamingScheme = rmtransport.ReadableNamingV1
	}
	resolver, err := rmtransport.NewRouteResolver(active, legacyRoutes)
	require.NoError(t, err)
	inputs, err := resolver.SubscriptionRoutes("qs.evaluation.lifecycle", "qs-worker")
	require.NoError(t, err)
	require.Len(t, inputs, 2)
	for _, row := range inputs {
		require.Equal(t, rmtransport.ReadableNamingV1, row.NamingScheme)
	}
	newStore := func(history []rmtransport.RouteBinding) *DeferredDeliveryStore {
		store, err := NewRocketMQDeferredStoreWithHistory(f.store.original, "qs-worker", 8, active, history)
		require.NoError(t, err)
		return store
	}
	// A missing history registration cannot rewrite the old transport identity
	// or consume one of its durable attempts when the real transaction aborts.
	item, err := newStore(nil).claim(t.Context(), time.Now().UTC(), time.Minute)
	require.Error(t, err)
	require.Nil(t, item)
	var attempts int
	require.NoError(t, f.pool.QueryRowContext(t.Context(), "SELECT replay_attempt_count FROM retry_event_hold").Scan(&attempts))
	require.Zero(t, attempts)
	store := newStore(legacyRoutes)
	require.NoError(t, store.Validate(t.Context()))
	require.NoError(t, store.HoldDelivery(t.Context(), f.message, eventcatalog.EvaluationRetryRequested, eventruntime.ErrAutomaticRetryPaused))
	item, err = store.claim(t.Context(), time.Now().UTC(), time.Minute)
	require.NoError(t, err)
	require.NotNil(t, item)
	require.Equal(t, f.message.TransportID, item.Message.TransportID)
	require.Equal(t, f.message.PhysicalTopic, item.Message.PhysicalTopic)
	require.Equal(t, f.message.ID, item.Message.ID)
	require.Equal(t, f.message.Wire, item.Message.Wire)
	require.Equal(t, f.message.Timestamp, item.Message.Timestamp)
	require.Equal(t, 1, item.ResumeCount)
	require.NoError(t, store.complete(t.Context(), item, time.Now().UTC()))
	remaining, err := store.claim(t.Context(), time.Now().UTC(), time.Minute)
	require.NoError(t, err)
	require.Nil(t, remaining)
	var rows int
	var replayed sql.NullTime
	require.NoError(t, f.pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM event_delivery_transport_evidence").Scan(&rows))
	require.Equal(t, 1, rows, "history was repackaged as a new physical delivery")
	require.NoError(t, f.pool.QueryRowContext(t.Context(), "SELECT replayed_at FROM retry_event_hold").Scan(&replayed))
	require.False(t, replayed.Valid, "database consumption must not claim MQ publication")
	f.cleanup()
	t.Log(`READABLE_DEFERRED_HISTORY_RECEIPT {"passed":4,"failed":0,"skipped":0,"cases":{"active_subscriptions_exclude_history":true,"missing_history_does_not_spend_attempt":true,"original_wire_and_transport_identity_retained":true,"completed_history_not_republished":true},"broker_not_claimed":true,"owned_database_dropped":true}`)
}

func TestRocketMQDeferredNative(t *testing.T) {
	cases := map[string]bool{}
	check := func(name string, run func(*testing.T)) {
		t.Helper()
		if t.Run(name, run) {
			cases[name] = true
		} else {
			t.FailNow()
		}
	}
	check("explicit_schema_required", func(t *testing.T) {
		f := nativeDeferredFixtureFor(t, false)
		require.Error(t, f.store.Validate(t.Context()))
		f.cleanup()
	})
	check("exact_wire_duplicate_and_legacy_refusal", func(t *testing.T) {
		f := nativeDeferredFixtureFor(t, true)
		require.NoError(t, f.store.Validate(t.Context()))
		nativeDeferredHold(t, f)
		nativeDeferredHold(t, f)
		var count int
		var wire []byte
		require.NoError(t, f.pool.QueryRowContext(t.Context(), "SELECT COUNT(*),MAX(original_wire) FROM event_delivery_transport_evidence").Scan(&count, &wire))
		require.Equal(t, 1, count)
		require.Equal(t, f.message.Wire, wire)
		item, err := f.store.claim(t.Context(), time.Now().UTC(), time.Minute)
		require.NoError(t, err)
		require.NotNil(t, item)
		nativeDeferredHold(t, f)
		var token, status string
		var attempts int
		require.NoError(t, f.pool.QueryRowContext(t.Context(), "SELECT status,claim_token,replay_attempt_count FROM retry_event_hold").Scan(&status, &token, &attempts))
		require.Equal(t, "consuming", status)
		require.Equal(t, item.Token, token)
		require.Equal(t, 1, attempts)
		require.NoError(t, f.store.complete(t.Context(), item, time.Now().UTC()))
		_, err = f.pool.ExecContext(t.Context(), "UPDATE retry_event_hold SET protocol_version=1")
		require.NoError(t, err)
		require.Error(t, f.store.HoldDelivery(t.Context(), f.message, eventcatalog.EvaluationRetryRequested, eventruntime.ErrAutomaticRetryPaused))
		f.cleanup()
	})
	check("broken_first_link_rolls_back_new_physical_evidence", func(t *testing.T) {
		f := nativeDeferredFixtureFor(t, true)
		nativeDeferredHold(t, f)
		_, err := f.pool.ExecContext(t.Context(), "UPDATE retry_event_hold SET transport_identity_hash=?", make([]byte, 32))
		require.NoError(t, err)
		duplicate := f.message
		duplicate.TransportID = "new-physical-receipt"
		require.Error(t, f.store.HoldDelivery(t.Context(), duplicate, eventcatalog.EvaluationRetryRequested, eventruntime.ErrAutomaticRetryPaused))
		var count int
		require.NoError(t, f.pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM event_delivery_transport_evidence WHERE transport_message_id=?", duplicate.TransportID).Scan(&count))
		require.Zero(t, count, "partial evidence escaped real rollback")
		f.cleanup()
	})
	check("real_expiry_new_claim_blocks_old_owner", func(t *testing.T) {
		f := nativeDeferredFixtureFor(t, true)
		nativeDeferredHold(t, f)
		first, err := f.store.claim(t.Context(), time.Now().UTC(), 100*time.Millisecond)
		require.NoError(t, err)
		require.NotNil(t, first)
		time.Sleep(150 * time.Millisecond)
		second, err := f.store.claim(t.Context(), time.Now().UTC(), time.Second)
		require.NoError(t, err)
		require.NotNil(t, second)
		require.NotEqual(t, first.Token, second.Token)
		require.Equal(t, 2, second.ResumeCount)
		require.Equal(t, f.message.Wire, second.Message.Wire)
		require.ErrorIs(t, f.store.renew(t.Context(), first, time.Now().UTC(), time.Second), ErrDeferredClaimLost)
		require.ErrorIs(t, f.store.complete(t.Context(), first, time.Now().UTC()), ErrDeferredClaimLost)
		require.NoError(t, f.store.complete(t.Context(), second, time.Now().UTC()))
		var consumed, replayed sql.NullTime
		require.NoError(t, f.pool.QueryRowContext(t.Context(), "SELECT consumed_at,replayed_at FROM retry_event_hold").Scan(&consumed, &replayed))
		require.True(t, consumed.Valid)
		require.False(t, replayed.Valid, "consumption must not claim publication")
		f.cleanup()
	})
	check("sql_lock_budget_does_not_grant_execution", func(t *testing.T) {
		f := nativeDeferredFixtureFor(t, true)
		nativeDeferredHold(t, f)
		lock, err := f.pool.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		defer func() { _ = lock.Rollback() }()
		var id int
		require.NoError(t, lock.QueryRowContext(t.Context(), "SELECT id FROM retry_event_hold FOR UPDATE").Scan(&id))
		busy, skipErr := f.store.claim(t.Context(), time.Now().UTC(), time.Minute)
		require.NoError(t, skipErr)
		require.Nil(t, busy, "locked responsibility was concurrently claimed")
		// SKIP LOCKED intentionally returns no eligible row. Lock the immutable
		// evidence instead to exercise the actual authority read's2s budget.
		require.NoError(t, lock.Rollback())
		lock, err = f.pool.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		defer func() { _ = lock.Rollback() }()
		_, err = lock.ExecContext(t.Context(), "UPDATE event_delivery_transport_evidence SET application_id=application_id")
		require.NoError(t, err)
		started := time.Now()
		item, err := f.store.claim(t.Context(), time.Now().UTC(), time.Minute)
		require.Error(t, err)
		require.Nil(t, item)
		require.GreaterOrEqual(t, time.Since(started), 1500*time.Millisecond)
		require.Less(t, time.Since(started), 4*time.Second)
		require.NoError(t, lock.Rollback())
		var attempts int
		require.NoError(t, f.pool.QueryRowContext(t.Context(), "SELECT replay_attempt_count FROM retry_event_hold").Scan(&attempts))
		require.Zero(t, attempts)
		f.cleanup()
	})
	check("durable_eight_attempt_budget_and_one_manual_delivery", func(t *testing.T) {
		f := nativeDeferredFixtureFor(t, true)
		nativeDeferredHold(t, f)
		var calls atomic.Int32
		options := DefaultDeferredConsumerOptions(time.Second)
		options.Interval = time.Millisecond
		dispatch := deferredDispatchFunc(func(context.Context, string, []byte) (eventruntime.DispatchResult, error) {
			calls.Add(1)
			return eventruntime.DispatchResult{}, errors.New("controlled callback failure, not network fault")
		})
		r, err := NewDeferredConsumer(f.store, dispatch, options)
		require.NoError(t, err)
		require.NoError(t, r.Start(context.Background()))
		deadline := time.Now().Add(5 * time.Second)
		var status string
		var count int
		for {
			require.NoError(t, f.pool.QueryRowContext(t.Context(), "SELECT retry_disposition,replay_attempt_count FROM retry_event_hold").Scan(&status, &count))
			if status == "manual_required" {
				break
			}
			require.True(t, time.Now().Before(deadline), "budget did not converge")
			time.Sleep(5 * time.Millisecond)
		}
		require.NoError(t, r.Close(t.Context()))
		require.EqualValues(t, 8, calls.Load())
		require.Equal(t, 8, count)
		_, err = f.pool.ExecContext(t.Context(), "UPDATE retry_event_hold SET retry_disposition='automatic',next_attempt_at=UTC_TIMESTAMP(3),manual_replay_request_id='fixture-technical-grant-1'")
		require.NoError(t, err)
		item, err := f.store.claim(t.Context(), time.Now().UTC(), time.Minute)
		require.NoError(t, err)
		require.NotNil(t, item)
		require.True(t, item.Manual)
		require.Equal(t, 9, item.ResumeCount)
		require.NoError(t, f.store.failed(t.Context(), item, time.Now().UTC(), deferredDispatchFailed))
		_, err = f.pool.ExecContext(t.Context(), "UPDATE retry_event_hold SET retry_disposition='automatic',next_attempt_at=UTC_TIMESTAMP(3)")
		require.NoError(t, err)
		item, err = f.store.claim(t.Context(), time.Now().UTC(), time.Minute)
		require.NoError(t, err)
		require.Nil(t, item, "same technical grant authorized another call")
		require.EqualValues(t, 8, calls.Load(), "store fixture itself must not execute business")
		f.cleanup()
	})
	check("real_child_kill_retains_attempt_and_frozen_wire", func(t *testing.T) {
		f := nativeDeferredFixtureFor(t, true)
		nativeDeferredHold(t, f)
		directory := t.TempDir()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDeferredNativeChild$")
		cmd.Env = append(os.Environ(), "QS_M3_DEFERRED_CHILD=1", "QS_M3_DEFERRED_DSN="+f.config.FormatDSN(), "QS_M3_DEFERRED_CHILD_DIR="+directory)
		log, err := os.OpenFile(filepath.Join(directory, "child.log"), os.O_CREATE|os.O_WRONLY, 0600)
		require.NoError(t, err)
		defer func() { _ = log.Close() }()
		cmd.Stdout, cmd.Stderr = log, log
		require.NoError(t, cmd.Start())
		reaped := false
		defer func() {
			if !reaped {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		}()
		for {
			if _, err = os.Stat(filepath.Join(directory, "entered")); err == nil {
				break
			}
			require.NoError(t, ctx.Err(), "native child did not enter callback")
			time.Sleep(5 * time.Millisecond)
		}
		require.NoError(t, cmd.Process.Kill())
		require.Error(t, cmd.Wait())
		reaped = true
		var expires time.Time
		var count int
		require.NoError(t, f.pool.QueryRowContext(t.Context(), "SELECT replay_attempt_count,claim_expires_at FROM retry_event_hold").Scan(&count, &expires))
		require.Equal(t, 1, count)
		if delay := time.Until(expires.Add(30 * time.Millisecond)); delay > 0 {
			time.Sleep(delay)
		}
		item, err := f.store.claim(t.Context(), time.Now().UTC(), time.Minute)
		require.NoError(t, err)
		require.NotNil(t, item)
		require.Equal(t, 2, item.ResumeCount)
		require.Equal(t, f.message.Wire, item.Message.Wire)
		require.Equal(t, f.message.Payload, item.Message.Payload)
		require.NoError(t, f.store.complete(t.Context(), item, time.Now().UTC()))
		require.NoError(t, log.Close())
		require.NoError(t, os.RemoveAll(directory))
		f.cleanup()
	})
	git, err := exec.Command("git", "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	receipt := map[string]any{"source_sha": strings.TrimSpace(string(git)), "passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "scope": "original MySQL deferred consumption, real lease/lock/process boundaries; controlled callback failures", "broker_not_claimed": true, "whole_worker_not_claimed": true, "system_login_not_claimed": true, "human_authorization_api_not_claimed": true, "owned_databases_dropped": true, "owned_child_reaped": true}
	body, err := json.Marshal(receipt)
	require.NoError(t, err)
	t.Log("M3_QS_DEFERRED_MYSQL_RECEIPT " + string(body))
}

func TestDeferredNativeChild(t *testing.T) {
	if os.Getenv("QS_M3_DEFERRED_CHILD") != "1" {
		return
	}
	cfg := nativeDeferredConfig(t)
	require.True(t, strings.HasPrefix(cfg.DBName, "qs_m3_deferred_"))
	pool, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	pool.SetMaxOpenConns(4)
	store, err := NewRocketMQDeferredStore(&mysqlRetryEventHoldStore{db: pool, provider: "rocketmq", policy: retrygovernance.DefaultOutboxPolicy}, "qs-worker", 8, deferredRoutes(t))
	require.NoError(t, err)
	options := DefaultDeferredConsumerOptions(time.Minute)
	options.Lease = 300 * time.Millisecond
	options.RenewEvery = 50 * time.Millisecond
	r, err := NewDeferredConsumer(store, deferredDispatchFunc(func(context.Context, string, []byte) (eventruntime.DispatchResult, error) {
		if err := os.WriteFile(filepath.Join(os.Getenv("QS_M3_DEFERRED_CHILD_DIR"), "entered"), []byte("owned callback entered\n"), 0600); err != nil {
			return eventruntime.DispatchResult{}, err
		}
		select {}
	}), options)
	require.NoError(t, err)
	require.NoError(t, r.Start(context.Background()))
	select {}
}
