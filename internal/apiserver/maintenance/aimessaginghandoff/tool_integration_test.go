//go:build integration

package aimessaginghandoff

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func checked(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	db                 *sql.DB
	tool               *Tool
	sealCalls          int
	dsn                string
	signing, recipient jose.JSONWebKey
}

func setup(t *testing.T) *fixture {
	t.Helper()
	cfg, err := mysql.ParseDSN(os.Getenv("QS_AI_MQ_TEST_DSN"))
	checked(t, err)
	if cfg.DBName == "" || cfg.Net != "tcp" || (!strings.HasPrefix(cfg.Addr, "127.0.0.1:") && !strings.HasPrefix(cfg.Addr, "localhost:")) {
		t.Fatal("explicit disposable loopback handoff database required")
	}
	cfg.DBName = ""
	cfg.ParseTime = true
	cfg.MultiStatements = true
	root, err := sql.Open("mysql", cfg.FormatDSN())
	checked(t, err)
	name := "rm_ai_mq_handoff_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = root.Exec("CREATE DATABASE " + name)
	checked(t, err)
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	checked(t, err)
	t.Cleanup(func() {
		_ = db.Close()
		_, err := root.Exec("DROP DATABASE " + name)
		if err != nil {
			t.Error(err)
		}
		_ = root.Close()
	})
	for _, id := range []string{"000072_ai_bridge_delivery", "000083_ai_runtime_index", "000091_ai_messaging", "000092_ai_messaging_failures", "000093_ai_messaging_legacy_commands", "000094_ai_messaging_admission"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "pkg", "migration", "migrations", "mysql", id+".up.sql"))
		checked(t, err)
		_, err = db.Exec(string(raw))
		checked(t, err)
	}
	_, err = db.Exec("CREATE TABLE schema_migrations(version BIGINT UNSIGNED NOT NULL,dirty BOOLEAN NOT NULL); INSERT INTO schema_migrations VALUES(95,FALSE)")
	checked(t, err)
	sign, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	checked(t, err)
	crypt, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	checked(t, err)
	f := &fixture{db: db, dsn: cfg.FormatDSN(), signing: jose.JSONWebKey{Key: sign, KeyID: "fixture.sign"}, recipient: jose.JSONWebKey{Key: &crypt.PublicKey, KeyID: "fixture.encrypt"}}
	f.tool = &Tool{DB: db, Handoff: &store.MessagingLegacyHandoff{Store: store.NewMessagingStore(), Seal: func(k pb.MessagingKind, id, agg, org, at string, b *pb.MessagingBody) (*app.PreparedMessaging, error) {
		f.sealCalls++
		return app.ProtectMessaging(k, id, agg, "", org, at, b, jose.JSONWebKey{Key: sign, KeyID: "fixture.sign"}, jose.JSONWebKey{Key: &crypt.PublicKey, KeyID: "fixture.encrypt"})
	}}}
	return f
}
func (f *fixture) start(t *testing.T) app.Start {
	t.Helper()
	r := app.Start{RequestID: uuid.NewString(), Actor: app.Actor{OrgID: "18446744073709551615", SubjectID: "42"}, TesteeID: "7", AssessmentIDs: []string{"9"}, Goal: "private body：原 Unicode 😀 must remain private"}
	checked(t, (&store.Store{DB: f.db}).StageStart(context.Background(), r))
	return r
}
func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	checked(t, db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&n))
	return n
}
func wire(t *testing.T, db *sql.DB, id string) []byte {
	t.Helper()
	var raw []byte
	checked(t, db.QueryRow("SELECT wire FROM ai_messaging_outbox WHERE message_id=?", id).Scan(&raw))
	return raw
}

func TestMQHandoffDryRunPreservesSourceAndReplayWire(t *testing.T) {
	f := setup(t)
	r := f.start(t)
	ctx := context.Background()
	_, err := f.db.Exec("UPDATE ai_bridge_commands SET attempts=3,available_at='2026-09-30 03:04:05.123456' WHERE command_id=?", r.RequestID)
	checked(t, err)
	m, err := f.tool.DryRun(ctx, []string{r.RequestID})
	checked(t, err)
	if count(t, f.db, "ai_messaging_outbox") != 0 || f.sealCalls != 0 || m.Rows[0].Attempts != 3 || m.Rows[0].AvailableAt != "2026-09-30T03:04:05.123456Z" {
		t.Fatal("dry-run wrote or changed original facts")
	}
	raw, err := json.Marshal(m)
	checked(t, err)
	if bytes.Contains(raw, []byte(r.Goal)) {
		t.Fatal("source body leaked into manifest")
	}
	first, err := f.tool.Apply(ctx, m, m.SHA256(), true)
	checked(t, err)
	if len(first.Rows) != 1 || first.Rows[0].Outcome != "transferred" {
		t.Fatal(first)
	}
	original := wire(t, f.db, r.RequestID)
	again, err := f.tool.Apply(ctx, m, m.SHA256(), true)
	checked(t, err)
	if len(again.Rows) != 1 || again.Rows[0].Outcome != "source_retained_no_new_transfer" || !bytes.Equal(original, wire(t, f.db, r.RequestID)) || f.sealCalls != 1 {
		t.Fatal("repeat resealed wire")
	}
	var attempts int
	var delivered bool
	checked(t, f.db.QueryRow("SELECT attempts,delivered FROM ai_bridge_commands WHERE command_id=?", r.RequestID).Scan(&attempts, &delivered))
	if attempts != 3 || delivered || count(t, f.db, "ai_bridge_commands") != 1 {
		t.Fatal("source erased/reset")
	}
}

func TestMQHandoffReviewAndMaintenanceDriftRefuseBeforeWrite(t *testing.T) {
	f := setup(t)
	r := f.start(t)
	ctx := context.Background()
	m, err := f.tool.DryRun(ctx, []string{r.RequestID})
	checked(t, err)
	for _, test := range []struct {
		digest  string
		stopped bool
		want    error
	}{{"wrong", true, ErrManifest}, {m.SHA256(), false, ErrMaintenance}} {
		_, err = f.tool.Apply(ctx, m, test.digest, test.stopped)
		if !errors.Is(err, test.want) {
			t.Fatal("missing explicit review or maintenance accepted", err)
		}
	}
	_, err = f.db.Exec("UPDATE ai_messaging_admission SET revision=revision+1")
	checked(t, err)
	_, err = f.tool.Apply(ctx, m, m.SHA256(), true)
	if !errors.Is(err, ErrManifestDrift) {
		t.Fatal("stale gate accepted", err)
	}
	m, err = f.tool.DryRun(ctx, []string{r.RequestID})
	checked(t, err)
	_, err = f.db.Exec("UPDATE ai_bridge_commands SET attempts=attempts+1 WHERE command_id=?", r.RequestID)
	checked(t, err)
	_, err = f.tool.Apply(ctx, m, m.SHA256(), true)
	if !errors.Is(err, ErrManifestDrift) {
		t.Fatal("source drift accepted", err)
	}
	m, err = f.tool.DryRun(ctx, []string{r.RequestID})
	checked(t, err)
	_, err = f.db.Exec("UPDATE ai_bridge_commands SET delivered=TRUE WHERE command_id=?", r.RequestID)
	checked(t, err)
	_, err = f.tool.Apply(ctx, m, m.SHA256(), true)
	if !errors.Is(err, ErrManifestDrift) {
		t.Fatal("source ownership changed after review", err)
	}
	if f.sealCalls != 0 || count(t, f.db, "ai_messaging_operations") != 0 || count(t, f.db, "ai_messaging_outbox") != 0 {
		t.Fatal("review failure left orphan")
	}
	_, err = f.db.Exec("UPDATE ai_messaging_admission SET closed=FALSE")
	checked(t, err)
	_, err = f.tool.DryRun(ctx, []string{r.RequestID})
	if !errors.Is(err, ErrMaintenance) {
		t.Fatal("open intake accepted", err)
	}
}

func TestMQHandoffStopsPartialBatchAtFirstDrift(t *testing.T) {
	f := setup(t)
	a, b, c := f.start(t), f.start(t), f.start(t)
	ctx := context.Background()
	m, err := f.tool.DryRun(ctx, []string{a.RequestID, b.RequestID, c.RequestID})
	checked(t, err)
	_, err = f.db.Exec("UPDATE ai_bridge_commands SET payload_hash=REPEAT('0',64) WHERE command_id=?", b.RequestID)
	checked(t, err)
	result, err := f.tool.Apply(ctx, m, m.SHA256(), true)
	if !errors.Is(err, ErrManifestDrift) || len(result.Rows) != 1 || result.Rows[0].CommandID != a.RequestID || count(t, f.db, "ai_messaging_outbox") != 1 {
		t.Fatal("partial progress hidden or later source transferred", result, err)
	}
	first := wire(t, f.db, a.RequestID)
	result, err = f.tool.Apply(ctx, m, m.SHA256(), true)
	if !errors.Is(err, ErrManifestDrift) || len(result.Rows) != 1 || !bytes.Equal(first, wire(t, f.db, a.RequestID)) || f.sealCalls != 1 {
		t.Fatal("partial retry resealed/reordered", result, err)
	}
}

func TestMQHandoffStorageFailureRollsBackWholeRow(t *testing.T) {
	f := setup(t)
	r := f.start(t)
	ctx := context.Background()
	m, err := f.tool.DryRun(ctx, []string{r.RequestID})
	checked(t, err)
	_, err = f.db.Exec("CREATE TRIGGER reject_handoff BEFORE INSERT ON ai_messaging_legacy_commands FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='isolated storage fault'")
	checked(t, err)
	result, err := f.tool.Apply(ctx, m, m.SHA256(), true)
	if err == nil || len(result.Rows) != 0 {
		t.Fatal("rolled back row claimed committed")
	}
	for _, table := range []string{"ai_messaging_outbox", "ai_messaging_operations", "ai_messaging_legacy_commands", "ai_messaging_aggregates"} {
		if count(t, f.db, table) != 0 {
			t.Fatal("orphan after storage rollback", table)
		}
	}
	if count(t, f.db, "ai_bridge_commands") != 1 {
		t.Fatal("original source lost")
	}
}

func TestMQHandoffRetainsDeliveredExhaustedAndUnknownTime(t *testing.T) {
	f := setup(t)
	history, exhausted, missing := f.start(t), f.start(t), f.start(t)
	ctx := context.Background()
	_, err := f.db.Exec("UPDATE ai_bridge_commands SET delivered=TRUE WHERE command_id=?", history.RequestID)
	checked(t, err)
	_, err = f.db.Exec("UPDATE ai_bridge_commands SET attempts=11 WHERE command_id=?", exhausted.RequestID)
	checked(t, err)
	_, err = f.db.Exec("UPDATE ai_bridge_requests SET created_at=NULL WHERE request_id=?", missing.RequestID)
	checked(t, err)
	m, err := f.tool.DryRun(ctx, []string{history.RequestID, exhausted.RequestID, missing.RequestID})
	checked(t, err)
	if m.Rows[2].OriginalCreatedAt != nil {
		t.Fatal("unknown time invented")
	}
	result, err := f.tool.Apply(ctx, m, m.SHA256(), true)
	checked(t, err)
	if len(result.Rows) != 3 || count(t, f.db, "ai_messaging_outbox") != 2 {
		t.Fatal("delivered history reset")
	}
	var stage, originalTime string
	var budget int
	checked(t, f.db.QueryRow("SELECT stage,attempts FROM ai_messaging_outbox WHERE message_id=?", exhausted.RequestID).Scan(&stage, &budget))
	if stage != "held" || budget != 8 {
		t.Fatal("exhausted history got new budget")
	}
	checked(t, f.db.QueryRow("SELECT source_original_time FROM ai_messaging_legacy_commands WHERE command_id=?", missing.RequestID).Scan(&originalTime))
	if originalTime != "" {
		t.Fatal("missing time invented from retry")
	}
}

func TestMQHandoffGlobalAmbiguousHistoryAndBoundsRefuse(t *testing.T) {
	f := setup(t)
	known, ambiguous := f.start(t), f.start(t)
	ctx := context.Background()
	session := uuid.NewString()
	_, err := f.db.Exec("UPDATE ai_bridge_commands SET delivered=TRUE WHERE command_id=?", ambiguous.RequestID)
	checked(t, err)
	_, err = f.db.Exec("UPDATE ai_bridge_requests SET session_id=? WHERE request_id=?", session, ambiguous.RequestID)
	checked(t, err)
	for i := 0; i < 2; i++ {
		checked(t, (&store.Store{DB: f.db}).StageChange(ctx, ambiguous.RequestID, app.Change{CommandID: uuid.NewString(), SessionID: session, Actor: ambiguous.Actor, Action: "cancel", ExpectedVersion: 2}))
	}
	m, err := f.tool.DryRun(ctx, []string{known.RequestID})
	checked(t, err)
	if m.UnknownOrderAggregates != 1 {
		t.Fatal("unselected ambiguous aggregate omitted")
	}
	_, err = f.tool.Apply(ctx, m, m.SHA256(), true)
	if !errors.Is(err, store.ErrLegacyCommandOrderUnknown) || f.sealCalls != 0 {
		t.Fatal("guessed global historical order", err)
	}
	for _, ids := range [][]string{nil, {known.RequestID, known.RequestID}, {uuid.Nil.String()}, make([]string, MaxBatch+1)} {
		if _, err = f.tool.DryRun(ctx, ids); !errors.Is(err, ErrManifest) {
			t.Fatal("unbounded or ambiguous selection accepted", err)
		}
	}
}

func TestMQHandoffRetainedWireCorruptionIsNotSuccessfulReplay(t *testing.T) {
	f := setup(t)
	r := f.start(t)
	ctx := context.Background()
	m, err := f.tool.DryRun(ctx, []string{r.RequestID})
	checked(t, err)
	_, err = f.tool.Apply(ctx, m, m.SHA256(), true)
	checked(t, err)
	_, err = f.db.Exec("UPDATE ai_messaging_outbox SET wire=CONCAT(wire,'x') WHERE message_id=?", r.RequestID)
	checked(t, err)
	result, err := f.tool.Apply(ctx, m, m.SHA256(), true)
	if err == nil || len(result.Rows) != 0 || f.sealCalls != 1 {
		t.Fatal("corrupt wire resealed or replay called successful")
	}
}

type crashInput struct {
	DSN                  string
	Manifest             Manifest
	Signing, Recipient   jose.JSONWebKey
	Marker, ReplayReport string
}

// The child uses the actual transfer API and original storage adapter. Only the
// test harness waits after commit; there is no fault switch in the shipped CLI.
func handoffCrashChild(t *testing.T, mode, inputFile string) {
	t.Helper()
	raw, err := os.ReadFile(inputFile)
	checked(t, err)
	var input crashInput
	checked(t, json.Unmarshal(raw, &input))
	cfg, err := mysql.ParseDSN(input.DSN)
	checked(t, err)
	if cfg.Net != "tcp" || !strings.HasPrefix(cfg.DBName, "rm_ai_mq_handoff_") {
		t.Fatal("disposable crash binding required")
	}
	db, err := sql.Open("mysql", input.DSN)
	checked(t, err)
	defer db.Close()
	calls := 0
	tool := &Tool{DB: db, Handoff: &store.MessagingLegacyHandoff{Store: store.NewMessagingStore(), Seal: func(k pb.MessagingKind, id, agg, org, at string, b *pb.MessagingBody) (*app.PreparedMessaging, error) {
		calls++
		return app.ProtectMessaging(k, id, agg, "", org, at, b, input.Signing, input.Recipient)
	}}}
	result, err := tool.Apply(context.Background(), input.Manifest, input.Manifest.SHA256(), true)
	checked(t, err)
	encoded, err := json.Marshal(struct {
		Result    Result
		SealCalls int
	}{result, calls})
	checked(t, err)
	if mode == "replay" {
		checked(t, os.WriteFile(input.ReplayReport, encoded, 0600))
		return
	}
	checked(t, os.WriteFile(input.Marker, encoded, 0600))
	// Keep a verifiably live child until the parent injects SIGKILL after commit.
	for {
		time.Sleep(time.Second)
	}
}

func TestMQHandoffProcessCrashBeforeAndAfterCommit(t *testing.T) {
	if mode := os.Getenv("QS_MQ_HANDOFF_CRASH_MODE"); mode != "" {
		handoffCrashChild(t, mode, os.Getenv("QS_MQ_HANDOFF_CRASH_INPUT"))
		return
	}
	for _, mode := range []string{"before", "after"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t)
			original := f.start(t)
			ctx := context.Background()
			m, err := f.tool.DryRun(ctx, []string{original.RequestID})
			checked(t, err)
			dir := t.TempDir()
			inputFile := filepath.Join(dir, "input.private.json")
			input := crashInput{DSN: f.dsn, Manifest: m, Signing: f.signing, Recipient: f.recipient, Marker: filepath.Join(dir, "committed.json"), ReplayReport: filepath.Join(dir, "replay.json")}
			raw, err := json.Marshal(input)
			checked(t, err)
			checked(t, os.WriteFile(inputFile, raw, 0600))
			lockName := "rm_ai_mq_before_" + uuid.NewString()
			if mode == "before" {
				_, err = f.db.Exec("CREATE TRIGGER pause_handoff BEFORE INSERT ON ai_messaging_legacy_commands FOR EACH ROW SET @mq_handoff_pause=IF(GET_LOCK('" + lockName + "',0)=1,SLEEP(60),NULL)")
				checked(t, err)
			}
			child := func(mode string) *exec.Cmd {
				cmd := exec.Command(os.Args[0], "-test.run", "^TestMQHandoffProcessCrashBeforeAndAfterCommit$")
				cmd.Env = append(os.Environ(), "QS_MQ_HANDOFF_CRASH_MODE="+mode, "QS_MQ_HANDOFF_CRASH_INPUT="+inputFile)
				return cmd
			}
			cmd := child(mode)
			logPath := filepath.Join(dir, "child.log")
			logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0600)
			checked(t, err)
			defer logFile.Close()
			cmd.Stdout, cmd.Stderr = logFile, logFile
			checked(t, cmd.Start())
			done := make(chan struct{})
			var waitErr error
			go func() { waitErr = cmd.Wait(); close(done) }()
			t.Cleanup(func() {
				select {
				case <-done:
				default:
					_ = cmd.Process.Kill()
					<-done
				}
			})
			deadline := time.Now().Add(15 * time.Second)
			ready := false
			for !ready && time.Now().Before(deadline) {
				select {
				case <-done:
					output, _ := os.ReadFile(logPath)
					t.Fatalf("child exited before boundary: %v: %s", waitErr, output)
				default:
				}
				if mode == "after" {
					_, err = os.Stat(input.Marker)
					ready = err == nil
				} else {
					var connection sql.NullInt64
					checked(t, f.db.QueryRow("SELECT IS_USED_LOCK(?)", lockName).Scan(&connection))
					if connection.Valid {
						var database string
						checked(t, f.db.QueryRow("SELECT DB FROM information_schema.processlist WHERE ID=?", connection.Int64).Scan(&database))
						ready = database == m.Database
					}
				}
				if !ready {
					time.Sleep(25 * time.Millisecond)
				}
			}
			if !ready {
				t.Fatal("child did not reach the observed original transaction boundary")
			}
			checked(t, cmd.Process.Signal(syscall.Signal(0))) // actual live process, not a marker-only assumption
			var committedWire []byte
			if mode == "after" {
				committedWire = wire(t, f.db, original.RequestID)
			} else {
				if count(t, f.db, "ai_messaging_outbox") != 0 {
					t.Fatal("uncommitted wire visible")
				}
			}
			checked(t, cmd.Process.Kill())
			<-done
			err = waitErr
			var exit *exec.ExitError
			if !errors.As(err, &exit) || !exit.ProcessState.Sys().(syscall.WaitStatus).Signaled() {
				t.Fatal("actual process kill not established")
			}
			if mode == "before" {
				// A closed client can leave the server cancelling its SLEEP. Wait for the
				// observed SQL handle to disappear; never infer rollback from timeout.
				deadline = time.Now().Add(90 * time.Second)
				for {
					var connection sql.NullInt64
					checked(t, f.db.QueryRow("SELECT IS_USED_LOCK(?)", lockName).Scan(&connection))
					if !connection.Valid {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("killed transaction handle still live")
					}
					time.Sleep(100 * time.Millisecond)
				}
				for _, table := range []string{"ai_messaging_outbox", "ai_messaging_operations", "ai_messaging_legacy_commands", "ai_messaging_aggregates"} {
					if count(t, f.db, table) != 0 {
						t.Fatal("process died before commit with orphan", table)
					}
				}
				_, err = f.db.Exec("DROP TRIGGER pause_handoff")
				checked(t, err)
			}
			restarted := child("replay")
			if output, runErr := restarted.CombinedOutput(); runErr != nil {
				t.Fatalf("fresh process could not reapply original manifest: %v: %s", runErr, output)
			}
			raw, err = os.ReadFile(input.ReplayReport)
			checked(t, err)
			var report struct {
				Result    Result
				SealCalls int
			}
			checked(t, json.Unmarshal(raw, &report))
			if len(report.Result.Rows) != 1 || count(t, f.db, "ai_messaging_legacy_commands") != 1 || count(t, f.db, "ai_bridge_commands") != 1 {
				t.Fatal("fresh process lost or duplicated source")
			}
			if mode == "after" && (report.SealCalls != 0 || !bytes.Equal(committedWire, wire(t, f.db, original.RequestID))) {
				t.Fatal("restart resealed committed wire")
			}
			if mode == "before" && (report.SealCalls != 1 || report.Result.Rows[0].Outcome != "transferred") {
				t.Fatal("rollback did not permit one complete transfer")
			}
		})
	}
}
