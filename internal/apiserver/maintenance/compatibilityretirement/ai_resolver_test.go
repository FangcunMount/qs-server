package retirement

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	jose "github.com/go-jose/go-jose/v4"
	"gorm.io/gorm"
)

func aiLocalTestBinding() AIResolverBinding {
	b := AIResolverBinding{DatabaseIdentityHash: strings.Repeat("d", 64), MigrationVersion: 99, SourceSHA: strings.Repeat("e", 40), OperationID: "123-1", AdmissionRevision: 7}
	for _, table := range []string{AIBridgeCommandSource, AILegacyCommandSource} {
		boundary := SourceBoundary{Database: "mysql", Name: table, Kind: "base_table", Present: true, Empty: true, PKType: "ascii_string", SchemaHash: strings.Repeat("a", 64)}
		boundary.IdentityHash = aiFramedParts("mysql-object-v1", table, boundary.SchemaHash)
		if table == AIBridgeCommandSource {
			b.BridgeBoundary, b.BridgeColumns = boundary, aiFixtureColumns(table)
		} else {
			b.LegacyBoundary, b.LegacyColumns = boundary, aiFixtureColumns(table)
		}
	}
	return b
}

func aiLocalGORM(tx *sql.Tx) *gorm.DB { return &gorm.DB{Statement: &gorm.Statement{ConnPool: tx}} }

func TestAILocalBindingAndOriginalTransaction(t *testing.T) {
	b := aiLocalTestBinding()
	if !aiValidBinding(b) {
		t.Fatal("valid binding rejected")
	}
	for _, mutate := range []func(*AIResolverBinding){func(b *AIResolverBinding) { b.DatabaseIdentityHash = strings.Repeat("D", 64) }, func(b *AIResolverBinding) { b.SourceSHA = "main" }, func(b *AIResolverBinding) { b.OperationID = "123-0-x" }, func(b *AIResolverBinding) { b.BridgeBoundary.Present = false }, func(b *AIResolverBinding) { b.BridgeBoundary.IdentityHash = strings.Repeat("a", 64) }, func(b *AIResolverBinding) { b.BridgeColumns[0][1] = strptr("varchar(36)") }, func(b *AIResolverBinding) { b.LegacyBoundary.UpperToken = "private-token" }} {
		copy := aiCloneBinding(b)
		mutate(&copy)
		if aiValidBinding(copy) {
			t.Fatal("invalid binding accepted")
		}
	}
	for _, db := range []*gorm.DB{nil, {}, {Statement: &gorm.Statement{}}, {Statement: &gorm.Statement{ConnPool: (*gorm.PreparedStmtTX)(nil)}}, {Statement: &gorm.Statement{ConnPool: (*sql.Tx)(nil)}}} {
		if _, err := NewAILocalResolver(context.Background(), db, b); err != ErrAILocalTransaction {
			t.Fatal("non-transaction accepted")
		}
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal("mock creation")
	}
	defer func() {
		if db.Close() != nil {
			t.Error("mock close")
		}
	}()
	if _, err = NewAILocalResolver(context.Background(), &gorm.DB{Statement: &gorm.Statement{ConnPool: db}}, b); err != ErrAILocalTransaction {
		t.Fatal("pool accepted")
	}
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal("mock begin")
	}
	mock.ExpectQuery("SELECT @@server_uuid,DATABASE\\(\\),VERSION\\(\\)").WillReturnError(errors.New("private-dsn-and-body"))
	if _, err = NewAILocalResolver(context.Background(), aiLocalGORM(tx), b); err != ErrAILocalRead || strings.Contains(fmt.Sprint(err), "private-") {
		t.Fatal("read error leaked")
	}
	mock.ExpectRollback()
	if tx.Rollback() != nil {
		t.Fatal("borrowed transaction was closed")
	}
	mock.ExpectClose()
	if db.Close() != nil {
		t.Fatal("mock close")
	}
	if mock.ExpectationsWereMet() != nil {
		t.Fatal("mock expectations")
	}
}

func TestAILocalReadBoundsCloseAndPrivacy(t *testing.T) {
	for _, which := range []string{"rows", "bytes", "rows_error", "close"} {
		t.Run(which, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal("mock creation")
			}
			defer func() {
				if db.Close() != nil {
					t.Error("mock close")
				}
			}()
			rows := sqlmock.NewRows([]string{"fact"})
			want := ErrAILocalRead
			switch which {
			case "rows":
				for i := 0; i <= aiLocalMaxRows; i++ {
					rows.AddRow("fact")
				}
				want = ErrAILocalBounds
			case "bytes":
				rows.AddRow(bytes.Repeat([]byte{'x'}, aiLocalMaxBytes+1))
				want = ErrAILocalBounds
			case "rows_error":
				rows.AddRow("private-body").RowError(0, errors.New("private-body"))
			case "close":
				rows.CloseError(errors.New("private-body"))
			}
			mock.ExpectQuery("fixed").WillReturnRows(rows).RowsWillBeClosed()
			r := &AILocalResolver{pool: db}
			if _, err = r.read(context.Background(), "fixed"); err != want {
				t.Fatal("fixed read rejection mismatch")
			}
			mock.ExpectClose()
			if db.Close() != nil {
				t.Fatal("mock close")
			}
			if mock.ExpectationsWereMet() != nil {
				t.Fatal("row ownership mismatch")
			}
		})
	}
}

func TestAILocalSourceBaselineBudgetAndTypedMapping(t *testing.T) {
	for _, kind := range []string{"start", "answer", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			bridgeRow := aiFixtureRow(t, AIBridgeCommandSource, kind)
			bridge := aiReadFixture(t, AIBridgeCommandSource, bridgeRow)
			b := aiLocalTestBinding()
			b.BridgeBoundary.Empty = false
			b.BridgeBoundary.UpperToken = base64.StdEncoding.EncodeToString(bridgeRow[0])
			b.LegacyBoundary.Empty = false
			b.LegacyBoundary.UpperToken = b.BridgeBoundary.UpperToken
			r := &AILocalResolver{binding: b}
			requestRaw, _ := aiFixturePayload(t, "start")
			var request app.Start
			if json.Unmarshal(requestRaw, &request) != nil {
				t.Fatal("request fixture")
			}
			hash, err := aiExpectedCommandHash(bridgeRow[3], kind)
			if err != nil {
				t.Fatal("mapping")
			}
			legacyRow := aiFixtureRow(t, AILegacyCommandSource, kind)
			legacyRow[8] = []byte(hash)
			if kind == "start" {
				legacyRow[7] = []byte("2026-10-08T19:12:12.123456+08:00")
			}
			legacySource := aiReadFixture(t, AILegacyCommandSource, legacyRow)
			// Source rows are decoded against the exact approved columns; the
			// fixture parser's synthetic schema hash is intentionally identical.
			mapped, hashes, err := r.verifySourceRows([][][]byte{bridgeRow}, [][][]byte{legacyRow}, bridge, legacySource, &request, []byte("2026-10-08 11:12:12.123456"))
			if err != nil || mapped[bridge.CommandID].Source.Object != AILegacyCommandSource || hashes[bridge.CommandID] != hash {
				t.Fatal("exact source mapping rejected", err)
			}
			for _, index := range []int{5, 6, 7} {
				changed := aiCopyCells(bridgeRow)
				switch index {
				case 5:
					changed[index] = []byte("1")
				case 6:
					changed[index] = []byte("4")
				default:
					changed[index] = []byte("2026-10-08 11:12:13.123455")
				}
				if _, _, err = r.verifySourceRows([][][]byte{changed}, [][][]byte{legacyRow}, bridge, legacySource, &request, []byte("2026-10-08 11:12:12.123456")); err == nil {
					t.Fatal("changed full source baseline accepted")
				}
			}
			changed := aiCopyCells(legacyRow)
			changed[8] = []byte(strings.Repeat("f", 64))
			changedProof := aiReadFixture(t, AILegacyCommandSource, changed)
			if _, _, err = r.verifySourceRows([][][]byte{bridgeRow}, [][][]byte{changed}, bridge, changedProof, &request, []byte("2026-10-08 11:12:12.123456")); err != ErrAILocalRelation {
				t.Fatal("forged stored mapping hash accepted")
			}
		})
	}
}

func aiCopyCells(row [][]byte) [][]byte {
	copy := make([][]byte, len(row))
	for i, cell := range row {
		if cell != nil {
			copy[i] = append([]byte{}, cell...)
		}
	}
	return copy
}
func aiCopyRows(rows [][][]byte) [][][]byte {
	copy := make([][][]byte, len(rows))
	for i, row := range rows {
		copy[i] = aiCopyCells(row)
	}
	return copy
}

type aiLocalGraphFixture struct {
	request                             app.Start
	projection                          app.Event
	ops, boxes, inbox, failures, events [][][]byte
	sources                             map[string]*DecodedAICommand
	hashes                              map[string]string
}

func aiLocalGraph(t *testing.T) aiLocalGraphFixture {
	t.Helper()
	raw, _ := aiFixturePayload(t, "start")
	var request app.Start
	if json.Unmarshal(raw, &request) != nil {
		t.Fatal("fixture request")
	}
	projection := app.Event{EventID: "50000000-0000-4000-8000-000000000001", RequestID: request.RequestID, SessionID: aiFixtureSessionID, Actor: request.Actor, TesteeID: request.TesteeID, Version: 2, Status: "cancelled"}
	bridgeRow := aiFixtureRow(t, AIBridgeCommandSource, "start")
	bridge := aiReadFixture(t, AIBridgeCommandSource, bridgeRow)
	legacyRow := aiFixtureRow(t, AILegacyCommandSource, "start")
	hash, err := aiExpectedCommandHash(raw, "start")
	if err != nil {
		t.Fatal("mapping")
	}
	legacyRow[8] = []byte(hash)
	source := aiReadFixture(t, AILegacyCommandSource, legacyRow)
	keys := func(id string) jose.JSONWebKey {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal("fixture key")
		}
		return jose.JSONWebKey{Key: key, KeyID: id}
	}
	sign, encrypt := keys("fixture-sign"), keys("fixture-encrypt")
	protect := func(kind pb.MessagingKind, id, correlation string, body *pb.MessagingBody) *app.PreparedMessaging {
		value, err := app.ProtectMessaging(kind, id, request.RequestID, correlation, request.Actor.OrgID, "", body, sign, encrypt.Public())
		if err != nil {
			t.Fatal("fixture protect")
		}
		return value
	}
	var start pb.StartCommand
	start.RequestId = request.RequestID
	start.Actor = &pb.Actor{OrgId: request.Actor.OrgID, SubjectId: request.Actor.SubjectID}
	start.TesteeId = request.TesteeID
	start.AssessmentIds = request.AssessmentIDs
	start.Goal = request.Goal
	for _, e := range request.Evidence {
		item := &pb.EvidenceItem{AssessmentId: e.AssessmentID, TesteeId: e.TesteeID, ReportId: e.ReportID, SourceVersion: e.SourceVersion}
		for _, f := range e.Facts {
			item.Facts = append(item.Facts, &pb.Fact{Ref: f.Ref, Value: f.Value})
		}
		start.Evidence = append(start.Evidence, item)
	}
	command := protect(pb.MessagingKind_START, request.RequestID, "", &pb.MessagingBody{Value: &pb.MessagingBody_Start{Start: &start}})
	receiptID := "60000000-0000-4000-8000-000000000001"
	receipt := protect(pb.MessagingKind_COMMAND_RECEIPT, receiptID, request.RequestID, &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: &pb.MessagingCommandReceipt{CommandId: request.RequestID, CommandBodySha256: hash, Decision: pb.MessagingDecision_ACCEPTED, Code: "accepted", OriginalReceipt: &pb.MessagingCommandReceipt_WorkflowReceipt{WorkflowReceipt: &pb.Receipt{SessionId: projection.SessionID, RunId: "70000000-0000-4000-8000-000000000001", Version: 1, Status: "queued"}}}}})
	state := protect(pb.MessagingKind_INTERPRETATION_STATE, projection.EventID, "", &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: &pb.StateEvent{EventId: projection.EventID, RequestId: projection.RequestID, SessionId: projection.SessionID, Actor: &pb.Actor{OrgId: projection.Actor.OrgID, SubjectId: projection.Actor.SubjectID}, TesteeId: projection.TesteeID, Version: projection.Version, Status: projection.Status}}})
	stamp := []byte("2026-10-08 11:12:14.123456")
	op := [][]byte{[]byte(request.RequestID), []byte("1"), []byte(hash), []byte(request.Actor.OrgID), []byte(request.Actor.SubjectID), []byte(request.RequestID), []byte(request.RequestID), []byte("1"), []byte("accepted"), []byte("accepted"), []byte(receiptID), receipt.Body, stamp, stamp, []byte("0"), nil, nil}
	box := func(message *app.PreparedMessaging) [][]byte {
		return [][]byte{[]byte("qs-server"), []byte("qs-ai"), []byte(message.Envelope.MessageId), []byte(message.Envelope.BodySha256), message.Body, message.Wire, []byte(sourceSHA(message.Wire)), []byte(strconv.Itoa(int(message.Envelope.Kind))), []byte(request.Actor.OrgID), []byte(message.Topic), []byte(request.RequestID), []byte("1"), []byte("0"), []byte("0"), []byte("confirmed"), []byte("3"), stamp, stamp, stamp, stamp, []byte{}}
	}
	commandBox := box(command)
	commandBox[12], commandBox[13] = []byte("1"), []byte("1")
	f := aiLocalGraphFixture{request: request, projection: projection, ops: [][][]byte{op}, boxes: [][][]byte{commandBox}, sources: map[string]*DecodedAICommand{bridge.CommandID: source}, hashes: map[string]string{bridge.CommandID: hash}}
	for i, message := range []*app.PreparedMessaging{receipt, state} {
		ackID := fmt.Sprintf("80000000-0000-4000-8000-%012d", i+1)
		ack := protect(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, ackID, "", &pb.MessagingBody{Value: &pb.MessagingBody_EventAcknowledgement{EventAcknowledgement: &pb.MessagingEventAcknowledgement{EventId: message.Envelope.MessageId, EventBodySha256: message.Envelope.BodySha256, EventKind: message.Envelope.Kind, Outcome: pb.MessagingEventAcknowledgement_STORED}}})
		f.boxes = append(f.boxes, box(ack))
		f.inbox = append(f.inbox, [][]byte{[]byte("qs-ai"), []byte(message.Envelope.MessageId), []byte(message.Envelope.BodySha256), message.Body, []byte(sourceSHA(message.Wire)), []byte(strconv.Itoa(int(message.Envelope.Kind))), []byte(request.RequestID), []byte(ackID), stamp, []byte("stored")})
	}
	eventRaw, err := json.Marshal(projection)
	if err != nil {
		t.Fatal("fixture projection")
	}
	f.events = [][][]byte{{[]byte(projection.EventID), []byte(request.RequestID), []byte("2"), []byte(sourceSHA(eventRaw))}}
	f.failures = [][][]byte{{[]byte("qs-ai"), []byte(receiptID), []byte(receipt.Envelope.BodySha256), []byte(strconv.Itoa(int(receipt.Envelope.Kind))), []byte(request.RequestID), receipt.Wire, []byte("2"), stamp, stamp}}
	return f
}

func (f aiLocalGraphFixture) verify() error {
	return aiVerifyMQGraph(f.ops, f.boxes, f.inbox, f.failures, f.events, &f.request, &f.projection, f.sources, f.hashes)
}

func TestAILocalPointGraphRequiresBusinessAndBothMessageDirections(t *testing.T) {
	f := aiLocalGraph(t)
	if err := f.verify(); err != nil {
		t.Fatal("valid local point graph rejected", err)
	}
	cases := []struct {
		name   string
		mutate func(*aiLocalGraphFixture)
	}{
		{"wrong_org", func(f *aiLocalGraphFixture) { f.ops[0][3] = []byte("1") }},
		{"wrong_aggregate", func(f *aiLocalGraphFixture) { f.boxes[0][10] = []byte(aiFixtureSessionID) }},
		{"missing_outbox", func(f *aiLocalGraphFixture) { f.boxes = f.boxes[1:] }},
		{"unconfirmed", func(f *aiLocalGraphFixture) { f.boxes[0][14] = []byte("published"); f.boxes[0][19] = nil }},
		{"budget_reset", func(f *aiLocalGraphFixture) { f.boxes[0][15] = []byte("2") }},
		{"original_body_mapping", func(f *aiLocalGraphFixture) { f.hashes[aiFixtureRequestID] = strings.Repeat("f", 64) }},
		{"receipt_missing", func(f *aiLocalGraphFixture) { f.inbox = f.inbox[1:] }},
		{"ack_missing", func(f *aiLocalGraphFixture) { f.boxes = f.boxes[:2] }},
		{"event_fact_missing", func(f *aiLocalGraphFixture) { f.events = nil }},
		{"event_hash_changed", func(f *aiLocalGraphFixture) { f.events[0][3] = []byte(strings.Repeat("f", 64)) }},
		{"failure_unknown", func(f *aiLocalGraphFixture) { f.failures[0][1] = []byte(aiFixtureQuestionID) }},
		{"inbox_held", func(f *aiLocalGraphFixture) { f.inbox[0][9] = []byte("held") }},
		{"retired_is_not_receipt", func(f *aiLocalGraphFixture) { f.ops[0][14] = []byte("1") }},
		{"original_wire_changed", func(f *aiLocalGraphFixture) { f.boxes[0][5] = append(f.boxes[0][5], byte('x')) }},
		{"orphan_operation", func(f *aiLocalGraphFixture) {
			row := aiCopyCells(f.ops[0])
			row[0] = []byte(aiFixtureCommandID)
			f.ops = append(f.ops, row)
		}},
		{"unknown_extra_message_kind", func(f *aiLocalGraphFixture) { f.boxes[0][7] = []byte("99") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			copy := f
			copy.ops = aiCopyRows(f.ops)
			copy.boxes = aiCopyRows(f.boxes)
			copy.inbox = aiCopyRows(f.inbox)
			copy.failures = aiCopyRows(f.failures)
			copy.events = aiCopyRows(f.events)
			copy.hashes = map[string]string{aiFixtureRequestID: f.hashes[aiFixtureRequestID]}
			tc.mutate(&copy)
			if copy.verify() == nil {
				t.Fatal("invalid local graph accepted")
			}
		})
	}
}

func TestAILocalWireAndOpaqueQualificationPrivacy(t *testing.T) {
	f := aiLocalGraph(t)
	wire := f.boxes[0][5]
	id := aiText(f.boxes[0], 2)
	if !aiCheckStoredWire(wire, id, sourceSHA(wire)) {
		t.Fatal("wire rejected")
	}
	outer, _, err := legacy.Decode(wire)
	if err != nil {
		t.Fatal("wire fixture")
	}
	outer.Metadata["extra"] = "private-secret"
	changed, err := legacy.Encode(outer, legacy.Revision2)
	if err != nil {
		t.Fatal("wire fixture encoding")
	}
	if aiCheckStoredWire(changed, id, sourceSHA(changed)) || aiCheckStoredWire(wire, aiFixtureSessionID, sourceSHA(wire)) {
		t.Fatal("wire identity/metadata mismatch accepted")
	}
	outer.Metadata = map[string]string{"secure_profile": protected.Profile}
	outer.Payload = []byte("private-sensitive-not-a-jwe")
	changed, err = legacy.Encode(outer, legacy.Revision2)
	if err != nil {
		t.Fatal("wire fixture encoding")
	}
	if aiCheckStoredWire(changed, id, sourceSHA(changed)) {
		t.Fatal("invalid encrypted wire shape accepted")
	}
	q := &AILocalQualification{binding: aiLocalTestBinding(), baseline: strings.Repeat("a", 64), nodes: 8, gaps: []string{"external_unknown"}, bridge: f.sources[aiFixtureRequestID], qualified: true}
	if _, err = json.Marshal(q); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("opaque proof serialized")
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", q, q), aiFixtureRequestID) {
		t.Fatal("opaque proof text leaked")
	}
	s := q.Summary()
	if !s.LocalQualified || s.DropReady || s.ExternalClosure != "unknown" || s.GlobalReverseCoverage != "unknown" || s.RetirementConclusion != "not_generated" {
		t.Fatal("local proof overstated")
	}
	s.Gaps[0] = "mutated"
	if reflect.DeepEqual(s.Gaps, q.gaps) {
		t.Fatal("summary aliases private state")
	}
	if err = q.Recheck(context.Background(), nil); err == nil || q.Summary().LocalQualified {
		t.Fatal("failed recheck did not invalidate")
	}
	if err = q.Recheck(context.Background(), nil); err != ErrAILocalChanged {
		t.Fatal("invalid proof resumed")
	}
	zero := new(AILocalQualification)
	if zero.Summary().LocalQualified || zero.Recheck(context.Background(), nil) != ErrAILocalChanged {
		t.Fatal("zero value forged a qualification")
	}
	if _, err = new(AILocalResolver).Resolve(context.Background(), nil, nil); err != ErrAILocalBinding {
		t.Fatal("zero resolver accepted")
	}
}

func TestAILocalAggregateOrderingAndArtifactCorrelation(t *testing.T) {
	f := aiLocalGraph(t)
	rows := [][][]byte{{[]byte(f.request.RequestID), []byte("2")}}
	if aiVerifyAggregate(rows, f.ops, f.request.RequestID) != nil || aiVerifyAggregate(nil, nil, f.request.RequestID) != nil {
		t.Fatal("valid aggregate proof rejected")
	}
	for _, bad := range [][][][]byte{nil, {{[]byte(f.request.RequestID), []byte("3")}}, {{[]byte(aiFixtureSessionID), []byte("2")}}} {
		if aiVerifyAggregate(bad, f.ops, f.request.RequestID) == nil {
			t.Fatal("aggregate contradiction accepted")
		}
	}
	request := f.request
	request.Evidence = append([]app.EvidenceItem(nil), f.request.Evidence...)
	request.Evidence[0].ReportID = "7"
	content := `{"schema_version":"ai-explanation-output/v1"}`
	hash := "sha256:" + strings.Repeat("a", 64)
	artifact := app.Artifact{ID: aiFixtureQuestionID, SessionID: aiFixtureSessionID, RunID: aiFixtureCommandID, EvidenceSetID: aiFixtureQuestionID, EvidenceFingerprint: strings.Repeat("a", 64), InvocationID: aiFixtureCommandID, ProviderRequestID: "fixture-provider", ContentJSON: content, ContentFingerprint: "sha256:" + sourceSHA([]byte(content)), InputFingerprint: hash, ProfileID: "fixture-profile", ProfileVersion: "v1", ProfileFingerprint: hash, PromptFingerprint: hash, RouteFingerprint: hash, OutputValidatorVersion: "v1", SafetyValidatorVersion: "v1", AssessmentID: request.AssessmentIDs[0], ReportID: "7", SourceVersion: request.Evidence[0].SourceVersion, SchemaVersion: "qs-ai-artifact/v1"}
	event := f.projection
	event.Status = "completed"
	raw, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal("fixture artifact")
	}
	event.ArtifactJSON = string(raw)
	if aiVerifyArtifactRequest(event, &request) != nil {
		t.Fatal("artifact correlation rejected")
	}
	request.Evidence[0].ReportID = "8"
	if aiVerifyArtifactRequest(event, &request) != ErrAILocalRelation {
		t.Fatal("wrong report artifact accepted")
	}
}
