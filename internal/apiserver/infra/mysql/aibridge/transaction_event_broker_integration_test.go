//go:build integration && m3_ai_event_broker

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
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	boundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	qsoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	jose "github.com/go-jose/go-jose/v4"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type eventNativeRoutes struct{}

func (eventNativeRoutes) GetTopicForEvent(string) (string, bool) { return app.AcksTopic, true }

func TestM3AIEventBrokerHost(t *testing.T) {
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	cfg, err := driver.ParseDSN(os.Getenv("QS_M3_EVENT_DSN"))
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
	name := fmt.Sprintf("qs_m3_ai_event_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.ExecContext(context.Background(), "DROP DATABASE `"+name+"`")
		require.NoError(t, err)
	})
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	for _, file := range []string{"000072_ai_bridge_delivery.up.sql", "000083_ai_runtime_index.up.sql", "000091_ai_messaging.up.sql", "000092_ai_messaging_failures.up.sql", "000094_ai_messaging_admission.up.sql", "000095_ai_messaging_observations.up.sql", "000097_ai_command_retirement.up.sql", "000106_transaction_messaging.up.sql", "000110_native_transaction_limit_notices.up.sql", "000108_ai_transaction_command_order.up.sql"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "pkg", "migration", "migrations", "mysql", file))
		require.NoError(t, err)
		_, err = pool.ExecContext(t.Context(), string(raw))
		require.NoError(t, err)
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
	producer, observer := load("QS_M3_PRODUCER_CONFIG"), load("QS_M3_OBSERVER_CONFIG")
	bind := producer.RocketMQ.Transactions["qs.mysql"]
	_, err = pool.ExecContext(t.Context(), "INSERT INTO rm_tx_binding VALUES(?,?,?,1)", []byte("qs.mysql"), []byte(bind.StoreID), []byte(bind.WriterEpoch))
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), "UPDATE ai_messaging_admission SET closed=FALSE WHERE singleton=1")
	require.NoError(t, err)
	key := func(id string) jose.JSONWebKey {
		value, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		return jose.JSONWebKey{Key: value, KeyID: id}
	}
	qsSign, qsCrypt, aiSign, aiCrypt := key("qs-sign"), key("qs-recipient"), key("ai-sign"), key("ai-recipient")
	var seals atomic.Int32
	seal := func(kind pb.MessagingKind, id, aggregate, org, original string, body *pb.MessagingBody) (*app.PreparedMessaging, error) {
		seals.Add(1)
		if original == "" {
			original = time.Now().Format(time.RFC3339Nano)
		}
		return app.ProtectMessaging(kind, id, aggregate, "", org, original, body, qsSign, aiCrypt.Public())
	}
	count := func(query string, args ...any) int {
		var n int
		require.NoError(t, pool.QueryRowContext(t.Context(), query, args...).Scan(&n))
		return n
	}
	before := count("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", name)
	shared, err := boundary.New(db, &producer, eventNativeRoutes{}, []string{app.CommandsTopic, app.AcksTopic})
	require.NoError(t, err)
	legacyStore := NewMessagingStore()
	events, err := NewTransactionEvents(db, shared, legacyStore, seal, bind)
	require.NoError(t, err)
	require.Nil(t, legacyStore.Archive, "constructor must not mutate the caller's historical store")
	require.Equal(t, before, count("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", name))
	commands, err := NewTransactionCommands(db, shared, seal)
	require.NoError(t, err)
	require.NoError(t, commands.Validate(t.Context()))
	require.NoError(t, shared.Runtime().Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, shared.Runtime().Close(ctx))
	})
	require.NoError(t, shared.Runtime().SetAdmission(true))
	consumerConfig, err := observer.RocketMQ.Load([]string{app.CommandsTopic, app.AcksTopic}, 1, 5*time.Second)
	require.NoError(t, err)
	consumer, err := rocketmq.NewSubscriber(rocketmq.SubscriberConfig{Config: consumerConfig, MaxAttempts: 3, InvisibleDuration: 15 * time.Second, HandlerTimeout: 5 * time.Second, RetryDelay: time.Second})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		require.NoError(t, consumer.Close(ctx))
	})
	received := make(chan transport.Received, 32)
	for topic, subscription := range map[string]string{app.CommandsTopic: "qs-ai.commands.v1", app.AcksTopic: "qs-ai.acks.v1"} {
		require.NoError(t, consumer.Subscribe(t.Context(), topic, subscription, func(_ context.Context, d transport.Delivery) error {
			message := d.Message()
			if err := d.Ack(); err != nil {
				return err
			}
			select {
			case received <- message:
				return nil
			default:
				return errors.New("bounded fixture queue full")
			}
		}, func(context.Context, transport.Failure) error { return errors.New("fixture delivery failed") }))
	}
	await := func(id string) transport.Received {
		deadline := time.NewTimer(20 * time.Second)
		defer deadline.Stop()
		for {
			select {
			case message := <-received:
				if message.ID == id {
					return message
				}
			case <-deadline.C:
				t.Fatal("committed message not observed")
				return transport.Received{}
			}
		}
	}
	ring := protected.Keyring{Decrypt: map[string]jose.JSONWebKey{qsCrypt.KeyID: qsCrypt}, Signers: map[string]protected.TrustedSigner{aiSign.KeyID: {Producer: "qs-ai", Key: aiSign.Public()}}}
	receiver := &MessagingEventReceiver{DB: pool, Store: legacyStore, Bodies: &localEventBody{}, Seal: seal, Keys: ring, Transactions: events}
	ai := func(kind pb.MessagingKind, id, aggregate string, body *pb.MessagingBody) *app.PreparedMessaging {
		correlation := ""
		if kind == pb.MessagingKind_COMMAND_RECEIPT {
			correlation = body.GetCommandReceipt().GetCommandId()
		}
		m, err := app.ProtectMessaging(kind, id, aggregate, correlation, "7", time.Now().Format(time.RFC3339Nano), body, aiSign, qsCrypt.Public())
		require.NoError(t, err)
		return m
	}
	state := func() *app.PreparedMessaging {
		id, run := uuid.NewString(), uuid.NewString()
		return ai(pb.MessagingKind_EVALUATION_STATE, id, run, &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationState{EvaluationState: &pb.EvaluationRuntimeState{RunId: run, OrganizationId: "7", EventSequence: 1, Version: 1, Status: "requested"}}})
	}
	lookup := func(m *app.PreparedMessaging) (app.FrozenIncomingEvent, transaction.Reservation, app.FrozenMessagingArchive) {
		incoming, err := app.FreezeIncomingEvent(m.Envelope, m.Body, m.Wire, "7")
		require.NoError(t, err)
		winner, err := events.archive.store.Lookup(t.Context(), incoming.Request())
		require.NoError(t, err)
		ack, err := incoming.ReadACK(winner.Execution.Plan)
		require.NoError(t, err)
		return incoming, winner, ack
	}
	cases := map[string]bool{"constructor_no_ddl": true}
	first := state()
	_, err = pool.ExecContext(t.Context(), "INSERT INTO ai_messaging_evaluation_states(run_id,organization_id,event_sequence,version,state,updated_at) VALUES(?,7,0,0,?,UTC_TIMESTAMP(6))", first.Envelope.AggregateKey, []byte{})
	require.NoError(t, err)
	blocker, err := pool.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocker.Rollback() })
	var version int
	require.NoError(t, blocker.QueryRowContext(t.Context(), "SELECT version FROM ai_messaging_evaluation_states WHERE run_id=? FOR UPDATE", first.Envelope.AggregateKey).Scan(&version))
	done := make(chan error, 1)
	go func() { done <- receiver.Receive(t.Context(), first.Wire) }()
	require.Eventually(t, func() bool { return count("SELECT COUNT(*) FROM rm_tx_witness WHERE half_outcome='prepared'") == 1 }, 15*time.Second, 20*time.Millisecond)
	_, firstWinner, firstACK := lookup(first)
	_, found, payloadErr := events.archive.Payload(t.Context(), &pb.MessagePayloadReference{Producer: "qs-server", Destination: "qs-ai", MessageId: firstACK.Envelope.MessageId, OrganizationId: "7", BodySha256: firstACK.Envelope.BodySha256, BodyLength: uint64(len(firstACK.Body))}, "qs-ai")
	require.True(t, found)
	require.ErrorIs(t, payloadErr, app.ErrNotFound)
	cases["prepared_ack_payload_not_exposed"] = true
	require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_archive"))
	require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_inbox"))
	select {
	case <-received:
		t.Fatal("half ACK visible before original root commit")
	case <-time.After(250 * time.Millisecond):
	}
	require.NoError(t, blocker.Commit())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("root did not finish")
	}
	message := await(firstACK.Envelope.MessageId)
	require.Equal(t, firstACK.Wire, message.Wire)
	ackHeader, err := app.AuthenticateMessaging(message.Wire, app.AcksTopic, protected.Keyring{Decrypt: map[string]jose.JSONWebKey{aiCrypt.KeyID: aiCrypt}, Signers: map[string]protected.TrustedSigner{qsSign.KeyID: {Producer: "qs-server", Key: qsSign.Public()}}})
	require.NoError(t, err)
	require.Equal(t, firstACK.Envelope.MessageId, ackHeader.MessageId)
	require.Equal(t, 1, count("SELECT COUNT(*) FROM ai_messaging_evaluation_states WHERE run_id=? AND version=1", first.Envelope.AggregateKey))
	require.Equal(t, 1, count("SELECT COUNT(*) FROM ai_messaging_inbox WHERE message_id=? AND ack_id=? AND outcome='stored'", first.Envelope.MessageId, firstACK.Envelope.MessageId))
	cases["half_invisible_before_root_commit"], cases["inbox_projection_archive_witness_atomic"], cases["first_ack_wire_authenticated"] = true, true, true
	beforeSeals := seals.Load()
	require.NoError(t, receiver.Receive(t.Context(), first.Wire))
	require.Equal(t, beforeSeals, seals.Load())
	_, duplicate, duplicateACK := lookup(first)
	require.Equal(t, firstWinner.Execution.Plan.Digest(), duplicate.Execution.Plan.Digest())
	require.Equal(t, firstACK.Wire, duplicateACK.Wire)
	require.Equal(t, 1, count("SELECT COUNT(*) FROM ai_messaging_inbox WHERE message_id=?", first.Envelope.MessageId))
	cases["committed_duplicate_reuses_first_ack"] = true
	changed := ai(first.Envelope.Kind, first.Envelope.MessageId, first.Envelope.AggregateKey, &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationState{EvaluationState: &pb.EvaluationRuntimeState{RunId: first.Envelope.AggregateKey, OrganizationId: "7", EventSequence: 2, Version: 2, Status: "running"}}})
	require.NoError(t, receiver.Receive(t.Context(), changed.Wire)) // isolated, not applied
	require.Equal(t, beforeSeals, seals.Load())
	require.Equal(t, 1, count("SELECT COUNT(*) FROM ai_messaging_quarantine WHERE wire_sha256=?", messagingHash(changed.Wire)))
	cases["conflicting_wire_isolated"] = true
	commandStore := &MessagingCommandStore{Store: &Store{DB: pool}, Messaging: legacyStore, Seal: seal, Transactions: commands}
	request, session := uuid.NewString(), uuid.NewString()
	require.NoError(t, commandStore.StageStart(t.Context(), app.Start{RequestID: request, Actor: app.Actor{OrgID: "7", SubjectID: "9"}, TesteeID: "19", AssessmentIDs: []string{"21"}, Goal: "frozen native interpretation"}))
	await(request)
	eventID := uuid.NewString()
	interpretation := ai(pb.MessagingKind_INTERPRETATION_STATE, eventID, request, &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: &pb.StateEvent{EventId: eventID, RequestId: request, SessionId: session, Actor: &pb.Actor{OrgId: "7", SubjectId: "9"}, TesteeId: "19", Version: 1, Status: "running"}}})
	require.NoError(t, receiver.Receive(t.Context(), interpretation.Wire))
	_, _, interpretationACK := lookup(interpretation)
	await(interpretationACK.Envelope.MessageId)
	require.Equal(t, 1, count("SELECT COUNT(*) FROM ai_bridge_events WHERE event_id=?", eventID))
	cases["interpretation_projection_original_root"] = true
	for _, historical := range []bool{false, true} {
		current := commandStore
		if historical {
			current = &MessagingCommandStore{Store: &Store{DB: pool}, Messaging: legacyStore, Seal: seal}
		}
		run, id := uuid.NewString(), uuid.NewString()
		require.NoError(t, current.SubmitEvaluationStart(t.Context(), app.EvaluationScope{OrganizationID: 7, OperatorUserID: 9, RunID: run}, id, app.EvaluationStart{ExpectedVersion: 1, Confirm: true}))
		var hash string
		if historical {
			require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT body_sha256 FROM ai_messaging_outbox WHERE message_id=?", id).Scan(&hash))
		} else {
			command := await(id)
			require.NotEmpty(t, command.Wire)
			archive, found, err := commands.Archive().ReadCommitted(t.Context(), id)
			require.NoError(t, err)
			require.True(t, found)
			hash = archive.Envelope.BodySha256
		}
		receipt := ai(pb.MessagingKind_COMMAND_RECEIPT, uuid.NewString(), run, &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: &pb.MessagingCommandReceipt{CommandId: id, CommandBodySha256: hash, Decision: pb.MessagingDecision_ACCEPTED, OriginalReceipt: &pb.MessagingCommandReceipt_EvaluationReceipt{EvaluationReceipt: &pb.EvaluationState{RunId: run, Version: 1}}}}})
		require.NoError(t, receiver.Receive(t.Context(), receipt.Wire))
		_, _, ack := lookup(receipt)
		await(ack.Envelope.MessageId)
		require.Equal(t, 1, count("SELECT COUNT(*) FROM ai_messaging_operations WHERE command_id=? AND decision='accepted'", id))
		if historical {
			require.Equal(t, 1, count("SELECT COUNT(*) FROM ai_messaging_outbox WHERE message_id=? AND stage='confirmed'", id))
			cases["legacy_command_receipt_compatible"] = true
		} else {
			require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_outbox WHERE message_id=?", id))
			cases["new_command_receipt_without_outbox"] = true
		}
	}
	// Reject on the real SQL server AFTER Inbox/projection/archive writes, before witness commit.
	reject := func() {
		_, err := pool.ExecContext(t.Context(), "CREATE TRIGGER fixture_event_reject BEFORE UPDATE ON rm_tx_witness FOR EACH ROW BEGIN IF NEW.state='committed' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='controlled fixture witness rejection'; END IF; END")
		require.NoError(t, err)
	}
	unreject := func() {
		_, err := pool.ExecContext(t.Context(), "DROP TRIGGER fixture_event_reject")
		require.NoError(t, err)
	}
	reject()
	aborted := state()
	require.Error(t, receiver.Receive(t.Context(), aborted.Wire))
	_, old, oldACK := lookup(aborted)
	require.Equal(t, transaction.LocalAborted, events.confirmEvent(t.Context(), mustIncomingEvent(t, aborted), old.Execution))
	require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_inbox WHERE message_id=?", aborted.Envelope.MessageId))
	require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_evaluation_states WHERE run_id=?", aborted.Envelope.AggregateKey))
	require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_archive WHERE message_id=?", []byte(oldACK.Envelope.MessageId)))
	unreject()
	beforeSeals = seals.Load()
	require.NoError(t, receiver.Receive(t.Context(), aborted.Wire))
	_, retried, retriedACK := lookup(aborted)
	require.Greater(t, retried.Execution.Fence, old.Execution.Fence)
	require.Equal(t, oldACK.Wire, retriedACK.Wire)
	require.Equal(t, beforeSeals, seals.Load())
	await(retriedACK.Envelope.MessageId)
	require.Equal(t, transaction.LocalAborted, events.confirmEvent(t.Context(), mustIncomingEvent(t, aborted), old.Execution))
	cases["server_rejection_rolls_back_root"], cases["aborted_delivery_explicit_retry"] = true, true
	reject()
	held := state()
	require.Error(t, receiver.Receive(t.Context(), held.Wire))
	_, beforeHeld, beforeHeldACK := lookup(held)
	unreject()
	budget, err := pool.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	for i := 0; i < 8; i++ {
		_, err = legacyStore.RecordTechnicalFailure(t.Context(), budget, held.Envelope, held.Wire)
		require.NoError(t, err)
	}
	require.NoError(t, budget.Commit())
	require.NoError(t, receiver.Receive(t.Context(), held.Wire))
	_, heldWinner, heldACK := lookup(held)
	require.NotEqual(t, beforeHeld.Execution.Plan.Input().OperationID, heldWinner.Execution.Plan.Input().OperationID)
	require.False(t, bytes.Equal(beforeHeldACK.Wire, heldACK.Wire))
	await(heldACK.Envelope.MessageId)
	require.Equal(t, 1, count("SELECT COUNT(*) FROM ai_messaging_inbox WHERE message_id=? AND outcome='held'", held.Envelope.MessageId))
	require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_evaluation_states WHERE run_id=?", held.Envelope.AggregateKey))
	require.Equal(t, transaction.LocalAborted, events.confirmEvent(t.Context(), mustIncomingEvent(t, held), beforeHeld.Execution))
	cases["technical_hold_without_projection"], cases["aborted_disposition_explicit_replan"] = true, true
	legacy := state()
	oldReceiver := *receiver
	oldReceiver.Transactions = nil
	require.NoError(t, oldReceiver.Receive(t.Context(), legacy.Wire))
	beforeSeals = seals.Load()
	require.ErrorIs(t, receiver.Receive(t.Context(), legacy.Wire), ErrLegacyEventResponsibility)
	require.Equal(t, beforeSeals, seals.Load())
	require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_failures WHERE message_id=?", legacy.Envelope.MessageId))
	cases["nonempty_legacy_inbox_not_adopted"] = true
	require.Equal(t, 1, count("SELECT COUNT(*) FROM ai_messaging_outbox WHERE kind=?", pb.MessagingKind_EVENT_ACKNOWLEDGEMENT)) // exactly the legacy fixture
	cases["no_new_ack_outbox"] = true
	fresh, err := NewTransactionEvents(db, shared, legacyStore, seal, bind)
	require.NoError(t, err)
	require.Equal(t, transaction.LocalCommitted, fresh.confirmEvent(t.Context(), mustIncomingEvent(t, first), firstWinner.Execution))
	proof, err := fresh.archive.store.Evidence(t.Context(), firstWinner.Execution.References()[0])
	require.NoError(t, err)
	require.Equal(t, transaction.Commit, transaction.Resolve(firstWinner.Execution.References()[0], proof))
	cases["fresh_adapter_exact_witness"] = true
	require.Len(t, cases, 17)
	receipt, err := json.Marshal(map[string]any{"passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "root": "original_mysql_sql", "system_login_not_claimed": true, "paired_ai_producer_not_claimed": true, "process_exit_not_claimed": true, "database_commit_network_fault_not_claimed": true})
	require.NoError(t, err)
	fmt.Println("M3_QS_AI_EVENT_RECEIPT " + string(receipt))
}

func mustIncomingEvent(t *testing.T, m *app.PreparedMessaging) app.FrozenIncomingEvent {
	t.Helper()
	incoming, err := app.FreezeIncomingEvent(m.Envelope, m.Body, m.Wire, "7")
	require.NoError(t, err)
	return incoming
}
