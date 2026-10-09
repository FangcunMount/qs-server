package retirement

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/go-jose/go-jose/v4"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// These private fixture seams exercise pure relation/classification rules;
// they cannot construct an exported production source or transaction authority.
func aiReverseUnitSnapshot() *AIReverseSnapshot {
	s := &AIReverseSnapshot{limits: DefaultAIReverseLimits(), started: time.Now(), byTable: map[string]map[string]*aiReverseNode{}, anchors: map[string]aiReverseAnchor{}}
	s.self = s
	for _, spec := range aiReverseSpecs {
		s.byTable[spec.table] = map[string]*aiReverseNode{}
	}
	return s
}
func aiReverseUnitAdd(s *AIReverseSnapshot, table string, n *aiReverseNode) {
	n.observation.Store = table
	n.observation.PrimaryKeySHA256 = aiReverseKeySHA([]string{n.id})
	n.observation.RowSHA256 = aiReverseHash("unit", table, n.id)
	s.byTable[table][n.id] = n
	s.nodes = append(s.nodes, n)
}
func aiReverseUnitRow(spec aiReverseSpec, raw [][]byte) aiReverseRow {
	r := aiReverseRow{}
	for i, name := range spec.columns {
		r[name] = raw[i]
	}
	return r
}
func aiReverseUnitGraph(t *testing.T) *AIReverseSnapshot {
	t.Helper()
	s := aiReverseUnitSnapshot()
	f := aiLocalGraph(t)
	requestRaw, e := json.Marshal(f.request)
	if e != nil {
		t.Fatal(e)
	}
	projectionRaw, e := json.Marshal(f.projection)
	if e != nil {
		t.Fatal(e)
	}
	r := aiReverseRow{"request_id": []byte(f.request.RequestID), "request_hash": []byte(sourceSHA(requestRaw)), "payload": requestRaw, "session_id": []byte(f.projection.SessionID), "version": []byte("2"), "status": []byte("cancelled"), "projection": projectionRaw, "organization_id": []byte(f.request.Actor.OrgID), "subject_id": []byte(f.request.Actor.SubjectID), "testee_id": []byte(f.request.TesteeID), "created_at": nil, "updated_at": nil}
	n := s.decode(aiReverseSpecByTable("ai_bridge_requests"), r, aiReverseMetadata{})
	aiReverseUnitAdd(s, "ai_bridge_requests", n)
	for _, id := range n.assessments {
		s.anchors[id] = aiReverseAnchor{id, n.org, n.testee, "9", strings.Repeat("a", 64)}
		aiReverseUnitAdd(s, "ai_bridge_request_assessments", &aiReverseNode{id: n.id + ":" + id, request: n.id, resource: id})
	}
	for table, rows := range map[string][][][]byte{"ai_messaging_operations": f.ops, "ai_messaging_outbox": f.boxes, "ai_messaging_inbox": f.inbox, "ai_messaging_failures": f.failures, "ai_bridge_events": f.events} {
		spec := aiReverseSpecByTable(table)
		for _, raw := range rows {
			aiReverseUnitAdd(s, table, s.decode(spec, aiReverseUnitRow(spec, raw), aiReverseMetadata{}))
		}
	}
	aiReverseUnitAdd(s, "ai_messaging_aggregates", &aiReverseNode{id: n.id, aggregate: n.id, sequence: 2})
	aiReverseUnitAdd(s, "ai_messaging_admission", &aiReverseNode{id: "1", state: "closed"})
	for kind := range aiReverseObservationKinds {
		aiReverseUnitAdd(s, "ai_messaging_observations", &aiReverseNode{id: kind})
	}
	return s
}

type aiReversePriorEventFixture struct {
	event           app.Event
	eventRow, inbox [][]byte
	ackOutbox       [][]byte
}

// A delayed version1 is a different original event under the same request.
// Its stored Inbox and final ACK follow the real receiver contract; it does
// not replace the existing version2 projection or introduce an operation.
func aiReversePriorEvent(t *testing.T, f aiLocalGraphFixture) aiReversePriorEventFixture {
	t.Helper()
	prior := f.projection
	prior.EventID = "50000000-0000-4000-8000-000000000002"
	prior.Version, prior.Status = 1, "queued"
	if prior.EventID == prior.RequestID || app.ValidateEvent(prior) != nil {
		t.Fatal("distinct valid original event fixture")
	}
	keys := func(id string) jose.JSONWebKey {
		key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal("fixture key")
		}
		return jose.JSONWebKey{Key: key, KeyID: id}
	}
	sign, encrypt := keys("prior-fixture-sign"), keys("prior-fixture-encrypt")
	protect := func(kind pb.MessagingKind, id string, body *pb.MessagingBody) *app.PreparedMessaging {
		v, e := app.ProtectMessaging(kind, id, prior.RequestID, "", prior.Actor.OrgID, "", body, sign, encrypt.Public())
		if e != nil {
			t.Fatal("fixture protect")
		}
		return v
	}
	state := protect(pb.MessagingKind_INTERPRETATION_STATE, prior.EventID, &pb.MessagingBody{Value: &pb.MessagingBody_InterpretationState{InterpretationState: &pb.StateEvent{EventId: prior.EventID, RequestId: prior.RequestID, SessionId: prior.SessionID, Actor: &pb.Actor{OrgId: prior.Actor.OrgID, SubjectId: prior.Actor.SubjectID}, TesteeId: prior.TesteeID, Version: prior.Version, Status: prior.Status, QuestionId: prior.QuestionID, Question: prior.Question, CanSkip: prior.CanSkip, FailureCode: prior.FailureCode, ArtifactJson: prior.ArtifactJSON}}})
	ackID := "80000000-0000-4000-8000-000000000003"
	ack := protect(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, ackID, &pb.MessagingBody{Value: &pb.MessagingBody_EventAcknowledgement{EventAcknowledgement: &pb.MessagingEventAcknowledgement{EventId: state.Envelope.MessageId, EventBodySha256: state.Envelope.BodySha256, EventKind: state.Envelope.Kind, Outcome: pb.MessagingEventAcknowledgement_STORED}}})
	raw, e := json.Marshal(prior)
	if e != nil {
		t.Fatal("fixture event JSON")
	}
	stamp := []byte("2026-10-08 11:12:14.123456")
	return aiReversePriorEventFixture{
		event:     prior,
		eventRow:  [][]byte{[]byte(prior.EventID), []byte(prior.RequestID), []byte("1"), []byte(sourceSHA(raw))},
		inbox:     [][]byte{[]byte("qs-ai"), []byte(state.Envelope.MessageId), []byte(state.Envelope.BodySha256), state.Body, []byte(sourceSHA(state.Wire)), []byte(strconv.Itoa(int(state.Envelope.Kind))), []byte(prior.RequestID), []byte(ackID), stamp, []byte("stored")},
		ackOutbox: [][]byte{[]byte("qs-server"), []byte("qs-ai"), []byte(ack.Envelope.MessageId), []byte(ack.Envelope.BodySha256), ack.Body, ack.Wire, []byte(sourceSHA(ack.Wire)), []byte(strconv.Itoa(int(ack.Envelope.Kind))), []byte(prior.Actor.OrgID), []byte(ack.Topic), []byte(prior.RequestID), []byte("1"), []byte("0"), []byte("0"), []byte("confirmed"), []byte("3"), stamp, stamp, stamp, stamp, []byte{}},
	}
}

func TestAIReverseEventOriginalPrimaryKey(t *testing.T) {
	f := aiLocalGraph(t)
	spec := aiReverseSpecByTable("ai_bridge_events")
	for _, which := range []string{"original", "different_original", "missing", "empty", "invalid"} {
		t.Run(which, func(t *testing.T) {
			r := aiReverseUnitRow(spec, f.events[0])
			want := f.projection.EventID
			switch which {
			case "different_original":
				want = "50000000-0000-4000-8000-000000000099"
				r["event_id"] = []byte(want)
			case "missing":
				want, r["event_id"] = "", nil
			case "empty":
				want, r["event_id"] = "", []byte{}
			case "invalid":
				want, r["event_id"] = "not-an-event-id", []byte("not-an-event-id")
			}
			n := aiReverseUnitSnapshot().decode(spec, r, aiReverseMetadata{})
			if n.id != want || n.id == f.request.RequestID || n.request != f.request.RequestID {
				t.Fatal("original event PK replaced by its parent or projection")
			}
			invalid := which == "missing" || which == "empty" || which == "invalid"
			if n.observation.Invalid != invalid {
				t.Fatal("original event PK validation changed")
			}
		})
	}
}

func TestAIReverseMultipleOriginalEventsKeepOwnerAndConflictChecks(t *testing.T) {
	for _, which := range []string{"two_versions", "physical_id_mismatch", "duplicate_request_version"} {
		t.Run(which, func(t *testing.T) {
			f := aiLocalGraph(t)
			prior := aiReversePriorEvent(t, f)
			s := aiReverseUnitGraph(t)
			if which == "physical_id_mismatch" {
				prior.eventRow[0] = []byte("50000000-0000-4000-8000-000000000099")
			}
			if which == "duplicate_request_version" {
				prior.eventRow[2] = []byte("2")
			}
			for table, raw := range map[string][][]byte{"ai_bridge_events": prior.eventRow, "ai_messaging_inbox": prior.inbox, "ai_messaging_outbox": prior.ackOutbox} {
				spec := aiReverseSpecByTable(table)
				aiReverseUnitAdd(s, table, s.decode(spec, aiReverseUnitRow(spec, raw), aiReverseMetadata{}))
			}
			s.reverse()
			if len(s.byTable["ai_bridge_events"]) != 2 || s.byTable["ai_bridge_events"][f.request.RequestID] != nil {
				t.Fatal("same-request events merged under the parent FK")
			}
			for _, id := range []string{f.projection.EventID, string(prior.eventRow[0])} {
				n := s.byTable["ai_bridge_events"][id]
				if n == nil || n.id != id || n.request != f.request.RequestID || n.org != f.request.Actor.OrgID || n.subject != f.request.Actor.SubjectID || n.testee != f.request.TesteeID || n.resource != f.projection.SessionID {
					t.Fatal("original event lost its exact owner")
				}
			}
			s.classify(&aiReverseScope{relatedRequests: map[string]bool{}, relatedIDs: map[string]bool{}, identityConflicts: map[string]bool{}})
			if which == "two_versions" {
				if s.report.Blocking != 0 || s.report.Unknown != 0 || s.byTable["ai_bridge_requests"][f.request.RequestID].event != f.projection.EventID {
					t.Fatal("valid original versions broke current projection", s.report.BlockingReasons)
				}
				return
			}
			want := "inbox_interpretation_event_conflict"
			if which == "duplicate_request_version" {
				want = "event_version_duplicate"
			}
			found := false
			for _, n := range s.nodes {
				for _, reason := range n.observation.Reasons {
					found = found || reason == want
				}
			}
			if s.report.Blocking == 0 || !found {
				t.Fatal("conflicting original event accepted or substituted", want)
			}
		})
	}
}

// Exercise the real Outbox decoding order, not a post-decode observation.
// A STORED body describes the received event; only confirmed transport plus
// that outcome is terminal for this ACK's own Outbox responsibility.
func TestAIReverseACKTransportAndOutcomeRemainIndependent(t *testing.T) {
	for _, stage := range []string{"staged", "awaiting_receipt", "held", "confirmed"} {
		for _, outcome := range []pb.MessagingEventAcknowledgement_Outcome{pb.MessagingEventAcknowledgement_STORED, pb.MessagingEventAcknowledgement_TECHNICALLY_HELD} {
			t.Run(stage+"/"+outcome.String(), func(t *testing.T) {
				f := aiLocalGraph(t)
				spec := aiReverseSpecByTable("ai_messaging_outbox")
				r := aiReverseUnitRow(spec, f.boxes[1])
				var body pb.MessagingBody
				if proto.Unmarshal(r["body"], &body) != nil || body.GetEventAcknowledgement() == nil {
					t.Fatal("original ACK body fixture")
				}
				body.GetEventAcknowledgement().Outcome = outcome
				key := func(id string) jose.JSONWebKey {
					value, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
					if e != nil {
						t.Fatal("fixture key")
					}
					return jose.JSONWebKey{Key: value, KeyID: id}
				}
				sign, encrypt := key("ack-transport-fixture-sign"), key("ack-transport-fixture-encrypt")
				ack, e := app.ProtectMessaging(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, r.text("message_id"), r.text("aggregate_key"), "", r.text("organization_id"), "", &body, sign, encrypt.Public())
				if e != nil {
					t.Fatal("valid protected ACK fixture")
				}
				r["body"], r["wire"] = ack.Body, ack.Wire
				r["body_sha256"], r["wire_sha256"] = []byte(ack.Envelope.BodySha256), []byte(sourceSHA(ack.Wire))
				r["stage"] = []byte(stage)
				if stage != "confirmed" {
					r["confirmed_at"] = nil
				}
				n := aiReverseUnitSnapshot().decode(spec, r, aiReverseMetadata{})
				wantHeld := stage == "held" || outcome == pb.MessagingEventAcknowledgement_TECHNICALLY_HELD
				wantUnfinished := stage != "confirmed" || outcome == pb.MessagingEventAcknowledgement_TECHNICALLY_HELD
				if n.observation.Invalid || n.state != stage || n.ackOutcome != outcome || n.observation.Held != wantHeld || n.observation.Unfinished != wantUnfinished || n.id != ack.Envelope.MessageId || n.event != body.GetEventAcknowledgement().EventId || n.bodyHash != ack.Envelope.BodySha256 || n.wireHash != sourceSHA(ack.Wire) {
					t.Fatal("ACK body outcome replaced transport responsibility", n.observation.Reasons)
				}
			})
		}
	}
}

func TestAIReverseNativePKPredicateAndNULLHash(t *testing.T) {
	spec := aiReverseSpecByTable("ai_bridge_request_assessments")
	q, args := aiReversePredicate(spec, []string{aiFixtureRequestID, "10"}, false)
	if q != "((`request_id`>?) OR (`request_id`=? AND `assessment_id`>?))" || !reflect.DeepEqual(args, []any{aiFixtureRequestID, aiFixtureRequestID, uint64(10)}) {
		t.Fatal("native keyset lost complete PK semantics")
	}
	if aiReverseCompare(spec, []string{"a", "2"}, []string{"a", "10"}) >= 0 {
		t.Fatal("numeric keys compared as byte strings")
	}
	if aiReverseRowSHA([]string{"x"}, aiReverseRow{"x": nil}) == aiReverseRowSHA([]string{"x"}, aiReverseRow{"x": []byte{}}) {
		t.Fatal("NULL lost its raw identity")
	}
	for _, key := range []string{" padded", "trailing ", "非ASCII"} {
		if _, e := aiReverseKey(aiReverseSpecByTable("ai_bridge_requests"), aiReverseRow{"request_id": []byte(key)}); e == nil {
			t.Fatal("unsupported PK cursor accepted")
		}
	}
}
func TestAIReverseCompleteKnownRelations(t *testing.T) {
	s := aiReverseUnitGraph(t)
	s.reverse()
	for _, n := range s.nodes {
		if n.observation.Invalid {
			t.Fatal(n.observation.Store, n.observation.Reasons)
		}
	}
	s.classify(nil)
	if !s.Summary().SourceAuthenticationRequired || s.Summary().Unknown == 0 || s.Summary().GlobalReverseQualified || s.Summary().DropReady {
		t.Fatal("local relations fabricated source/global authority")
	}
}
func TestAIReverseOutsidePendingRequiresFullPrivateSourceScope(t *testing.T) {
	s := aiReverseUnitGraph(t)
	s.reverse()
	id := aiFixtureRequestID
	s.byTable["ai_messaging_outbox"]["80000000-0000-4000-8000-000000000001"].observation.Unfinished = true
	s.classify(nil)
	if s.report.OutsideActive != 0 {
		t.Fatal("missing source scope classified active row as outside")
	}
	outside := &aiReverseScope{relatedRequests: map[string]bool{}, relatedIDs: map[string]bool{}, identityConflicts: map[string]bool{}}
	s.classify(outside)
	if s.report.Blocking != 0 || s.report.OutsideActive != 1 {
		t.Fatal("valid unrelated traffic became target blocker", s.report.BlockingReasons)
	}
	related := &aiReverseScope{relatedRequests: map[string]bool{id: true}, relatedIDs: map[string]bool{}, identityConflicts: map[string]bool{}}
	s.classify(related)
	if s.report.Blocking == 0 || s.report.Related == 0 {
		t.Fatal("related unfinished responsibility omitted")
	}
}
func TestAIReverseOrphanCrossOrganizationACKAndAggregate(t *testing.T) {
	for _, which := range []string{"orphan", "organization", "ack", "sequence", "retired"} {
		t.Run(which, func(t *testing.T) {
			s := aiReverseUnitGraph(t)
			id := aiFixtureRequestID
			switch which {
			case "orphan":
				delete(s.byTable["ai_messaging_operations"], id)
			case "organization":
				s.byTable["ai_messaging_outbox"][id].org = "999"
			case "ack":
				for _, n := range s.byTable["ai_messaging_outbox"] {
					if n.kind == pb.MessagingKind_EVENT_ACKNOWLEDGEMENT {
						n.hash = strings.Repeat("b", 64)
						break
					}
				}
			case "sequence":
				s.byTable["ai_messaging_aggregates"][id].sequence = ^uint64(0)
			case "retired":
				s.byTable["ai_messaging_operations"][id].retired = true
			}
			s.reverse()
			s.classify(&aiReverseScope{relatedRequests: map[string]bool{}, relatedIDs: map[string]bool{}, identityConflicts: map[string]bool{}})
			if s.report.Blocking == 0 {
				t.Fatal("invalid global edge hidden as unrelated")
			}
		})
	}
}
func TestAIReverseReceiptResourceDoesNotReplaceOperationIdentity(t *testing.T) {
	f := aiLocalGraph(t)
	s := aiReverseUnitSnapshot()
	n := s.decode(aiReverseSpecByTable("ai_messaging_operations"), aiReverseUnitRow(aiReverseSpecByTable("ai_messaging_operations"), f.ops[0]), aiReverseMetadata{})
	if n.resource != f.request.RequestID || n.receiptResource != f.projection.SessionID || n.receiptRun == "" {
		t.Fatal("original operation resource overwritten by receipt session")
	}
}
func TestAIReverseOutboxRowOrganizationMustMatchBody(t *testing.T) {
	f := aiLocalGraph(t)
	r := aiReverseUnitRow(aiReverseSpecByTable("ai_messaging_outbox"), f.boxes[0])
	r["organization_id"] = []byte("999")
	n := aiReverseUnitSnapshot().decode(aiReverseSpecByTable("ai_messaging_outbox"), r, aiReverseMetadata{})
	if !n.observation.Invalid {
		t.Fatal("body overwrote conflicting physical organization")
	}
}
func TestAIReverseUnknownProtoFieldsDoNotDisappear(t *testing.T) {
	v := &pb.MessagingBody{Value: &pb.MessagingBody_EventAcknowledgement{EventAcknowledgement: &pb.MessagingEventAcknowledgement{}}}
	v.GetEventAcknowledgement().ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01})
	if !aiReverseProtoUnknown(v.ProtoReflect()) {
		t.Fatal("nested unknown protobuf field silently omitted")
	}
	raw, e := proto.Marshal(v)
	if e != nil || len(raw) == 0 {
		t.Fatal("fixture")
	}
}
func TestAIReverseLimitsAndOpaqueZero(t *testing.T) {
	if (AIReverseLimits{}).valid() || !DefaultAIReverseLimits().valid() {
		t.Fatal("limits")
	}
	zero := &AIReverseSnapshot{}
	if zero.ValidateBorrowedSnapshot(context.Background()) == nil || zero.Summary().DropReady {
		t.Fatal("zero DTO became trusted snapshot")
	}
	if _, e := json.Marshal(zero); !errors.Is(e, ErrSourceSerialization) {
		t.Fatal("private object serialized")
	}
	if rows, next, e := zero.ObservationsPage(0, 1); e == nil || len(rows) != 0 || next != 0 {
		t.Fatal("zero exported row page")
	}
	s := aiReverseUnitSnapshot()
	copy := *s
	if copy.ValidateBorrowedSnapshot(context.Background()) == nil {
		t.Fatal("copied opaque handle accepted")
	}
	if e := (&HistoricalCoordinator{}).BindAIReverseSourceScope(context.Background(), s, nil); e == nil {
		t.Fatal("caller manufactured source closure")
	}
	if proof, e := zero.RecheckFresh(context.Background(), nil, 99, &HistoricalCoordinator{}, nil); e == nil || proof != nil {
		t.Fatal("zero coordinator/summary manufactured fresh source authority")
	}
	l := DefaultAIReverseLimits()
	l.MaxRetainedBytes = 1
	s.limits = l
	scope := &aiReverseScope{relatedIDs: map[string]bool{}}
	if e := s.scopeAdd(scope, scope.relatedIDs, aiFixtureRequestID); e == nil {
		t.Fatal("scope set exceeded retained budget")
	}
}
func TestAIReverseMetadataNULLAndUnsupportedFamilies(t *testing.T) {
	col := aiReverseRow{"name": []byte("body_sha256"), "type": []byte("char(64)"), "nullable": []byte("YES")}
	if !aiReverseSupportedColumn("ai_messaging_operations", col) {
		t.Fatal("supported retired nullable field rejected")
	}
	col["nullable"] = []byte("NO")
	if aiReverseSupportedColumn("ai_messaging_operations", col) {
		t.Fatal("NULL semantics silently changed")
	}
	col["name"] = []byte("future_unknown")
	if aiReverseSupportedColumn("ai_messaging_operations", col) {
		t.Fatal("unknown column supported")
	}
}

func aiReverseUnitMapping(t *testing.T, s *AIReverseSnapshot, attempts uint64) {
	t.Helper()
	old := aiFixtureRow(t, AIBridgeCommandSource, "start")
	old[6] = []byte(strconv.FormatUint(attempts, 10))
	legacy := aiFixtureRow(t, AILegacyCommandSource, "start")
	legacy[5] = old[6]
	legacy[7] = []byte{}
	legacy[8] = []byte(s.byTable["ai_messaging_operations"][aiFixtureRequestID].bodyHash)
	for table, row := range map[string][][]byte{AIBridgeCommandSource: old, AILegacyCommandSource: legacy} {
		spec := aiReverseSpecByTable(table)
		aiReverseUnitAdd(s, table, s.decode(spec, aiReverseUnitRow(spec, row), aiReverseMetadata{}))
	}
}
func TestAIReversePendingSessionRequiresOriginalAcceptedStart(t *testing.T) {
	for _, which := range []string{"bound", "unbound", "wrong_session", "wrong_receipt_family", "held_start"} {
		t.Run(which, func(t *testing.T) {
			s := aiReverseUnitGraph(t)
			r := s.byTable["ai_bridge_requests"][aiFixtureRequestID]
			r.projectionAbsent, r.version, r.event, r.state = true, 0, "", "pending"
			r.observation.Unfinished = true
			for id, n := range s.byTable["ai_messaging_inbox"] {
				if n.kind == pb.MessagingKind_INTERPRETATION_STATE {
					delete(s.byTable["ai_messaging_inbox"], id)
					delete(s.byTable["ai_messaging_outbox"], n.ack)
				}
			}
			s.byTable["ai_bridge_events"] = map[string]*aiReverseNode{}
			op := s.byTable["ai_messaging_operations"][r.id]
			switch which {
			case "unbound":
				r.resource = ""
				delete(s.byTable["ai_messaging_inbox"], op.receipt)
				s.byTable["ai_messaging_failures"] = map[string]*aiReverseNode{}
				delete(s.byTable["ai_messaging_outbox"], "80000000-0000-4000-8000-000000000001")
				op.state, op.receipt, op.linkedHash, op.receiptResource, op.receiptRun, op.receiptFamily = "", "", "", "", "", ""
				op.decision = pb.MessagingDecision_MESSAGING_DECISION_UNSPECIFIED
				op.observation.Unfinished = true
				box := s.byTable["ai_messaging_outbox"][r.id]
				box.state = "staged"
				box.observation.Unfinished = true
			case "wrong_session":
				r.resource = "90000000-0000-4000-8000-000000000001"
			case "wrong_receipt_family":
				op.receiptFamily = "evaluation"
			case "held_start":
				op.state = "held"
			}
			// Decode the actual NULL projection separately; it cannot by itself
			// reject the legitimate session identity-only receipt transition.
			f := aiLocalGraph(t)
			raw, e := json.Marshal(f.request)
			if e != nil {
				t.Fatal(e)
			}
			decoded := s.decode(aiReverseSpecByTable("ai_bridge_requests"), aiReverseRow{"request_id": []byte(r.id), "request_hash": []byte(sourceSHA(raw)), "payload": raw, "session_id": []byte(f.projection.SessionID), "version": []byte("0"), "status": []byte("pending")}, aiReverseMetadata{})
			if decoded.observation.Invalid || !decoded.projectionAbsent || !decoded.observation.Unfinished {
				t.Fatal("valid physical identity-only transition rejected during decode")
			}
			s.reverse()
			if which == "bound" || which == "unbound" {
				if r.observation.Invalid || !r.observation.Unfinished {
					t.Fatal("legitimate pending identity transition rejected or called terminal", r.observation.Reasons)
				}
			} else if !r.observation.Invalid {
				t.Fatal("unproven identity-only binding accepted")
			}
			s.classify(&aiReverseScope{relatedRequests: map[string]bool{r.id: true}, relatedIDs: map[string]bool{}, identityConflicts: map[string]bool{}})
			if s.report.Blocking == 0 {
				t.Fatal("pending request lost its target responsibility")
			}
		})
	}
}
func TestAIReverseLegacyResponsibilityMovesOnlyWithExactMapping(t *testing.T) {
	for _, which := range []string{"mapped", "unmapped", "budget_reset", "raw_payload_changed", "source_clock_changed", "original_time_changed", "missing_live_operation", "held_current"} {
		t.Run(which, func(t *testing.T) {
			s := aiReverseUnitGraph(t)
			aiReverseUnitMapping(t, s, 3)
			old, legacy := s.byTable[AIBridgeCommandSource][aiFixtureRequestID], s.byTable[AILegacyCommandSource][aiFixtureRequestID]
			box := s.byTable["ai_messaging_outbox"][aiFixtureRequestID]
			switch which {
			case "unmapped":
				delete(s.byTable[AILegacyCommandSource], legacy.id)
			case "budget_reset":
				box.attempts = 2
			case "raw_payload_changed":
				legacy.payloadSHA = strings.Repeat("b", 64)
			case "source_clock_changed":
				legacy.sourceClock = "2026-10-08 11:12:14.123456"
			case "original_time_changed":
				legacy.originalClock = "2026-10-08T11:12:12.123456+08:00"
			case "missing_live_operation":
				delete(s.byTable["ai_messaging_operations"], old.id)
			case "held_current":
				box.observation.Held, box.observation.Unfinished = true, true
			}
			s.reverse()
			s.classify(&aiReverseScope{relatedRequests: map[string]bool{aiFixtureRequestID: true}, relatedIDs: map[string]bool{}, identityConflicts: map[string]bool{}})
			if which == "mapped" {
				if old.delivered || old.observation.Unfinished || old.observation.Invalid || s.report.Blocking != 0 {
					t.Fatal("exact handoff invented delivery or retained old transport pending", s.report.BlockingReasons)
				}
			} else if which == "held_current" {
				if old.observation.Unfinished || !box.observation.Held || !box.observation.Unfinished || s.report.Blocking == 0 {
					t.Fatal("handoff erased current hold/pending responsibility")
				}
			} else if s.report.Blocking == 0 || !old.observation.Unfinished {
				t.Fatal("broken/unmapped handoff cleared original responsibility", s.report.BlockingReasons)
			}
		})
	}
}
func TestAIReverseLegacyOriginalTimeUsesActualImmutableRequestClock(t *testing.T) {
	r := &aiReverseNode{createdClock: "2026-10-08 03:12:12.123456"}
	legacy := &aiReverseNode{sourceKind: "start", originalClock: "2026-10-08T11:12:12.123456+08:00"}
	if !aiReverseLegacyTimeMatches(legacy, r) {
		t.Fatal("actual UTC SQL clock mapping rejected")
	}
	r.createdClock = "2026-10-08 03:12:13.123456"
	if aiReverseLegacyTimeMatches(legacy, r) {
		t.Fatal("retry/nearest time substituted for immutable clock")
	}
	r.createdClock = ""
	if aiReverseLegacyTimeMatches(legacy, r) {
		t.Fatal("NULL original clock became inferred timestamp")
	}
	legacy.originalClock = ""
	if !aiReverseLegacyTimeMatches(legacy, r) {
		t.Fatal("known original NULL failed exact empty mapping")
	}
}

type aiReverseEvaluationFixture struct {
	aggregate    string
	op           [][]byte
	boxes, inbox [][][]byte
}

func aiReverseEvaluationGraph(t *testing.T, kind pb.MessagingKind, shape string) aiReverseEvaluationFixture {
	t.Helper()
	run := "90000000-0000-4000-8000-000000000001"
	commandID := "90000000-0000-4000-8000-000000000002"
	receiptID := "90000000-0000-4000-8000-000000000003"
	ackID := "90000000-0000-4000-8000-000000000004"
	key := func(id string) jose.JSONWebKey {
		v, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal("synthetic native fixture key")
		}
		return jose.JSONWebKey{Key: v, KeyID: id}
	}
	sign, encrypt := key("reverse-sign"), key("reverse-encrypt")
	protect := func(k pb.MessagingKind, id, correlation string, b *pb.MessagingBody) *app.PreparedMessaging {
		v, e := app.ProtectMessaging(k, id, run, correlation, "1", "", b, sign, encrypt.Public())
		if e != nil {
			t.Fatal("synthetic original envelope", e)
		}
		return v
	}
	scope := &pb.EvaluationQuery{RunId: run, OrganizationId: 1, OperatorUserId: 42}
	body := &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationStart{EvaluationStart: &pb.EvaluationStartCommand{Scope: scope, ExpectedVersion: 1, Reason: "frozen fixture", Confirm: true}}}
	if kind == pb.MessagingKind_EVALUATION_CANCEL {
		body = &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationCancel{EvaluationCancel: &pb.EvaluationCancelCommand{Scope: scope, ExpectedVersion: 1, Discard: proto.Bool(false), Reason: "frozen fixture", Confirm: true}}}
	}
	command := protect(kind, commandID, "", body)
	receipt := &pb.MessagingCommandReceipt{CommandId: commandID, CommandBodySha256: command.Envelope.BodySha256, Decision: pb.MessagingDecision_ACCEPTED, Code: "accepted", OriginalReceipt: &pb.MessagingCommandReceipt_EvaluationReceipt{EvaluationReceipt: &pb.EvaluationState{RunId: run, Version: 1}}}
	if shape == "wrong_run" {
		receipt.GetEvaluationReceipt().RunId = "90000000-0000-4000-8000-000000000005"
	}
	if shape == "workflow" {
		receipt.OriginalReceipt = &pb.MessagingCommandReceipt_WorkflowReceipt{WorkflowReceipt: &pb.Receipt{SessionId: run, RunId: run, Version: 1, Status: "queued"}}
	}
	decision := "accepted"
	if shape == "rejected" {
		decision = "rejected"
		receipt.Decision = pb.MessagingDecision_REJECTED
		receipt.Code = "rejected"
		receipt.OriginalReceipt = nil
	}
	received := protect(pb.MessagingKind_COMMAND_RECEIPT, receiptID, commandID, &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: receipt}})
	ack := protect(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, ackID, "", &pb.MessagingBody{Value: &pb.MessagingBody_EventAcknowledgement{EventAcknowledgement: &pb.MessagingEventAcknowledgement{EventId: receiptID, EventBodySha256: received.Envelope.BodySha256, EventKind: pb.MessagingKind_COMMAND_RECEIPT, Outcome: pb.MessagingEventAcknowledgement_STORED}}})
	stamp := []byte("2026-10-08 11:12:14.123456")
	box := func(m *app.PreparedMessaging, ordered bool) [][]byte {
		value := []byte("0")
		if ordered {
			value = []byte("1")
		}
		return [][]byte{[]byte("qs-server"), []byte("qs-ai"), []byte(m.Envelope.MessageId), []byte(m.Envelope.BodySha256), m.Body, m.Wire, []byte(sourceSHA(m.Wire)), []byte(strconv.Itoa(int(m.Envelope.Kind))), []byte("1"), []byte(m.Topic), []byte(run), []byte("1"), value, value, []byte("confirmed"), []byte("1"), stamp, stamp, stamp, stamp, []byte{}}
	}
	return aiReverseEvaluationFixture{run, [][]byte{[]byte(commandID), []byte(strconv.Itoa(int(kind))), []byte(command.Envelope.BodySha256), []byte("1"), []byte("42"), []byte(run), []byte(run), []byte("1"), []byte(decision), []byte(receipt.Code), []byte(receiptID), received.Body, stamp, stamp, []byte("0"), nil, nil}, [][][]byte{box(command, true), box(ack, false)}, [][][]byte{{[]byte("qs-ai"), []byte(receiptID), []byte(received.Envelope.BodySha256), received.Body, []byte(sourceSHA(received.Wire)), []byte(strconv.Itoa(int(pb.MessagingKind_COMMAND_RECEIPT))), []byte(run), []byte(ackID), stamp, []byte("stored")}}}
}
func aiReverseUnitEvaluation(t *testing.T, kind pb.MessagingKind, shape string) *AIReverseSnapshot {
	t.Helper()
	s := aiReverseUnitSnapshot()
	f := aiReverseEvaluationGraph(t, kind, shape)
	for table, rows := range map[string][][][]byte{"ai_messaging_operations": {f.op}, "ai_messaging_outbox": f.boxes, "ai_messaging_inbox": f.inbox} {
		spec := aiReverseSpecByTable(table)
		for _, row := range rows {
			aiReverseUnitAdd(s, table, s.decode(spec, aiReverseUnitRow(spec, row), aiReverseMetadata{}))
		}
	}
	aiReverseUnitAdd(s, "ai_messaging_aggregates", &aiReverseNode{id: f.aggregate, aggregate: f.aggregate, sequence: 2})
	aiReverseUnitAdd(s, "ai_messaging_admission", &aiReverseNode{id: "1", state: "closed"})
	for kind := range aiReverseObservationKinds {
		aiReverseUnitAdd(s, "ai_messaging_observations", &aiReverseNode{id: kind})
	}
	return s
}
func TestAIReverseEvaluationReceiptRequiresOriginalKindAndRun(t *testing.T) {
	for _, kind := range []pb.MessagingKind{pb.MessagingKind_EVALUATION_START, pb.MessagingKind_EVALUATION_CANCEL} {
		for _, shape := range []string{"exact", "wrong_run", "workflow", "rejected"} {
			t.Run(kind.String()+"/"+shape, func(t *testing.T) {
				s := aiReverseUnitEvaluation(t, kind, shape)
				s.reverse()
				s.classify(&aiReverseScope{relatedRequests: map[string]bool{}, relatedIDs: map[string]bool{}, identityConflicts: map[string]bool{}})
				if shape == "exact" || shape == "rejected" {
					if s.report.Blocking != 0 {
						t.Fatal("valid original evaluation/rejection ledger rejected", s.report.BlockingReasons)
					}
				} else if s.report.Blocking == 0 {
					t.Fatal("outside source scope hid wrong receipt family/original run")
				}
				if s.report.GlobalReverseQualified || s.report.DropReady || s.report.CASAuthority {
					t.Fatal("local original run verification fabricated authority")
				}
			})
		}
	}
}

// This structural assertion prevents future compaction from quietly retaining
// first-epoch million-row maps through an indirect field. The only reference
// capability allowed here is the actual borrowed pool and the self guard.
func TestAIReverseGraphlessAnchorHasNoFirstEpochGraphFields(t *testing.T) {
	root := reflect.TypeOf(AIReverseRecheckAnchor{})
	seen := map[reflect.Type]bool{}
	var visit func(reflect.Type)
	visit = func(v reflect.Type) {
		if seen[v] {
			return
		}
		seen[v] = true
		if v == reflect.TypeOf(time.Time{}) {
			return
		}
		switch v.Kind() {
		case reflect.Interface:
			if v != reflect.TypeOf((*gorm.ConnPool)(nil)).Elem() {
				t.Fatal("anchor retains unbounded interface", v.String())
			}
			return
		case reflect.Pointer:
			if v != reflect.PointerTo(root) {
				t.Fatal("anchor retains a first-epoch graph pointer", v.String())
			}
			return
		case reflect.Map, reflect.Func, reflect.Chan, reflect.UnsafePointer:
			t.Fatal("anchor retains graph/closure", v.String())
		case reflect.Array, reflect.Slice:
			visit(v.Elem())
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				visit(v.Field(i).Type)
			}
		}
	}
	visit(root)
}
func aiReverseUnitStreamFactsSHA(t *testing.T, f authFixture, auth *VerifiedSourceCopies, change string) string {
	t.Helper()
	h := sha256.New()
	sourceFrame(h, []byte("ai-reverse-authenticated-source-facts/v1"), false)
	sourceFrame(h, []byte(coordinatorBinding().SourceSHA), false)
	sourceFrame(h, []byte(coordinatorBinding().OperationID), false)
	type fact struct{ database, object, pk, kind string }
	rows := [4]fact{{f.sql.Source.Database, f.sql.Source.Object, f.sql.Source.PrimaryKeySHA256, f.sql.EventType}, {f.bridge.Source.Database, f.bridge.Source.Object, f.bridge.Source.PrimaryKeySHA256, "ai-command/" + f.bridge.SourceKind}, {f.legacy.Source.Database, f.legacy.Source.Object, f.legacy.Source.PrimaryKeySHA256, "ai-command/" + f.legacy.SourceKind}, {f.mongo.Source.Database, f.mongo.Source.Object, f.mongo.Source.PrimaryKeySHA256, f.mongo.EventType}}
	for i, expectation := range f.expected {
		if change == "boundary" && i == 0 {
			expectation.Boundary.UpperToken = "changed-fixed-boundary"
		}
		raw, e := json.Marshal(expectation)
		if e != nil {
			t.Fatal(e)
		}
		sourceFrame(h, []byte(strconv.Itoa(i)), false)
		sourceFrame(h, raw, false)
		r := rows[i]
		if change == "event_type" && i == 0 {
			r.kind = "evaluation.requested"
		}
		if e = aiReverseSourceFactFrame(h, auth, r.database, r.object, r.pk, r.kind); e != nil {
			t.Fatal(e)
		}
		receipt := auth.receipts[i]
		if change == "receipt" && i == 0 {
			receipt.DataHash = strings.Repeat("d", 64)
		}
		raw, e = json.Marshal(receipt)
		if e != nil {
			t.Fatal(e)
		}
		sourceFrame(h, raw, false)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func TestAIReverseStreamingFactsSealUsesAuthenticatedPKTypedFactsAndAllFourMetadata(t *testing.T) {
	f := sourceAuthFixture(t, true, true)
	auth, e := VerifySourceCopies(t.Context(), f.inputs())
	if e != nil {
		t.Fatal("actual four-copy EOF fixture", e)
	}
	if _, e = auth.BindEvent(f.sql); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.BindEvent(f.mongo); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.BindAICommand(f.bridge); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.BindAICommand(f.legacy); e != nil {
		t.Fatal(e)
	}
	original := aiReverseUnitStreamFactsSHA(t, f, auth, "")
	if original != aiReverseUnitStreamFactsSHA(t, f, auth, "") {
		t.Fatal("same authenticated stream facts changed")
	}
	for _, change := range []string{"boundary", "event_type", "receipt"} {
		if original == aiReverseUnitStreamFactsSHA(t, f, auth, change) {
			t.Fatal("streaming facts lost metadata/type", change)
		}
	}
	key, e := sourceAuthKey(f.sql.Source.Database, f.sql.Source.Object, f.sql.Source.PrimaryKeySHA256)
	if e != nil {
		t.Fatal(e)
	}
	old := auth.rows[key]
	changed := old
	changed.facts[0] ^= 1
	auth.rows[key] = changed
	if original == aiReverseUnitStreamFactsSHA(t, f, auth, "") {
		t.Fatal("actual private typed facts omitted")
	}
	auth.rows[key] = old
	if e = aiReverseSourceFactFrame(sha256.New(), auth, f.sql.Source.Database, f.sql.Source.Object, strings.Repeat("f", 64), f.sql.EventType); e == nil {
		t.Fatal("unobserved primary key framed as authenticated")
	}
	auth.complete = false
	if e = aiReverseSourceFactFrame(sha256.New(), auth, f.sql.Source.Database, f.sql.Source.Object, f.sql.Source.PrimaryKeySHA256, f.sql.EventType); e == nil {
		t.Fatal("incomplete four-copy handle framed")
	}
}
func TestAIReverseClassificationSealDetectsSameCountDifferentObservation(t *testing.T) {
	s := aiReverseUnitSnapshot()
	aiReverseUnitAdd(s, "ai_messaging_observations", &aiReverseNode{id: "a"})
	aiReverseUnitAdd(s, "ai_messaging_observations", &aiReverseNode{id: "b"})
	s.report.WholeLedgerEOF = true
	s.report.Rows = 2
	s.scope = &aiReverseScope{}
	s.nodes[0].observation.Scope = "retirement_related"
	s.nodes[1].observation.Scope = "outside_retirement"
	a, e := aiReverseClassificationSHA(s)
	if e != nil {
		t.Fatal(e)
	}
	s.nodes[0].observation.Scope, s.nodes[1].observation.Scope = s.nodes[1].observation.Scope, s.nodes[0].observation.Scope
	b, e := aiReverseClassificationSHA(s)
	if e != nil || a == b {
		t.Fatal("equal aggregate counts hid changed row classifications")
	}
	s.nodes[0].observation.Held = true
	c, e := aiReverseClassificationSHA(s)
	if e != nil || c == b {
		t.Fatal("hold disappeared from sealed observation")
	}
	s.nodes[0].observation.Reasons = []string{"original_receipt_identity_unproven"}
	d, e := aiReverseClassificationSHA(s)
	if e != nil || d == c {
		t.Fatal("reason disappeared from sealed observation")
	}
	s.report.Rows = 1
	if _, e = aiReverseClassificationSHA(s); e == nil {
		t.Fatal("partial row seal accepted")
	}
}

// This private metadata fixture intentionally has no live native transaction
// and can test only sealing/deadline mechanics. Production Freeze still needs
// actual borrowed paired epochs, real four EOF and whole physical scans.
func aiReverseUnitCompactAnchor(t *testing.T) *AIReverseRecheckAnchor {
	t.Helper()
	now := time.Now()
	limits := DefaultAIReverseLimits()
	originLimits := DefaultSourceOriginLimits()
	originLimits.MaxDuration = 90 * time.Second
	a := &AIReverseRecheckAnchor{pool: &sql.Tx{}, started: now.Add(-20 * time.Second), sealedAt: now, coordinatorStarted: now.Add(-20 * time.Second), coordinatorDuration: 120 * time.Second, limits: limits, head: 99, typedFactsSHA: strings.Repeat("a", 64), classificationSHA: strings.Repeat("b", 64), sqlCycle: "actual-cycle-fixture", ai: AIReverseSummary{Ledgers: make([]AIReverseLedgerSummary, 14), WholeLedgerEOF: true}, origin: aiReverseOriginSeal{Limits: originLimits, Started: now.Add(-15 * time.Second), Transaction: mongoCycleTxn{session: []byte("private-fixture-lsid"), number: 1}}}
	a.self = a
	var e error
	a.poolToken, e = aiReversePoolToken(a.pool)
	if e != nil {
		t.Fatal(e)
	}
	a.seal, e = a.factsDigest()
	if e != nil || !a.intact() {
		t.Fatal("private bounded metadata seal")
	}
	return a
}
func TestAIReverseCompactAnchorRejectsCopyTamperAndOriginalDeadlineReset(t *testing.T) {
	a := aiReverseUnitCompactAnchor(t)
	if _, e := json.Marshal(a); !errors.Is(e, ErrSourceSerialization) {
		t.Fatal("private anchor serialized")
	}
	copy := *a
	if copy.intact() {
		t.Fatal("copied metadata anchor accepted")
	}
	old := a.origin.Transaction.session[0]
	a.origin.Transaction.session[0] ^= 1
	if a.intact() {
		t.Fatal("actual transaction token omitted from seal")
	}
	a.origin.Transaction.session[0] = old
	scope, cancel, e := a.recheckContext(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	deadline, ok := scope.Deadline()
	cancel()
	expected := a.origin.Started.Add(a.origin.Limits.MaxDuration)
	if !ok || !deadline.Equal(expected) {
		t.Fatal("fresh scope reset original absolute budget")
	}
	parent, parentCancel := context.WithCancel(t.Context())
	scope, cancel, e = a.recheckContext(parent)
	if e != nil {
		t.Fatal(e)
	}
	parentCancel()
	if scope.Err() == nil {
		t.Fatal("fresh scope detached parent cancellation")
	}
	cancel()
	a.origin.Started = time.Now().Add(-2 * time.Minute)
	a.seal, e = a.factsDigest()
	if e != nil {
		t.Fatal(e)
	}
	if scope, cancel, e = a.recheckContext(t.Context()); e == nil || scope != nil || cancel != nil {
		t.Fatal("expired original origin budget restarted")
	}
	zero := &AIReverseRecheckAnchor{}
	if zero.intact() {
		t.Fatal("zero metadata anchor accepted")
	}
	if p, e := zero.RecheckFresh(t.Context(), nil, 99, &HistoricalCoordinator{}, nil); e == nil || p != nil {
		t.Fatal("zero summary minted actual fresh proof")
	}
	if p, e := zero.RecheckOrigin(t.Context(), &AIReverseFreshProof{}, nil, nil, &HistoricalCoordinator{}, nil); e == nil || p != nil {
		t.Fatal("zero fresh DTO minted origin proof")
	}
}

// This uses the actual four-EOF coordinator constructor, but only the private
// metadata checker: no native snapshot/proof/execution permission is minted.
func aiReverseUnitCurrentOwner(t *testing.T) (*AIReverseRecheckAnchor, *HistoricalCoordinator) {
	t.Helper()
	a := aiReverseUnitCompactAnchor(t)
	f := sourceAuthFixture(t, true, true)
	c, e := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if e != nil {
		t.Fatal("actual complete-copy owner fixture", e)
	}
	a.binding = c.binding
	a.expected = f.expected
	a.receipts = c.authenticated.receipts
	a.entries = c.authenticated.entries
	a.seal, e = a.factsDigest()
	if e != nil || !a.intact() {
		t.Fatal("private metadata fixture seal")
	}
	return a, c
}
func TestAIReverseCurrentCoordinatorFailedOrWrongBindingRejected(t *testing.T) {
	for _, which := range []string{"valid_metadata_only", "failed", "old_constructor", "binding", "expected", "incomplete_auth", "cardinality", "receipts", "nil_clock"} {
		t.Run(which, func(t *testing.T) {
			a, c := aiReverseUnitCurrentOwner(t)
			c.mu.Lock()
			switch which {
			case "failed":
				c.failed = true
			case "old_constructor":
				c.started = a.sealedAt
			case "binding":
				c.binding.OperationID = "987-1"
			case "expected":
				c.copies[0].Expected.Boundary.UpperToken = "changed-original-boundary"
			case "incomplete_auth":
				c.authenticated.complete = false
			case "cardinality":
				c.authenticated.entries++
			case "receipts":
				c.authenticated.receipts[0].DataHash = strings.Repeat("f", 64)
			case "nil_clock":
				c.now = nil
			}
			c.mu.Unlock()
			e := a.currentCoordinatorState(t.Context(), c)
			if which == "valid_metadata_only" {
				if e != nil {
					t.Fatal("same authenticated metadata rejected")
				}
			} else if !errors.Is(e, ErrAIReverseBinding) {
				t.Fatal("failed/unbound owner accepted", which)
			}
		})
	}
}
func TestAIReverseCurrentCoordinatorInspectionUsesOwnerMutex(t *testing.T) {
	a, c := aiReverseUnitCurrentOwner(t)
	c.mu.Lock()
	held := true
	defer func() {
		if held {
			c.mu.Unlock()
		}
	}()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() { close(started); done <- a.currentCoordinatorState(t.Context(), c) }()
	<-started
	select {
	case <-done:
		t.Fatal("private owner state inspected outside its held mutex")
	case <-time.After(50 * time.Millisecond):
	}
	c.failed = true
	c.mu.Unlock()
	held = false
	select {
	case e := <-done:
		if !errors.Is(e, ErrAIReverseBinding) {
			t.Fatal("concurrent failed owner returned valid metadata")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("owner inspection deadlocked after releasing its own mutex")
	}
	// A terminal failure is never reset. Repeated concurrent readers must all
	// observe rejection and leave the constructor clock/TTL/binding unchanged.
	before := c.started
	var wg sync.WaitGroup
	results := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- a.currentCoordinatorState(t.Context(), c) }()
	}
	wg.Wait()
	close(results)
	for e := range results {
		if !errors.Is(e, ErrAIReverseBinding) {
			t.Fatal("failed owner accepted by a concurrent reader")
		}
	}
	c.mu.Lock()
	unchanged := c.failed && c.started.Equal(before)
	c.mu.Unlock()
	if !unchanged {
		t.Fatal("metadata check adopted failed state or reset original budget")
	}
}
