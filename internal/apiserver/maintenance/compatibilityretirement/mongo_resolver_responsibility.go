package retirement

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"go.mongodb.org/mongo-driver/bson"
)

type mongoOwnerStandardRow struct {
	ID                    bson.Raw   `bson:"_id"`
	Producer              string     `bson:"producer"`
	MessageID             string     `bson:"message_id"`
	Destination           string     `bson:"destination"`
	EventType             string     `bson:"event_type"`
	SchemaVersion         string     `bson:"schema_version"`
	Scope                 string     `bson:"scope"`
	ContentType           string     `bson:"content_type"`
	OccurredAt            string     `bson:"occurred_at"`
	Payload               []byte     `bson:"payload"`
	Fingerprint           []byte     `bson:"fingerprint"`
	State                 string     `bson:"state"`
	ManualReplayRequestID string     `bson:"manual_replay_request_id"`
	ManualReplayVersion   uint64     `bson:"manual_replay_version"`
	Version               uint64     `bson:"version"`
	FailureCount          uint64     `bson:"failure_count"`
	TransportConfirmedAt  *time.Time `bson:"transport_confirmed_at"`
}

func (row mongoOwnerStandardRow) envelope() (*domainwire.Envelope, error) {
	fields, err := exactBSONFields(row.ID)
	if err != nil || len(fields) != 3 {
		return nil, ErrMongoOwnerConflict
	}
	for key, value := range map[string]string{"producer": row.Producer, "message_id": row.MessageID, "destination": row.Destination} {
		raw, ok := fields[key]
		if !ok || raw.Type != bson.TypeString || raw.StringValue() != value {
			return nil, ErrMongoOwnerConflict
		}
	}
	in := message.Input{Producer: row.Producer, ID: row.MessageID, Destination: row.Destination, EventType: row.EventType, SchemaVersion: row.SchemaVersion, Scope: row.Scope, ContentType: row.ContentType, OccurredAt: row.OccurredAt, Payload: row.Payload}
	msg, err := message.New(in)
	if err != nil || len(row.Fingerprint) != 32 {
		return nil, ErrMongoOwnerConflict
	}
	inner, err := standard.VerifyReference(in, hex.EncodeToString(row.Fingerprint), standard.ReferenceFromMessage(msg))
	if err != nil {
		return nil, ErrMongoOwnerConflict
	}
	outer, recognized, err := legacy.Decode(row.Payload)
	if err != nil || !recognized || strictJSON(outer.Payload) != nil || strictJSON(inner.Data) != nil {
		return nil, ErrMongoOwnerConflict
	}
	return inner, nil
}

// An organization-wide bounded read detects reverse candidates whose outer
// event type or identity has drifted. Other-organization/unbound rows still
// require the coordinator's separate global scan. Exceeding bounds is failure.
func (r *MongoOwnerResolution) readMongoResponsibilities(ctx context.Context) error {
	rows, err := r.readRows(ctx, "rm_outbox", bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "scope", Value: "org:" + strconv.FormatUint(r.source.OrgID, 10)}}, bson.D{{Key: "message_id", Value: r.source.EventID}}}}})
	if err != nil {
		return err
	}
	relevant := map[string]mongoOwnerStandardRow{}
	for _, raw := range rows {
		var row mongoOwnerStandardRow
		if bson.Unmarshal(raw, &row) != nil {
			return ErrMongoOwnerConflict
		}
		inner, err := row.envelope()
		if err != nil {
			return err
		}
		covered, err := r.mongoEnvelopeCovered(inner)
		if err != nil {
			return err
		}
		if !covered {
			if row.MessageID == r.source.EventID {
				return ErrMongoOwnerConflict
			}
			continue
		}
		if row.Scope != "org:"+strconv.FormatUint(r.local.OrgID, 10) {
			return ErrSourceOrganization
		}
		if _, exists := relevant[row.MessageID]; exists {
			return ErrMongoOwnerConflict
		}
		relevant[row.MessageID] = row
		r.local.CurrentResponsibilityCount++
		fields, _ := exactBSONFields(raw)
		for _, key := range []string{"version", "failure_count", "attempt_count", "manual_replay_version"} {
			if value, exists := fields[key]; exists {
				n, ok := mongoExactInteger(value)
				if !ok || n < 0 {
					return ErrMongoOwnerConflict
				}
			}
		}
		_, lease := fields["lease_until"]
		_, claim := fields["claim_token"]
		switch row.State {
		case "pending", "retry_wait", "publishing", "quarantined":
			r.block("mongo_outbox_transport_or_manual_responsibility_unclosed")
		case "published":
			if row.Version == 0 || row.TransportConfirmedAt == nil || row.TransportConfirmedAt.IsZero() || fields["transport_confirmed_at"].Type != bson.TypeDateTime {
				return ErrMongoOwnerConflict
			}
			if lease || claim {
				r.block("mongo_published_lease_or_claim_present")
			}
		default:
			return ErrMongoOwnerConflict
		}
		if err = r.verifyReverseMongoOwner(ctx, row, inner); err != nil {
			return err
		}
	}
	for _, expected := range r.expectedStandard {
		row, found := relevant[expected.EventID]
		if !found {
			r.block("mongo_live_standard_reference_absent")
			continue
		}
		msg, err := message.New(row.input())
		if err != nil || standard.ReferenceFromMessage(msg) != expected {
			return ErrMongoOwnerConflict
		}
	}
	replays, err := r.readRows(ctx, "qs_rm_replay_requests", bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "org_id", Value: int64(r.source.OrgID)}}, bson.D{{Key: "items.event_id", Value: r.source.EventID}}}}})
	if err != nil {
		return err
	}
	for _, raw := range replays {
		if err = r.checkMongoReplay(raw, relevant); err != nil {
			return err
		}
	}
	r.currentStandard = relevant
	r.local.Gaps = append(r.local.Gaps, "sql_retry_event_hold_and_dead_letter_not_mongo_stores", "inbox_and_global_unbound_coverage_require_actual_runtime_coordinator")
	return nil
}

func (row mongoOwnerStandardRow) input() message.Input {
	return message.Input{Producer: row.Producer, ID: row.MessageID, Destination: row.Destination, EventType: row.EventType, SchemaVersion: row.SchemaVersion, Scope: row.Scope, ContentType: row.ContentType, OccurredAt: row.OccurredAt, Payload: row.Payload}
}

func mongoPayloadID(raw []byte, key string) (string, error) {
	var fields map[string]json.RawMessage
	if strictJSON(raw) != nil || json.Unmarshal(raw, &fields) != nil {
		return "", ErrMongoOwnerConflict
	}
	value, ok := fields[key]
	if !ok {
		return "", nil
	}
	var text string
	if json.Unmarshal(value, &text) == nil {
		if text == "" {
			return "", nil
		}
		n, e := strconv.ParseUint(text, 10, 64)
		if e != nil || n == 0 || strconv.FormatUint(n, 10) != text {
			return "", ErrMongoOwnerConflict
		}
		return text, nil
	}
	var n json.Number
	if json.Unmarshal(value, &n) != nil {
		return "", ErrMongoOwnerConflict
	}
	v, e := strconv.ParseUint(n.String(), 10, 64)
	if e != nil || v == 0 || strconv.FormatUint(v, 10) != n.String() {
		return "", ErrMongoOwnerConflict
	}
	return n.String(), nil
}

func (r *MongoOwnerResolution) mongoEnvelopeCovered(inner *domainwire.Envelope) (bool, error) {
	if inner == nil {
		return false, ErrMongoOwnerConflict
	}
	keys := map[string]uint64{"answer_sheet_id": r.local.AnswerSheetID, "answersheet_id": r.local.AnswerSheetID, "generation_id": r.local.GenerationID, "assessment_id": r.local.AssessmentID, "outcome_id": r.local.OutcomeID}
	covered := inner.ID == r.source.EventID || (inner.AggregateType == r.source.AggregateType && inner.AggregateID == r.source.AggregateID)
	for key, id := range keys {
		if id == 0 {
			continue
		}
		text, err := mongoPayloadID(inner.Data, key)
		if err != nil {
			return false, err
		}
		if text == strconv.FormatUint(id, 10) {
			covered = true
		}
	}
	return covered, nil
}

func (r *MongoOwnerResolution) verifyReverseMongoOwner(ctx context.Context, row mongoOwnerStandardRow, inner *domainwire.Envelope) error {
	switch inner.EventType {
	case "answersheet.submitted":
		var p eventpayload.AnswerSheetSubmittedData
		if strictTyped(inner.Data, &p) != nil || p.OrgID != r.local.OrgID || p.TesteeID != r.local.TesteeID {
			return ErrMongoOwnerConflict
		}
		id, err := strconv.ParseUint(p.AnswerSheetID, 10, 64)
		if err != nil || id == 0 || inner.AggregateType != "AnswerSheet" || inner.AggregateID != p.AnswerSheetID {
			return ErrMongoOwnerConflict
		}
		var owner sheetmongo.AnswerSheetPO
		if _, err = r.readOne(ctx, "answersheets", id, &owner); err != nil {
			return err
		}
		actual, err := mongoSubmissionPayload(owner)
		if err != nil {
			return err
		}
		a, err := eventevidencebinding.AnswerSheet(actual)
		if err != nil {
			return ErrMongoOwnerConflict
		}
		b, err := eventevidencebinding.AnswerSheet(p)
		if err != nil || a != b {
			return ErrMongoOwnerConflict
		}
		if owner.DurableAcceptance == nil || owner.DurableAcceptance.SchemaVersion != 1 || owner.DurableAcceptance.EventID != row.MessageID {
			r.block("mongo_standard_submission_reverse_atomic_owner_absent")
		}
	case "interpretation.report.generated":
		if r.sqlFacts == nil {
			return ErrMongoOwnerResolution
		}
		var p eventoutcome.ReportGeneratedPayload
		if strictTyped(inner.Data, &p) != nil || p.OrgID <= 0 || uint64(p.OrgID) != r.local.OrgID || p.TesteeID != r.local.TesteeID {
			return ErrMongoOwnerConflict
		}
		gid, e := strconv.ParseUint(p.GenerationID, 10, 64)
		if e != nil || gid == 0 || inner.AggregateType != "ReportGeneration" || inner.AggregateID != p.GenerationID {
			return ErrMongoOwnerConflict
		}
		var g interpretmongo.ReportGenerationPO
		if _, e = r.readOne(ctx, "report_generations", gid, &g); e != nil {
			return e
		}
		if g.GeneratedEventID != row.MessageID || g.TransactionSchemaVersion != 1 {
			r.block("mongo_standard_generated_reverse_atomic_owner_absent")
		}
		var a interpretmongo.InterpretReportPO
		var run interpretmongo.InterpretationRunPO
		aid, e := strconv.ParseUint(p.ReportID, 10, 64)
		if e != nil {
			return ErrMongoOwnerConflict
		}
		rid, e := strconv.ParseUint(p.RunID, 10, 64)
		if e != nil {
			return ErrMongoOwnerConflict
		}
		if _, e = r.readOne(ctx, "interpret_report_artifacts", aid, &a); e != nil {
			return e
		}
		if _, e = r.readOne(ctx, "interpretation_runs", rid, &run); e != nil {
			return e
		}
		fact, e := r.sqlFacts.OutcomeRecord(g.OutcomeID)
		if e != nil {
			return ErrMongoOwnerConflict
		}
		actual, e := interpretmongo.HistoricalGeneratedPayload(g, a, run, fact)
		if e != nil {
			return ErrMongoOwnerConflict
		}
		x, e := eventevidencebinding.Generated(actual)
		if e != nil {
			return ErrMongoOwnerConflict
		}
		y, e := eventevidencebinding.Generated(p)
		if e != nil || x != y {
			return ErrMongoOwnerConflict
		}
	case "interpretation.retry.requested":
		var p eventoutcome.InterpretationRetryRequestedPayload
		if strictTyped(inner.Data, &p) != nil || p.OrgID <= 0 || uint64(p.OrgID) != r.local.OrgID || p.TesteeID != r.local.TesteeID {
			return ErrMongoOwnerConflict
		}
		id, e := strconv.ParseUint(p.RunID, 10, 64)
		if e != nil {
			return ErrMongoOwnerConflict
		}
		var run interpretmongo.InterpretationRunPO
		if _, e = r.readOne(ctx, "interpretation_runs", id, &run); e != nil {
			return e
		}
		if run.RetryEventID != row.MessageID || p.GenerationID != strconv.FormatUint(run.GenerationID, 10) || p.ExpectedAttempt != run.Attempt+1 {
			return ErrMongoOwnerConflict
		}
		// This authorization is not proof that the requested attempt ran.
		r.block("mongo_current_retry_execution_closure_required")
	case "interpretation.report.failed":
		var p eventoutcome.ReportFailedPayload
		if strictTyped(inner.Data, &p) != nil || p.OrgID <= 0 || uint64(p.OrgID) != r.local.OrgID || p.TesteeID != r.local.TesteeID {
			return ErrMongoOwnerConflict
		}
		id, e := strconv.ParseUint(p.RunID, 10, 64)
		if e != nil {
			return ErrMongoOwnerConflict
		}
		var run interpretmongo.InterpretationRunPO
		if _, e = r.readOne(ctx, "interpretation_runs", id, &run); e != nil {
			return e
		}
		if run.Status != "failed" || run.Attempt != int(p.Attempt) || p.GenerationID != strconv.FormatUint(run.GenerationID, 10) || run.Failure == nil || run.Failure.Kind != p.FailureKind || run.Failure.Code != p.FailureCode || run.FinishedAt == nil || !BusinessTimeEqual(*run.FinishedAt, p.FailedAt) {
			return ErrMongoOwnerConflict
		}
	default:
		return ErrSourceEventType
	}
	return nil
}

func (r *MongoOwnerResolution) checkMongoReplay(raw bson.Raw, relevant map[string]mongoOwnerStandardRow) error {
	fields, err := exactBSONFields(raw)
	if err != nil {
		return ErrMongoOwnerConflict
	}
	allowed := map[string]bool{"_id": true, "org_id": true, "request_id": true, "store_name": true, "reason": true, "input_hash": true, "items": true, "created_at": true}
	for key := range fields {
		if !allowed[key] {
			return ErrMongoOwnerConflict
		}
	}
	org, ok := mongoExactInteger(fields["org_id"])
	if !ok || org <= 0 || fields["input_hash"].Type != bson.TypeBinary || fields["items"].Type != bson.TypeArray {
		return ErrMongoOwnerConflict
	}
	for _, key := range []string{"_id", "request_id", "store_name", "reason"} {
		if fields[key].Type != bson.TypeString {
			return ErrMongoOwnerConflict
		}
	}
	items, e := fields["items"].Array().Values()
	if e != nil || len(items) == 0 || len(items) > 100 {
		return ErrMongoOwnerConflict
	}
	for _, item := range items {
		if item.Type != bson.TypeEmbeddedDocument {
			return ErrMongoOwnerConflict
		}
		f, e := exactBSONFields(item.Document())
		if e != nil || len(f) != 4 {
			return ErrMongoOwnerConflict
		}
		n, ok := mongoExactInteger(f["expected_failure_count"])
		if !ok || n <= 0 || f["event_id"].Type != bson.TypeString || f["authorized"].Type != bson.TypeBoolean || f["reason"].Type != bson.TypeString {
			return ErrMongoOwnerConflict
		}
	}
	var row struct {
		ID        string `bson:"_id"`
		OrgID     int64  `bson:"org_id"`
		RequestID string `bson:"request_id"`
		Store     string `bson:"store_name"`
		Reason    string `bson:"reason"`
		InputHash []byte `bson:"input_hash"`
		Items     []struct {
			EventID    string `bson:"event_id"`
			Count      uint64 `bson:"expected_failure_count"`
			Authorized bool   `bson:"authorized"`
			Reason     string `bson:"reason"`
		} `bson:"items"`
	}
	if bson.Unmarshal(raw, &row) != nil || row.OrgID <= 0 || uint64(row.OrgID) != r.local.OrgID || row.ID != strconv.FormatInt(row.OrgID, 10)+":"+row.RequestID || row.Store != "mongo-domain-events" {
		return ErrMongoOwnerConflict
	}
	request := standard.ReplayRequest{OrgID: row.OrgID, RequestID: row.RequestID, Store: row.Store, Reason: row.Reason}
	for _, item := range row.Items {
		request.Targets = append(request.Targets, standard.ReplayTarget{EventID: item.EventID, ExpectedFailureCount: item.Count})
	}
	digest, err := request.Fingerprint()
	if err != nil || !bytes.Equal(digest[:], row.InputHash) {
		return ErrMongoOwnerConflict
	}
	for _, item := range row.Items {
		outbox, found := relevant[item.EventID]
		if !found && item.EventID != r.source.EventID {
			continue
		}
		r.local.CurrentResponsibilityCount++
		if item.Authorized {
			if !found || outbox.State != "published" || outbox.ManualReplayRequestID != row.RequestID || outbox.ManualReplayVersion == 0 || outbox.Version <= outbox.ManualReplayVersion || outbox.Version-outbox.ManualReplayVersion < 2 || outbox.FailureCount < item.Count {
				r.block("mongo_replay_authorization_execution_unclosed")
			}
		} else if item.Reason == "" {
			return ErrMongoOwnerConflict
		}
	}
	return nil
}
