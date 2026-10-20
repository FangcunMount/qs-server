//go:build integration && m3_ai_command_broker

package aibridge

import (
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
	"google.golang.org/protobuf/proto"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type commandNativeRoutes struct{}

func (commandNativeRoutes) GetTopicForEvent(string) (string, bool) { return app.CommandsTopic, true }

func TestM3AICommandBrokerHost(t *testing.T) {
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
	name := fmt.Sprintf("qs_m3_ai_command_%d", time.Now().UnixNano())
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
	install := func(file string) {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "pkg", "migration", "migrations", "mysql", file))
		require.NoError(t, err)
		_, err = pool.ExecContext(t.Context(), string(raw))
		require.NoError(t, err)
	}
	for _, file := range []string{"000072_ai_bridge_delivery.up.sql", "000083_ai_runtime_index.up.sql", "000091_ai_messaging.up.sql", "000094_ai_messaging_admission.up.sql", "000097_ai_command_retirement.up.sql", "000106_transaction_messaging.up.sql"} {
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
	before := count("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", name)
	shared, err := boundary.New(db, &producerOptions, commandNativeRoutes{}, []string{app.CommandsTopic})
	require.NoError(t, err)
	commands, err := NewTransactionCommands(db, shared, seal)
	require.NoError(t, err)
	require.Error(t, commands.Validate(t.Context()), "missing explicit order installation must refuse startup")
	require.Equal(t, before, count("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", name))
	install("000108_ai_transaction_command_order.up.sql")
	require.NoError(t, commands.Validate(t.Context()))
	require.False(t, shared.Runtime().Stats().Ready)
	require.NoError(t, shared.Runtime().Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, shared.Runtime().Close(ctx))
	})
	require.NoError(t, shared.Runtime().SetAdmission(true))
	consumerConfig, err := observerOptions.RocketMQ.Load([]string{app.CommandsTopic}, 1, 5*time.Second)
	require.NoError(t, err)
	consumer, err := rocketmq.NewSubscriber(rocketmq.SubscriberConfig{Config: consumerConfig, MaxAttempts: 3, InvisibleDuration: 15 * time.Second, HandlerTimeout: 5 * time.Second, RetryDelay: time.Second})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		require.NoError(t, consumer.Close(ctx))
	})
	received := make(chan transport.Received, 16)
	require.NoError(t, consumer.Subscribe(t.Context(), app.CommandsTopic, "qs-m3-ai-command-observer", func(_ context.Context, d transport.Delivery) error {
		message := d.Message()
		if err := d.Ack(); err != nil {
			return err
		}
		select {
		case received <- message:
			return nil
		default:
			return errors.New("bounded fixture observer queue full")
		}
	}, func(context.Context, transport.Failure) error { return errors.New("fixture command delivery failed") }))
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
				t.Fatal("committed command not observed")
				return transport.Received{}
			}
		}
	}
	store := &MessagingCommandStore{Store: &Store{DB: pool}, Messaging: NewMessagingStore(), Seal: seal, Transactions: commands}
	r := app.Start{RequestID: uuid.NewString(), Actor: app.Actor{OrgID: "7", SubjectID: "9"}, TesteeID: "19", AssessmentIDs: []string{"21"}, Goal: "冻结输入：中文🙂"}
	_, err = pool.ExecContext(t.Context(), "UPDATE ai_messaging_admission SET closed=FALSE WHERE singleton=1")
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), "INSERT INTO ai_messaging_aggregates VALUES(?,1)", r.RequestID)
	require.NoError(t, err)
	blocker, err := pool.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocker.Rollback() })
	var sequence uint64
	require.NoError(t, blocker.QueryRowContext(t.Context(), "SELECT next_sequence FROM ai_messaging_aggregates WHERE aggregate_key=? FOR UPDATE", r.RequestID).Scan(&sequence))
	done := make(chan error, 1)
	go func() { done <- store.StageStart(t.Context(), r) }()
	require.Eventually(t, func() bool {
		return count("SELECT COUNT(*) FROM rm_tx_witness WHERE half_outcome='prepared'") == 1
	}, 15*time.Second, 20*time.Millisecond)
	require.Zero(t, count("SELECT COUNT(*) FROM ai_bridge_requests WHERE request_id=?", r.RequestID))
	require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_archive"))
	require.ErrorIs(t, store.StageStart(t.Context(), r), ErrCommandUnresolved)
	require.EqualValues(t, 1, seals.Load(), "in-flight duplicate must not Seal or execute again")
	pending, err := store.ReadRequestOperation(t.Context(), app.OperationScope{OrganizationID: "7", SubjectID: "9"}, r.RequestID, r.RequestID)
	require.NoError(t, err)
	require.Equal(t, "pending", pending.Status)
	require.Equal(t, "transaction_pending", pending.TransportStatus)
	pendingCommand, err := app.NewTransactionCommand(pb.MessagingKind_START, r.RequestID, r.RequestID, app.OperationScope{OrganizationID: "7", SubjectID: "9", ResourceID: r.RequestID}, startMessagingBody(r))
	require.NoError(t, err)
	pendingWinner, err := commands.archive.store.Lookup(t.Context(), transaction.Request{Owner: "qs.mysql", Scope: "org:7", Key: pendingCommand.Key(), InputHash: pendingCommand.InputHash()})
	require.NoError(t, err)
	_, pendingArchive, err := app.ReadTransactionCommand(pendingWinner.Execution.Plan)
	require.NoError(t, err)
	_, found, err := commands.archive.Payload(t.Context(), &pb.MessagePayloadReference{Producer: "qs-server", Destination: "qs-ai", MessageId: r.RequestID, OrganizationId: "7", BodySha256: pendingArchive.Envelope.BodySha256, BodyLength: uint64(len(pendingArchive.Body))}, "qs-ai")
	require.True(t, found)
	require.ErrorIs(t, err, app.ErrNotFound)
	select {
	case <-received:
		t.Fatal("half message was consumable before original business commit")
	case <-time.After(250 * time.Millisecond):
	}
	require.NoError(t, blocker.Commit())
	require.NoError(t, <-done)
	first := await(r.RequestID)
	a, found, err := commands.Archive().ReadCommitted(t.Context(), r.RequestID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, first.Wire, a.Wire)
	require.EqualValues(t, 1, a.Sequence)
	header, err := app.AuthenticateMessaging(first.Wire, app.CommandsTopic, protected.Keyring{Decrypt: map[string]jose.JSONWebKey{recipient.KeyID: recipient}, Signers: map[string]protected.TrustedSigner{signing.KeyID: {Producer: "qs-server", Key: signing.Public()}}})
	require.NoError(t, err)
	require.Equal(t, r.RequestID, header.MessageId)
	require.NoError(t, store.StageStart(t.Context(), r))
	require.EqualValues(t, 1, seals.Load())
	changed := r
	changed.Goal = "changed business input"
	require.ErrorIs(t, store.StageStart(t.Context(), changed), app.ErrConflict)
	require.EqualValues(t, 1, seals.Load())
	op, err := store.ReadRequestOperation(t.Context(), app.OperationScope{OrganizationID: "7", SubjectID: "9"}, r.RequestID, r.RequestID)
	require.NoError(t, err)
	require.Equal(t, "submitted", op.Status)
	require.Equal(t, "awaiting_receipt", op.TransportStatus)
	_, err = store.ReadOperation(t.Context(), app.OperationScope{OrganizationID: "8", SubjectID: "9"}, r.RequestID)
	require.ErrorIs(t, err, app.ErrNotFound)
	payload, found, err := commands.Archive().Payload(t.Context(), &pb.MessagePayloadReference{Producer: "qs-server", Destination: "qs-ai", MessageId: r.RequestID, OrganizationId: "7", BodySha256: a.Envelope.BodySha256, BodyLength: uint64(len(a.Body))}, "qs-ai")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, a.Body, payload)
	fresh, err := NewTransactionArchive(pool, commands.archive.store.Binding())
	require.NoError(t, err)
	_, found, err = fresh.ReadCommitted(t.Context(), r.RequestID)
	require.NoError(t, err)
	require.True(t, found)
	session := uuid.NewString()
	change := app.Change{CommandID: uuid.NewString(), SessionID: session, Actor: r.Actor, Action: "cancel", ExpectedVersion: 1}
	require.ErrorIs(t, store.StageChange(t.Context(), r.RequestID, change), ErrCommandPredecessorPending)
	require.EqualValues(t, 1, seals.Load())
	// Controlled matching receipts stand in for AI admission only in this host
	// fixture. They are not evidence of the paired AI receiver or business ACK.
	receipt := func(id string, decision pb.MessagingDecision) {
		archive, found, err := commands.Archive().ReadCommitted(t.Context(), id)
		require.NoError(t, err)
		require.True(t, found)
		body := &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: &pb.MessagingCommandReceipt{CommandId: id, CommandBodySha256: archive.Envelope.BodySha256, Decision: decision, OriginalReceipt: &pb.MessagingCommandReceipt_WorkflowReceipt{WorkflowReceipt: &pb.Receipt{SessionId: session, Version: 1, Status: "running"}}}}}
		raw, err := proto.Marshal(body)
		require.NoError(t, err)
		state := "accepted"
		if decision == pb.MessagingDecision_HELD {
			state = "held"
		}
		tx, err := pool.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(t.Context(), "UPDATE ai_messaging_operations SET decision=?,receipt_id=?,receipt=?,decided_at=UTC_TIMESTAMP(6) WHERE command_id=?", state, uuid.NewString(), raw, id)
		require.NoError(t, err)
		_, err = tx.ExecContext(t.Context(), "UPDATE ai_bridge_requests SET session_id=? WHERE request_id=?", session, r.RequestID)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
	}
	receipt(r.RequestID, pb.MessagingDecision_ACCEPTED)
	require.NoError(t, store.StageChange(t.Context(), r.RequestID, change))
	await(change.CommandID)
	receipt(change.CommandID, pb.MessagingDecision_HELD)
	beforeRetry := seals.Load()
	require.ErrorIs(t, store.SubmitParticipantRetry(t.Context(), app.DraftScope{OrganizationID: 7, OperatorUserID: 9}, session, app.ParticipantRetry{CommandID: uuid.NewString(), Confirm: true}), ErrCommandPredecessorPending)
	require.Equal(t, beforeRetry, seals.Load())
	receipt(change.CommandID, pb.MessagingDecision_ACCEPTED)
	retryID := uuid.NewString()
	require.NoError(t, store.SubmitParticipantRetry(t.Context(), app.DraftScope{OrganizationID: 7, OperatorUserID: 9}, session, app.ParticipantRetry{CommandID: retryID, Confirm: true}))
	await(retryID)
	run := app.EvaluationScope{OrganizationID: 7, OperatorUserID: 9, RunID: uuid.NewString()}
	startID := uuid.NewString()
	require.NoError(t, store.SubmitEvaluationStart(t.Context(), run, startID, app.EvaluationStart{ExpectedVersion: 1, Confirm: true}))
	await(startID)
	// Evaluation CANCEL must also preserve the run's real admission ordering.
	cancelID := uuid.NewString()
	require.ErrorIs(t, store.SubmitEvaluationCancel(t.Context(), run, cancelID, app.EvaluationCancel{ExpectedVersion: 1, Confirm: true}), ErrCommandPredecessorPending)
	evaluationArchive, found, err := commands.Archive().ReadCommitted(t.Context(), startID)
	require.NoError(t, err)
	require.True(t, found)
	evaluationReceipt, err := proto.Marshal(&pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: &pb.MessagingCommandReceipt{CommandId: startID, CommandBodySha256: evaluationArchive.Envelope.BodySha256, Decision: pb.MessagingDecision_ACCEPTED, OriginalReceipt: &pb.MessagingCommandReceipt_EvaluationReceipt{EvaluationReceipt: &pb.EvaluationState{RunId: run.RunID, Version: 1}}}}})
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), "UPDATE ai_messaging_operations SET decision='accepted',receipt_id=?,receipt=?,decided_at=UTC_TIMESTAMP(6) WHERE command_id=?", uuid.NewString(), evaluationReceipt, startID)
	require.NoError(t, err)
	require.NoError(t, store.SubmitEvaluationCancel(t.Context(), run, cancelID, app.EvaluationCancel{ExpectedVersion: 1, Confirm: true}))
	await(cancelID)
	require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_outbox"))
	require.Zero(t, count("SELECT COUNT(*) FROM ai_bridge_commands"))
	// A real server-side rejection after original business writes must roll the
	// whole root back; this is not simulated commit uncertainty.
	_, err = pool.ExecContext(t.Context(), "CREATE TRIGGER fixture_command_reject BEFORE UPDATE ON rm_tx_witness FOR EACH ROW BEGIN IF NEW.state='committed' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='controlled fixture witness rejection'; END IF; END")
	require.NoError(t, err)
	aborted := r
	aborted.RequestID = uuid.NewString()
	require.ErrorIs(t, store.StageStart(t.Context(), aborted), ErrCommandUnresolved)
	_, err = pool.ExecContext(t.Context(), "DROP TRIGGER fixture_command_reject")
	require.NoError(t, err)
	require.Zero(t, count("SELECT COUNT(*) FROM ai_bridge_requests WHERE request_id=?", aborted.RequestID))
	require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_operations WHERE command_id=?", aborted.RequestID))
	require.Zero(t, count("SELECT COUNT(*) FROM ai_messaging_archive WHERE message_id=?", []byte(aborted.RequestID)))
	q, err := app.NewTransactionCommand(pb.MessagingKind_START, aborted.RequestID, aborted.RequestID, app.OperationScope{OrganizationID: "7", SubjectID: "9", ResourceID: aborted.RequestID}, startMessagingBody(aborted))
	require.NoError(t, err)
	winner, err := commands.archive.store.Lookup(t.Context(), transaction.Request{Owner: "qs.mysql", Scope: "org:7", Key: q.Key(), InputHash: q.InputHash()})
	require.NoError(t, err)
	require.Equal(t, transaction.LocalAborted, commands.archive.confirmCommand(t.Context(), winner.Execution))
	beforeDuplicate := seals.Load()
	require.ErrorIs(t, store.StageStart(t.Context(), aborted), ErrCommandUnresolved)
	require.Equal(t, beforeDuplicate, seals.Load())
	abortDeadline := time.NewTimer(300 * time.Millisecond)
checkAbortedVisibility:
	for {
		select {
		case message := <-received:
			require.NotEqual(t, aborted.RequestID, message.ID, "rolled back command became visible")
		case <-abortDeadline.C:
			break checkAbortedVisibility
		}
	}
	// Nonempty historical rows remain owned by the original path. Reading or
	// submitting their duplicate must not fabricate a new transaction witness.
	legacyStore := &MessagingCommandStore{Store: &Store{DB: pool}, Messaging: NewMessagingStore(), Seal: seal}
	old := r
	old.RequestID = uuid.NewString()
	require.NoError(t, legacyStore.StageStart(t.Context(), old))
	op, err = store.ReadOperation(t.Context(), app.OperationScope{OrganizationID: "7", SubjectID: "9"}, old.RequestID)
	require.NoError(t, err)
	require.Equal(t, "staged", op.TransportStatus)
	beforeDuplicate = seals.Load()
	require.NoError(t, store.StageStart(t.Context(), old))
	require.Equal(t, beforeDuplicate, seals.Load())
	cases := map[string]bool{"constructor_no_ddl": true, "missing_order_schema_refused": true, "half_invisible_before_root_commit": true, "original_business_archive_witness_atomic": true, "first_wire_authenticated_and_reused": true, "stable_input_conflict_before_reseal": true, "operation_and_payload_scoped": true, "fresh_adapter_exact_witness": true, "predecessor_pending_before_half": true, "held_predecessor_before_half": true, "workflow_and_evaluation_original_entrypoints": true, "no_new_command_outbox": true, "real_server_rejection_root_rollback": true, "aborted_duplicate_never_reexecutes": true, "nonempty_legacy_operation_compatible": true}
	raw, err := json.Marshal(map[string]any{"passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "root": "original_mysql_sql", "system_login_not_claimed": true, "paired_ai_receiver_not_claimed": true, "process_exit_not_claimed": true, "database_commit_network_fault_not_claimed": true})
	require.NoError(t, err)
	t.Log("M3_QS_AI_COMMAND_RECEIPT " + string(raw))
}
