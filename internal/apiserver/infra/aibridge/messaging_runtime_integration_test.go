//go:build integration

package aibridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	jose "github.com/go-jose/go-jose/v4"
	_ "github.com/go-sql-driver/mysql"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	driver "github.com/nsqio/go-nsq"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMQRuntimeNSQCommitDuplicateRearmAndStopBorrowedPool(t *testing.T) {
	dsn, address, httpAddress := os.Getenv("QS_AI_MQ_TEST_DSN"), os.Getenv("QS_AI_MQ_NSQD"), os.Getenv("QS_AI_MQ_NSQD_HTTP")
	if dsn == "" || address == "" || httpAddress == "" {
		t.Fatal("required disposable MySQL/NSQ endpoints missing")
	}
	db, e := sql.Open("mysql", dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = db.Close() })
	if e = db.Ping(); e != nil {
		t.Fatal(e)
	}
	// A maintenance closure refuses new commands, but must keep original event
	// receiving and final-ACK publication alive for drain and reconciliation.
	gateTx, e := db.BeginTx(t.Context(), nil)
	if e != nil {
		t.Fatal(e)
	}
	gate := store.MessagingAdmission{}
	gateBefore, e := gate.Inspect(t.Context(), gateTx)
	if e != nil {
		_ = gateTx.Rollback()
		t.Fatal(e)
	}
	gateClosed, e := gate.Set(t.Context(), gateTx, true, gateBefore.Revision)
	if e != nil {
		_ = gateTx.Rollback()
		t.Fatal(e)
	}
	if e = gateTx.Commit(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = tx.Rollback() }()
		_, err = gate.Set(context.Background(), tx, gateBefore.Closed, gateClosed.Revision)
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			t.Error(err)
		}
	})
	// Only isolated fixture provisioning mutates topology. Runtime startup is read-only.
	for topic, channel := range map[string]string{app.CommandsTopic: "qs-ai.commands.v1", app.EventsTopic: "qs-server.ai-events.v1", app.AcksTopic: "qs-ai.acks.v1"} {
		for name, ch := range map[string]string{topic: channel, legacy.FailedHandoffTopic(topic, channel): legacy.FailedHandoffChannel} {
			topicResponse, err := http.Post(httpAddress+"/topic/create?"+url.Values{"topic": {name}}.Encode(), "application/octet-stream", nil)
			if err != nil {
				t.Fatal(err)
			}
			_ = topicResponse.Body.Close()
			if topicResponse.StatusCode != 200 {
				t.Fatal("fixture topic unavailable")
			}
			response, e := http.Post(httpAddress+"/channel/create?"+url.Values{"topic": {name}, "channel": {ch}}.Encode(), "application/octet-stream", nil)
			if e != nil {
				t.Fatal(e)
			}
			_ = response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatal("fixture topology unavailable")
			}
		}
	}
	qsSign, qsCrypt, aiSign, aiCrypt := localMessagingKey(t, "qs-sign"), localMessagingKey(t, "qs-crypt"), localMessagingKey(t, "ai-sign"), localMessagingKey(t, "ai-crypt")
	s := store.NewMessagingStore()
	receiver := &store.MessagingEventReceiver{DB: db, Store: s, Bodies: &MessagingPayloadClient{}, Keys: protected.Keyring{Decrypt: map[string]jose.JSONWebKey{qsCrypt.KeyID: qsCrypt}, Signers: map[string]protected.TrustedSigner{aiSign.KeyID: {Producer: "qs-ai", Key: aiSign.Public()}}}}
	receiver.Seal = func(k pb.MessagingKind, id, agg, org, original string, b *pb.MessagingBody) (*app.PreparedMessaging, error) {
		return app.ProtectMessaging(k, id, agg, "", org, time.Now().In(time.FixedZone("UTC+8", 28800)).Format(time.RFC3339Nano), b, qsSign, aiCrypt.Public())
	}
	o := opts.AIWorkflowMessagingOptions{Enabled: true, NSQD: map[string]string{address: httpAddress}, SigningKeyFile: "host-loaded", AIRecipientKeyFile: "host-loaded", DecryptKeyFiles: map[string]string{"qs": "host-loaded"}, AISignerFiles: map[string]string{"ai": "host-loaded"}}
	run, id := uuid.NewString(), uuid.NewString()
	originalTime := "2026-10-03T12:00:00.123456+08:00"
	b := &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationState{EvaluationState: &pb.EvaluationRuntimeState{RunId: run, OrganizationId: "18446744073709551615", Version: 1, EventSequence: 1, Status: "running", ReleaseFingerprint: "frozen-original"}}}
	m, e := app.ProtectMessaging(pb.MessagingKind_EVALUATION_STATE, id, run, "", "18446744073709551615", originalTime, b, aiSign, qsCrypt.Public())
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(m.Wire)
	firstHash := hex.EncodeToString(hash[:])
	t.Cleanup(func() {
		for _, table := range []string{"ai_messaging_inbox", "ai_messaging_outbox", "ai_messaging_failures"} {
			if _, e := db.Exec("DELETE FROM "+table+" WHERE aggregate_key=?", run); e != nil {
				t.Error(e)
			}
		}
		if _, e := db.Exec("DELETE FROM ai_messaging_evaluation_states WHERE run_id=?", run); e != nil {
			t.Error(e)
		}
	})
	makeRuntime := func() *MessagingRuntime {
		r, e := NewMessagingRuntime(o, db, s, receiver)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	stop := func(r *MessagingRuntime) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if e := r.Stop(ctx); e != nil {
			t.Fatal(e)
		}
	}
	r := makeRuntime()
	if r.started || r.business != nil || r.relayDone != nil {
		t.Fatal("constructor created hidden work")
	}
	if e = r.Start(t.Context()); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { stop(r) })
	firstConsumer, firstRelay := r.business, r.relayDone
	if e = r.Start(t.Context()); e != nil || r.business != firstConsumer || r.relayDone != firstRelay {
		t.Fatal("duplicate start")
	}
	producer, e := driver.NewProducer(address, driver.NewConfig())
	if e != nil {
		t.Fatal(e)
	}
	producer.SetLogger(log.New(io.Discard, "", 0), driver.LogLevelError)
	defer producer.Stop()
	await := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("isolated MQ settlement timeout")
	}
	if e = producer.Publish(app.EventsTopic, m.Wire); e != nil {
		t.Fatal(e)
	}
	var ackID string
	await(func() bool {
		return db.QueryRow("SELECT ack_id FROM ai_messaging_inbox WHERE producer='qs-ai' AND message_id=?", id).Scan(&ackID) == nil
	})
	var savedWire []byte
	var savedHash string
	if e = db.QueryRow("SELECT wire_sha256 FROM ai_messaging_inbox WHERE producer='qs-ai' AND message_id=?", id).Scan(&savedHash); e != nil || savedHash != firstHash {
		t.Fatal("first raw wire changed")
	}
	if e = db.QueryRow("SELECT wire FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&savedWire); e != nil {
		t.Fatal(e)
	}
	await(func() bool {
		var stage string
		return db.QueryRow("SELECT stage FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&stage) == nil && stage == "confirmed"
	})
	stop(r)
	if e = db.Ping(); e != nil {
		t.Fatal("runtime closed host pool")
	}
	// Object recreation proves lifecycle/DB restoration; process-kill proof is separate.
	r = makeRuntime()
	if e = r.Start(t.Context()); e != nil {
		t.Fatal(e)
	}
	if e = producer.Publish(app.EventsTopic, m.Wire); e != nil {
		t.Fatal(e)
	}
	await(func() bool {
		var attempts int
		return db.QueryRow("SELECT attempts FROM ai_messaging_outbox WHERE message_id=?", ackID).Scan(&attempts) == nil && attempts >= 2
	})
	var duplicateWire []byte
	var duplicateID string
	if e = db.QueryRow("SELECT ack_id FROM ai_messaging_inbox WHERE producer='qs-ai' AND message_id=?", id).Scan(&duplicateID); e != nil {
		t.Fatal(e)
	}
	if e = db.QueryRow("SELECT wire FROM ai_messaging_outbox WHERE message_id=?", duplicateID).Scan(&duplicateWire); e != nil || duplicateID != ackID || !bytes.Equal(savedWire, duplicateWire) {
		t.Fatal("duplicate ACK recreated")
	}
	var sequence uint64
	if e = db.QueryRow("SELECT event_sequence FROM ai_messaging_evaluation_states WHERE run_id=?", run).Scan(&sequence); e != nil || sequence != 1 {
		t.Fatal("duplicate projection effect")
	}

	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	unavailable := listener.Addr().String()
	_ = listener.Close()
	badOptions := o
	badOptions.NSQD = map[string]string{unavailable: httpAddress}
	partial, e := NewMessagingRuntime(badOptions, db, s, receiver)
	if e != nil {
		t.Fatal(e)
	}
	if e = partial.Start(t.Context()); e == nil || partial.business != nil || partial.failure != nil || partial.handlerCancel != nil || partial.started || partial.stopping {
		t.Fatal("partial startup retained active resources", e)
	}
	if e = db.Ping(); e != nil {
		t.Fatal("partial startup closed host pool")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if e = makeRuntime().Start(canceled); !errors.Is(e, context.Canceled) {
		t.Fatal("canceled start performed I/O")
	}
}

func TestMQRuntimeMissingTechnicalCoverageRefusesBeforeTransport(t *testing.T) {
	cfg, err := mysqlDriver.ParseDSN(os.Getenv("QS_AI_MQ_TEST_DSN"))
	if err != nil || cfg.DBName == "" || strings.ContainsAny(cfg.DBName, "`\\") {
		t.Fatal("required safe disposable test schema", err)
	}
	source, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	sourceSchema := cfg.DBName
	name := "mq_preflight_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = source.Exec("CREATE DATABASE `" + name + "`"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := source.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Error(err)
		}
	}()
	for _, table := range []string{"ai_messaging_admission", "ai_messaging_aggregates", "ai_messaging_evaluation_states", "ai_messaging_inbox", "ai_messaging_failures", "ai_messaging_outbox", "ai_messaging_operations", "ai_messaging_quarantine", "ai_messaging_observations"} {
		if _, err = source.Exec("CREATE TABLE `" + name + "`." + table + " LIKE `" + sourceSchema + "`." + table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = source.Exec("INSERT INTO `" + name + "`.ai_messaging_admission SELECT * FROM `" + sourceSchema + "`.ai_messaging_admission"); err != nil {
		t.Fatal(err)
	}
	if _, err = source.Exec("INSERT INTO `" + name + "`.ai_messaging_observations SELECT * FROM `" + sourceSchema + "`.ai_messaging_observations WHERE kind<>'duplicate_event'"); err != nil {
		t.Fatal(err)
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runtime := &MessagingRuntime{DB: db}
	err = runtime.Start(t.Context())
	if err == nil || err.Error() != "AI MQ required technical observation schema unavailable" || runtime.httpTransport != nil || runtime.business != nil || runtime.failure != nil || runtime.publisher != nil || runtime.started {
		t.Fatal("partial schema consumed/connected transport", err)
	}
	if _, err = source.Exec("INSERT INTO `" + name + "`.ai_messaging_observations SELECT * FROM `" + sourceSchema + "`.ai_messaging_observations WHERE kind='duplicate_event'"); err != nil {
		t.Fatal(err)
	}
	if err = runtime.preflightStorage(t.Context()); err != nil {
		t.Fatal("complete schema refused", err)
	}
	if _, err = db.Exec("DROP TABLE ai_messaging_observations"); err != nil {
		t.Fatal(err)
	}
	err = runtime.Start(t.Context())
	if err == nil || err.Error() != "AI MQ required schema unavailable" || runtime.httpTransport != nil {
		t.Fatal("missing table exposed details/created clients", err)
	}
	if err = db.Ping(); err != nil {
		t.Fatal("borrowed pool closed", err)
	}
}
