package evaluation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"go.mongodb.org/mongo-driver/bson"
)

func cycleTestRow(values map[string]string) historicalSQLRow {
	row := historicalSQLRow{}
	for key, value := range values {
		copy := value
		row[key] = &copy
	}
	return row
}
func pointerValue(value string) *string { return &value }
func cycleTestMessage(t *testing.T, id, eventType, aggregateType, aggregateID string, data any) historicalSQLRow {
	t.Helper()
	at := time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC)
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := json.Marshal(domainwire.Envelope{ID: id, EventType: eventType, OccurredAt: at, AggregateType: aggregateType, AggregateID: aggregateID, Data: body})
	if err != nil {
		t.Fatal(err)
	}
	outer, err := legacy.Encode(legacy.Envelope{UUID: id, Payload: inner, Metadata: map[string]string{"event_type": eventType, "aggregate_type": aggregateType, "aggregate_id": aggregateID, "source": "qs-server", "occurred_at": at.Format(domainwire.OccurredAtLayout)}}, legacy.Revision2)
	if err != nil {
		t.Fatal(err)
	}
	in := message.Input{Producer: "qs-server", ID: id, Destination: "qs.evaluation.lifecycle", EventType: eventType, SchemaVersion: "v1", Scope: "org:7", ContentType: "application/json", OccurredAt: at.Format(time.RFC3339Nano), Payload: outer}
	msg, err := message.New(in)
	if err != nil {
		t.Fatal(err)
	}
	fp := msg.Fingerprint()
	return cycleTestRow(map[string]string{"id": "1", "producer": in.Producer, "message_id": in.ID, "destination": in.Destination, "event_type": in.EventType, "schema_version": in.SchemaVersion, "scope": in.Scope, "content_type": in.ContentType, "occurred_at": in.OccurredAt, "payload": string(in.Payload), "fingerprint": string(fp[:]), "state": "published", "version": "0", "transport_confirmed_at": "2026-10-08 01:02:03.456"})
}

func cycleRequestedPayload() eventpayload.EvaluationRequestedData {
	return eventpayload.EvaluationRequestedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, QuestionnaireCode: "Q", QuestionnaireVer: "1.0", AnswerSheetID: "10042", ModelKind: "scale", ModelAlgorithm: "scale_scoring", ModelCode: "S", ModelVersion: "1.0", RequestedAt: time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC)}
}

func TestSQLResponsibilityCycleActualWireAndSeparateDigests(t *testing.T) {
	row := cycleTestMessage(t, "current-one", "evaluation.requested", "Evaluation", "42", cycleRequestedPayload())
	v := cycleDecode("rm_outbox", row)
	if v.Invalid || v.Unfinished || v.AssessmentID != 42 || v.OrgID != 7 || v.SDKFingerprintSHA256 == "" || v.payloadSHA256 == "" {
		t.Fatal("actual standard wire rejected or bytes layers missing")
	}
	columns := []string{"fingerprint", "payload", "version"}
	digest := cycleRowDigest(columns, row)
	row["version"] = nil
	if digest == cycleRowDigest(columns, row) {
		t.Fatal("NULL lost in original row digest")
	}
	row["version"] = new(string)
	if digest == cycleRowDigest(columns, row) {
		t.Fatal("empty value lost in original row digest")
	}
	row["payload"] = pointerValue(strings.Replace(valueOrEmpty(row["payload"]), "current-one", "current-two", 1))
	if !cycleDecode("rm_outbox", row).Invalid {
		t.Fatal("tampered outer ID accepted")
	}
}

func TestSQLResponsibilityCycleCurrentOtherEventOutsideScope(t *testing.T) {
	p := eventpayload.TaskOpenedReminderRequestedData{TaskID: "task-1", PlanID: "plan-1", OrgID: 7, TesteeID: "21", OpenAt: time.Now().UTC(), ScheduleRevision: 1}
	row := cycleTestMessage(t, "task-reminder", "task.opened.reminder.requested", "AssessmentTask", "task-1", p)
	row["state"] = pointerValue("pending")
	row["transport_confirmed_at"] = nil
	v := cycleDecode("rm_outbox", row)
	if v.Invalid || !v.Unfinished || v.ScopeClass != "scope_outside_retirement" || !v.OwnerUnproven {
		t.Fatal("legal current task confused with retired chain")
	}
	c := &SQLHistoricalResponsibilityCycle{observations: []SQLResponsibilityObservation{v}, owners: map[uint64]sqlResponsibilityOwner{}, byOwner: map[uint64][]int{}, byEvent: map[string][]int{}, byOrgActions: map[uint64][]int{}}
	c.checkReverse()
	if c.report.Blocking != 0 || c.report.OutsideRetirement != 1 || c.UnboundEventCount() != 1 {
		t.Fatal("scope or external owner boundary lost")
	}
}

func TestSQLResponsibilityCycleUnknownAndDuplicateReferences(t *testing.T) {
	row := cycleTestMessage(t, "event-one", "unknown.event", "Evaluation", "42", cycleRequestedPayload())
	if !cycleDecode("rm_outbox", row).Invalid {
		t.Fatal("unknown current type ignored")
	}
	v := cycleDecode("rm_outbox", cycleTestMessage(t, "current-one", "evaluation.requested", "Evaluation", "42", cycleRequestedPayload()))
	copied := v
	copied.OrgID = 8
	ref := SQLResponsibilityObservation{Store: "qs_rm_evaluation_request_ref", EventID: v.EventID, OrgID: 7, AssessmentID: 42, ScopeClass: "retirement_related"}
	c := &SQLHistoricalResponsibilityCycle{observations: []SQLResponsibilityObservation{v, copied, ref}, owners: map[uint64]sqlResponsibilityOwner{}, byOwner: map[uint64][]int{}, byEvent: map[string][]int{}, byOrgActions: map[uint64][]int{}}
	c.checkReverse()
	if !c.observations[0].Invalid || !c.observations[1].Invalid || !c.observations[2].Invalid {
		t.Fatal("duplicate current identity hidden")
	}
	values := c.ForEvent(v.EventID)
	values[0].Reasons[0] = "edited"
	if c.ForEvent(v.EventID)[0].Reasons[0] == "edited" {
		t.Fatal("editable lookup modified private cycle")
	}
}

func TestSQLResponsibilityCycleCompositeKeyAndFraming(t *testing.T) {
	spec := sqlResponsibilityTables[6]
	keys := [][]string{{"7", "A", "0"}, {"7", "A", "1"}, {"7", "a", "0"}, {"7", "a\x00", "0"}, {"8", "A", "0"}}
	for i := 1; i < len(keys); i++ {
		if cycleCompare(spec, keys[i-1], keys[i]) >= 0 {
			t.Fatal("actual typed PK order lost")
		}
	}
	if cycleKeyDigest([]string{"a", "bc"}) == cycleKeyDigest([]string{"ab", "c"}) {
		t.Fatal("unframed composite key hash")
	}
	for _, row := range []historicalSQLRow{cycleTestRow(map[string]string{"org_id": "07", "request_id": "A", "ordinal": "0"}), cycleTestRow(map[string]string{"org_id": "7", "request_id": "", "ordinal": "0"})} {
		if _, err := cycleKey(spec, row); err == nil {
			t.Fatal("invalid actual PK accepted")
		}
	}
	if !DefaultSQLResponsibilityLimits().valid() || (SQLResponsibilityLimits{PageRows: 4097, MaxRows: 1, MaxBytes: 1, MaxRetainedBytes: 1}).valid() {
		t.Fatal("unsafe page budget")
	}
}

func TestSQLResponsibilityCyclePrivateCannotSerialize(t *testing.T) {
	for _, value := range []any{&SQLHistoricalResponsibilityCycle{}, SQLResponsibilityObservation{EventID: "private"}} {
		if _, err := json.Marshal(value); !errors.Is(err, ErrSQLHistoricalFactsSerialization) {
			t.Fatal("private SQL JSON leaked")
		}
		if _, err := bson.Marshal(value); !errors.Is(err, ErrSQLHistoricalFactsSerialization) {
			t.Fatal("private SQL BSON leaked")
		}
	}
}

func TestSQLResponsibilityCycleDDLOnlyAllocationCounterExcluded(t *testing.T) {
	a := "CREATE TABLE `synthetic` (\n `id` bigint NOT NULL\n) ENGINE=InnoDB AUTO_INCREMENT=12 DEFAULT CHARSET=utf8mb4 COMMENT='AUTO_INCREMENT=secret'"
	b := strings.Replace(a, "AUTO_INCREMENT=12", "AUTO_INCREMENT=123", 1)
	first, e := cycleNormalizeDDL(a)
	second, f := cycleNormalizeDDL(b)
	if e != nil || f != nil || first != second || !strings.Contains(first, "AUTO_INCREMENT=secret") {
		t.Fatal("DDL normalization removed more than allocation counter")
	}
	c := strings.Replace(a, "bigint NOT NULL", "bigint NOT NULL CHECK (`id` >= 0)", 1)
	changed, e := cycleNormalizeDDL(c)
	if e != nil || changed == first {
		t.Fatal("CHECK/DDL text omitted from schema identity")
	}
	if _, e := cycleNormalizeDDL("unrecognized DDL"); !errors.Is(e, ErrSQLResponsibilitySchema) {
		t.Fatal("unknown SHOW CREATE framing accepted")
	}
}
