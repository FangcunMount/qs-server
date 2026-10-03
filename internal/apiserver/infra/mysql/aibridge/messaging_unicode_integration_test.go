//go:build integration

package aibridge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	mysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

// The normal host pool negotiates utf8mb3. Native fixtures using the driver's
// default utf8mb4 missed four-byte Unicode conversion at JSON persistence.
func TestMQOriginalUTF8BytesSurviveNormalHostCharset(t *testing.T) {
	f := newMQFixture(t)
	cfg, err := mysql.ParseDSN(os.Getenv("QS_AI_MQ_TEST_DSN"))
	mustMQ(t, err)
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["charset"] = "utf8"
	cfg.Params["time_zone"] = "'+08:00'"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	mustMQ(t, err)
	defer db.Close()
	f.db = db
	var before string
	mustMQ(t, db.QueryRow("SELECT @@character_set_connection").Scan(&before))
	if before != "utf8mb3" && before != "utf8" {
		t.Fatal("fixture did not negotiate the normal three-byte charset", before)
	}
	f.request.Goal = "中文🙂𐐷：原接单"
	f.request.Evidence = []app.EvidenceItem{{AssessmentID: "9", TesteeID: "7", ReportID: "99", SourceVersion: "standard-v1:101", Facts: []app.Fact{}}}
	original := &Store{DB: db}
	commands := &MessagingCommandStore{Store: original, Messaging: f.store, Seal: func(k pb.MessagingKind, id, agg, org, at string, b *pb.MessagingBody) (*app.PreparedMessaging, error) {
		return app.ProtectMessaging(k, id, agg, "", org, at, b, f.qsSign, f.aiCrypt.Public())
	}}
	mustMQ(t, commands.StageStart(context.Background(), f.request))
	mustMQ(t, commands.StageStart(context.Background(), f.request))
	got, err := original.Original(context.Background(), f.request.RequestID)
	mustMQ(t, err)
	if got.Goal != f.request.Goal {
		t.Fatal("original Unicode changed")
	}
	_, requestHash, err := encode(f.request)
	mustMQ(t, err)
	var storedHash string
	mustMQ(t, db.QueryRow("SELECT request_hash FROM ai_bridge_requests WHERE request_id=?", f.request.RequestID).Scan(&storedHash))
	if storedHash != requestHash {
		t.Fatal("business hash was reconstructed")
	}

	event := app.Event{EventID: uuid.NewString(), RequestID: f.request.RequestID, SessionID: f.session, Actor: f.request.Actor, TesteeID: f.request.TesteeID, Version: 1, Status: "awaiting_answer", QuestionID: uuid.NewString(), Question: "原问题🙂𐐷"}
	state := func(e app.Event) *app.PreparedMessaging {
		return f.protect(pb.MessagingKind_INTERPRETATION_STATE, e.EventID, e.RequestID, "", &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: &pb.StateEvent{EventId: e.EventID, RequestId: e.RequestID, SessionId: e.SessionID, Actor: &pb.Actor{OrgId: e.Actor.OrgID, SubjectId: e.Actor.SubjectID}, TesteeId: e.TesteeID, Version: e.Version, Status: e.Status, QuestionId: e.QuestionID, Question: e.Question, ArtifactJson: e.ArtifactJSON}}}, true)
	}
	message := state(event)
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.receive(tx, message, nil) }))
	answer := "回答🙂𐐷"
	change := app.Change{CommandID: uuid.NewString(), SessionID: f.session, Actor: f.request.Actor, Action: "answer", ExpectedVersion: 1, QuestionID: event.QuestionID, Answer: &answer}
	// Verify the retained historical JSON writer too; this does not activate a
	// legacy relay or add a second runtime path.
	mustMQ(t, original.StageChange(context.Background(), f.request.RequestID, change))
	var payload []byte
	mustMQ(t, db.QueryRow("SELECT CAST(payload AS BINARY) FROM ai_bridge_commands WHERE command_id=?", change.CommandID).Scan(&payload))
	var roundtrip app.Change
	mustMQ(t, json.Unmarshal(payload, &roundtrip))
	if roundtrip.Answer == nil || *roundtrip.Answer != answer {
		t.Fatal("retained original answer changed")
	}
	// A reviewed historical transfer must decode the same first Unicode source,
	// retain its original hash and reuse its wire on repeated apply.
	handoffSeals := 0
	handoff := handoffFixture(f, &handoffSeals)
	for i := 0; i < 2; i++ {
		mustMQ(t, f.tx(func(tx *sql.Tx) error {
			moved, err := handoff.StageSingle(context.Background(), tx, change.CommandID)
			if err == nil && moved != (i == 0) {
				return errors.New("historical Unicode transfer did not preserve first ownership")
			}
			return err
		}))
	}
	if handoffSeals != 1 {
		t.Fatal("historical duplicate resealed", handoffSeals)
	}
	_, changeHash, err := encode(change)
	mustMQ(t, err)
	var source []byte
	mustMQ(t, db.QueryRow("SELECT source_payload,source_payload_hash FROM ai_messaging_legacy_commands WHERE command_id=?", change.CommandID).Scan(&source, &storedHash))
	var transferred app.Change
	mustMQ(t, json.Unmarshal(source, &transferred))
	if transferred.Answer == nil || *transferred.Answer != answer || storedHash != changeHash {
		t.Fatal("historical first source changed")
	}

	content, _ := json.Marshal(map[string]string{"schema_version": "ai-explanation-output/v1", "text": strings.Repeat("边界🙂", 11000)})
	sum := sha256.Sum256(content)
	d := "sha256:" + strings.Repeat("a", 64)
	artifact := app.Artifact{ID: uuid.NewString(), SessionID: f.session, RunID: uuid.NewString(), EvidenceSetID: uuid.NewString(), EvidenceFingerprint: strings.Repeat("a", 64), InvocationID: uuid.NewString(), ProviderRequestID: "isolated", ContentJSON: string(content), ContentFingerprint: "sha256:" + hex.EncodeToString(sum[:]), InputFingerprint: d, ProfileID: "profile", ProfileVersion: "v1", ProfileFingerprint: d, PromptFingerprint: d, RouteFingerprint: d, OutputValidatorVersion: "v1", SafetyValidatorVersion: "v1", AssessmentID: "9", ReportID: "99", SourceVersion: "standard-v1:101", SchemaVersion: "qs-ai-artifact/v1"}
	raw, err := json.Marshal(artifact)
	mustMQ(t, err)
	padding := 131072 - len(raw)
	if padding < 0 {
		t.Fatal("fixture exceeds the existing artifact limit")
	}
	// Extend content and its original digest to the exact legal byte boundary.
	var textContent map[string]string
	mustMQ(t, json.Unmarshal(content, &textContent))
	textContent["text"] += strings.Repeat("x", padding)
	content, err = json.Marshal(textContent)
	mustMQ(t, err)
	sum = sha256.Sum256(content)
	artifact.ContentJSON = string(content)
	artifact.ContentFingerprint = "sha256:" + hex.EncodeToString(sum[:])
	raw, err = json.Marshal(artifact)
	mustMQ(t, err)
	if len(raw) != 131072 {
		t.Fatal("fixture is not the legal 128-KiB boundary", len(raw))
	}
	event.EventID = uuid.NewString()
	event.Version = 2
	event.Status = "completed"
	event.QuestionID = ""
	event.Question = ""
	event.ArtifactJSON = string(raw)
	message = state(event)
	injected := errors.New("ACK storage unavailable")
	if err = f.tx(func(tx *sql.Tx) error {
		return f.store.ReceiveEvent(context.Background(), tx, message.Envelope, message.Body, message.Wire, func(string, string) (*app.PreparedMessaging, error) { return nil, injected })
	}); !errors.Is(err, injected) {
		t.Fatal("expected original transaction rollback", err)
	}
	projection, err := original.Projection(context.Background(), f.request.RequestID)
	mustMQ(t, err)
	if projection.Version != 1 {
		t.Fatal("result escaped failed ACK transaction")
	}
	calls := 0
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.receive(tx, message, &calls) }))
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return f.receive(tx, message, &calls) }))
	if calls != 1 {
		t.Fatal("duplicate rebuilt ACK", calls)
	}
	projection, err = original.Projection(context.Background(), f.request.RequestID)
	mustMQ(t, err)
	if projection.Version != 2 || projection.ArtifactJSON != event.ArtifactJSON {
		t.Fatal("original Unicode artifact changed")
	}
	_, eventHash, err := encode(event)
	mustMQ(t, err)
	mustMQ(t, db.QueryRow("SELECT payload_hash FROM ai_bridge_events WHERE event_id=?", event.EventID).Scan(&storedHash))
	if storedHash != eventHash {
		t.Fatal("original event hash changed")
	}
	var after string
	mustMQ(t, db.QueryRow("SELECT @@character_set_connection").Scan(&after))
	if after != before {
		t.Fatal("adapter changed the borrowed pool charset")
	}
}
