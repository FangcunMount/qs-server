package evaluation

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	hostoutbox "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"gorm.io/gorm"
)

// Selection hints are deliberately not limited by organization or outer type.
// Wrongly attributed candidates must be inspected rather than silently missed.
func sqlHistoricalPayloadCandidates(column string) string {
	// SDK payloads are binary columns: MySQL JSON_VALID(BLOB) returns false
	// even for valid UTF-8 JSON. Convert only selection hints; authentication
	// still consumes the original exact bytes, never this text conversion.
	text := "CONVERT(" + column + " USING utf8mb4)"
	outer := "CASE WHEN JSON_VALID(" + text + ") THEN " + text + " ELSE NULL END"
	decoded := "CONVERT(FROM_BASE64(JSON_UNQUOTE(JSON_EXTRACT(" + outer + ",'$.payload'))) USING utf8mb4)"
	inner := "CASE WHEN JSON_VALID(" + decoded + ") THEN " + decoded + " ELSE NULL END"
	return "CAST(JSON_UNQUOTE(JSON_EXTRACT(" + outer + ",'$.metadata.aggregate_id')) AS BINARY)=? OR CAST(JSON_UNQUOTE(JSON_EXTRACT(" + outer + ",'$.aggregateID')) AS BINARY)=? OR CAST(JSON_UNQUOTE(JSON_EXTRACT(" + outer + ",'$.data.assessment_id')) AS BINARY)=? OR CAST(JSON_UNQUOTE(JSON_EXTRACT(" + inner + ",'$.aggregateID')) AS BINARY)=? OR CAST(JSON_UNQUOTE(JSON_EXTRACT(" + inner + ",'$.data.assessment_id')) AS BINARY)=?"
}

func (f *SQLHistoricalOwnerFacts) readResponsibilities(tx *gorm.DB) error {
	owner := strconv.FormatUint(f.assessmentID, 10)
	var err error
	ids := []string{f.eventID}
	f.rows["qs_rm_evaluation_request_ref"], err = sqlHistoricalQueryRows(tx, "SELECT * FROM qs_rm_evaluation_request_ref WHERE assessment_id=? OR CAST(event_id AS BINARY)=? ORDER BY event_id", f.assessmentID, f.eventID)
	if err != nil {
		return err
	}
	for _, row := range f.rows["qs_rm_evaluation_request_ref"] {
		org, e := sqlHistoricalUint(row, "org_id")
		assessment, a := sqlHistoricalUint(row, "assessment_id")
		id := valueOrEmpty(row["event_id"])
		if id != "" && len(id) <= 128 {
			ids = append(ids, id)
		}
		f.snapshot.Responsibilities = append(f.snapshot.Responsibilities, SQLHistoricalResponsibility{Store: "qs_rm_evaluation_request_ref", ID: id, EventID: id, OrgID: org, AssessmentID: assessment, State: "retained_business_reference", Invalid: e != nil || a != nil || org != f.snapshot.Owner.OrgID || assessment != f.assessmentID || id == "" || len(id) > 128})
	}
	args := []any{ids, owner, owner, owner, owner, owner}
	for _, table := range []string{"rm_outbox", "retry_event_hold", "event_delivery_dead_letter"} {
		column, idColumn := "payload_json", "event_id"
		if table == "rm_outbox" {
			column, idColumn = "payload", "message_id"
		}
		f.rows[table], err = sqlHistoricalRows(tx, table, "CAST("+idColumn+" AS BINARY) IN ? OR ("+sqlHistoricalPayloadCandidates(column)+")", args...)
		if err != nil {
			return err
		}
		for _, row := range f.rows[table] {
			f.snapshot.Responsibilities = append(f.snapshot.Responsibilities, sqlHistoricalResponsibility(table, row, f.assessmentID, f.snapshot.Owner.OrgID))
		}
	}
	for _, row := range f.snapshot.Responsibilities {
		if row.EventID != "" {
			ids = append(ids, row.EventID)
		}
	}
	f.rows["qs_rm_gap_recovery_request"], err = sqlHistoricalQueryRows(tx, "SELECT * FROM qs_rm_gap_recovery_request WHERE assessment_id=? OR event_id IN ? ORDER BY org_id,request_id", f.assessmentID, ids)
	if err != nil {
		return err
	}
	for _, row := range f.rows["qs_rm_gap_recovery_request"] {
		org, e := sqlHistoricalUint(row, "org_id")
		assessment, a := sqlHistoricalUint(row, "assessment_id")
		fact := SQLHistoricalResponsibility{Store: "qs_rm_gap_recovery_request", ID: valueOrEmpty(row["request_id"]), EventID: valueOrEmpty(row["event_id"]), OrgID: org, AssessmentID: assessment, State: "retained_gap_decision", Invalid: e != nil || a != nil || org != f.snapshot.Owner.OrgID || assessment != f.assessmentID || valueOrEmpty(row["event_id"]) == "" || valueOrEmpty(row["request_id"]) == ""}
		code, authorized := valueOrEmpty(row["result_code"]), valueOrEmpty(row["authorized"])
		if code == "" || (authorized != "0" && authorized != "1") {
			fact.Invalid, fact.Unfinished = true, true
		} else if authorized == "1" {
			before, b := sqlHistoricalUint(row, "outbox_version_before")
			after, c := sqlHistoricalUint(row, "outbox_version_after")
			expected, d := sqlHistoricalUint(row, "expected_version")
			fact.Invalid = fact.Invalid || code != "authorized" || b != nil || c != nil || d != nil || expected != before || after <= before
			matched, settled := 0, false
			for _, current := range f.snapshot.Responsibilities {
				if current.Store == "rm_outbox" && current.EventID == fact.EventID {
					matched++
					settled = current.EventType == "evaluation.requested" && !current.Invalid && !current.Unfinished && !current.LeasePresent
				}
			}
			fact.Unfinished = matched != 1 || !settled
		} else if !sqlHistoricalGapDenial(code) || row["outbox_version_before"] != nil || row["outbox_version_after"] != nil {
			fact.Invalid = true
		}
		f.snapshot.Responsibilities = append(f.snapshot.Responsibilities, fact)
	}
	f.rows["qs_rm_replay_items"], err = sqlHistoricalQueryRows(tx, "SELECT * FROM qs_rm_replay_items WHERE event_id IN ? ORDER BY org_id,request_id,ordinal", ids)
	if err != nil {
		return err
	}
	requests := make([]string, 0, len(f.rows["qs_rm_replay_items"]))
	for _, row := range f.rows["qs_rm_replay_items"] {
		org, e := sqlHistoricalUint(row, "org_id")
		if e != nil {
			return e
		}
		requests = append(requests, valueOrEmpty(row["request_id"]))
		fact := SQLHistoricalResponsibility{Store: "qs_rm_replay_items", ID: valueOrEmpty(row["request_id"]) + ":" + valueOrEmpty(row["ordinal"]), EventID: valueOrEmpty(row["event_id"]), OrgID: org, AssessmentID: f.assessmentID, State: "retained_authorization", Invalid: org != f.snapshot.Owner.OrgID}
		// Retained authorization alone cannot prove a delivery accepted or
		// still pending. Current messages and the action audit are independent.
		f.snapshot.Responsibilities = append(f.snapshot.Responsibilities, fact)
	}
	if len(requests) > 0 {
		f.rows["qs_rm_replay_requests"], err = sqlHistoricalQueryRows(tx, "SELECT * FROM qs_rm_replay_requests WHERE request_id IN ? ORDER BY org_id,request_id", requests)
		if err != nil {
			return err
		}
		for _, row := range f.rows["qs_rm_replay_requests"] {
			org, e := sqlHistoricalUint(row, "org_id")
			if e != nil {
				return e
			}
			f.snapshot.Responsibilities = append(f.snapshot.Responsibilities, SQLHistoricalResponsibility{Store: "qs_rm_replay_requests", ID: valueOrEmpty(row["request_id"]), OrgID: org, AssessmentID: f.assessmentID, State: "retained_authorization", Invalid: org != f.snapshot.Owner.OrgID || valueOrEmpty(row["store_name"]) != "assessment-mysql-outbox"})
		}
	}
	// All unsettled replay actions in this organization remain a conservative
	// blocker: this adapter cannot choose a target from an unresolved audit.
	f.rows["system_governance_action_runs"], err = sqlHistoricalRows(tx, "system_governance_action_runs", "org_id=? AND action_id IN ('events.replay_pending','events.replay_delivery','evaluation.retry','evaluation.force_retry','interpretation.retry','interpretation.force_retry') AND status NOT IN ('succeeded','failed','timeout')", f.snapshot.Owner.OrgID)
	if err != nil {
		return err
	}
	for _, row := range f.rows["system_governance_action_runs"] {
		f.snapshot.Responsibilities = append(f.snapshot.Responsibilities, SQLHistoricalResponsibility{Store: "system_governance_action_runs", ID: valueOrEmpty(row["id"]), OrgID: f.snapshot.Owner.OrgID, State: valueOrEmpty(row["status"]), Unfinished: true})
	}
	return nil
}

func sqlHistoricalGapDenial(code string) bool {
	switch code {
	case "assessment_missing", "reference_missing_or_ambiguous", "ever_claimed", "outbox_missing_or_ambiguous", "version_conflict", "delivery_pending", "within_grace", "identity_mismatch", "fingerprint_missing", "assessment_not_submitted", "model_missing", "reference_mismatch", "outbox_identity_mismatch", "outbox_fingerprint_mismatch", "wire_identity_mismatch", "wire_revision_mismatch", "domain_identity_mismatch", "occurred_at_mismatch", "frozen_payload_mismatch":
		return true
	default:
		return false
	}
}

type sqlHistoricalEventOwner struct {
	OrgID        int64  `json:"org_id"`
	AssessmentID int64  `json:"assessment_id"`
	TesteeID     uint64 `json:"testee_id"`
}

func sqlHistoricalEnvelopeOwner(inner *domainwire.Envelope) (sqlHistoricalEventOwner, bool) {
	var owner sqlHistoricalEventOwner
	if inner == nil {
		return owner, false
	}
	switch inner.EventType {
	case "evaluation.requested", "evaluation.retry.requested":
		var payload eventpayload.EvaluationRequestedData
		if sqlHistoricalStrictJSON(inner.Data, &payload) != nil {
			return owner, false
		}
		owner = sqlHistoricalEventOwner{payload.OrgID, payload.AssessmentID, payload.TesteeID}
	case "evaluation.failed":
		var payload eventpayload.EvaluationFailedData
		if sqlHistoricalStrictJSON(inner.Data, &payload) != nil {
			return owner, false
		}
		owner = sqlHistoricalEventOwner{payload.OrgID, payload.AssessmentID, payload.TesteeID}
	case "evaluation.outcome.committed":
		var payload eventpayload.EvaluationOutcomeCommittedData
		if sqlHistoricalStrictJSON(inner.Data, &payload) != nil {
			return owner, false
		}
		owner = sqlHistoricalEventOwner{payload.OrgID, payload.AssessmentID, payload.TesteeID}
	default:
		return owner, false
	}
	if owner.OrgID <= 0 || owner.AssessmentID <= 0 || owner.TesteeID == 0 || inner.AggregateType != "Evaluation" || inner.AggregateID != strconv.FormatInt(owner.AssessmentID, 10) {
		return owner, false
	}
	return owner, true
}

func sqlHistoricalResponsibility(table string, row historicalSQLRow, assessment, org uint64) SQLHistoricalResponsibility {
	fact := SQLHistoricalResponsibility{Store: table, ID: valueOrEmpty(row["id"]), Unfinished: true}
	var inner *domainwire.Envelope
	var err error
	if table == "rm_outbox" {
		in := message.Input{Producer: valueOrEmpty(row["producer"]), ID: valueOrEmpty(row["message_id"]), Destination: valueOrEmpty(row["destination"]), EventType: valueOrEmpty(row["event_type"]), SchemaVersion: valueOrEmpty(row["schema_version"]), Scope: valueOrEmpty(row["scope"]), ContentType: valueOrEmpty(row["content_type"]), OccurredAt: valueOrEmpty(row["occurred_at"]), Payload: []byte(valueOrEmpty(row["payload"]))}
		msg, e := message.New(in)
		if e != nil {
			fact.Invalid = true
			return fact
		}
		outer, recognized, e := legacy.Decode(in.Payload)
		if e != nil || !recognized || sqlHistoricalUniqueJSON(outer.Payload) != nil {
			fact.Invalid = true
			return fact
		}
		inner, err = hostoutbox.VerifyReference(in, hex.EncodeToString([]byte(valueOrEmpty(row["fingerprint"]))), hostoutbox.ReferenceFromMessage(msg))
		fact.State = valueOrEmpty(row["state"])
		fact.LeasePresent = row["lease_until"] != nil
		switch fact.State {
		case "published":
			fact.Unfinished = row["transport_confirmed_at"] == nil || fact.LeasePresent
		case "pending", "retry_wait", "publishing", "quarantined":
		default:
			fact.Invalid = true
		}
	} else {
		raw := []byte(valueOrEmpty(row["payload_json"]))
		outer, recognized, e := legacy.Decode(raw)
		if e != nil {
			fact.Invalid = true
			return fact
		}
		if recognized {
			encoded, e := legacy.Encode(outer, legacy.Revision2)
			if e != nil || !bytes.Equal(encoded, raw) {
				fact.Invalid = true
				return fact
			}
			if sqlHistoricalUniqueJSON(outer.Payload) != nil {
				fact.Invalid = true
				return fact
			}
			inner, err = domainwire.DecodeEnvelope(outer.Payload)
			if err == nil && (outer.UUID != inner.ID || outer.Metadata["event_type"] != inner.EventType || outer.Metadata["aggregate_type"] != inner.AggregateType || outer.Metadata["aggregate_id"] != inner.AggregateID) {
				fact.Invalid = true
			}
		} else {
			if sqlHistoricalUniqueJSON(raw) != nil {
				fact.Invalid = true
				return fact
			}
			inner, err = domainwire.DecodeEnvelope(raw)
		}
		if table == "retry_event_hold" {
			fact.State = valueOrEmpty(row["status"])
			fact.LeasePresent = row["claim_expires_at"] != nil
			fact.Unfinished = fact.State != "replayed" || fact.LeasePresent || row["replayed_at"] == nil
		} else {
			fact.State = valueOrEmpty(row["retry_disposition"])
			fact.Unfinished = fact.State != "resolved_verified"
		}
	}
	if err != nil || inner == nil {
		fact.Invalid = true
		return fact
	}
	owner, valid := sqlHistoricalEnvelopeOwner(inner)
	fact.EventID, fact.EventType = inner.ID, inner.EventType
	if valid {
		fact.OrgID, fact.AssessmentID, fact.TesteeID = uint64(owner.OrgID), uint64(owner.AssessmentID), owner.TesteeID
	}
	if !valid || fact.OrgID != org || fact.AssessmentID != assessment {
		fact.Invalid = true
	}
	if table != "rm_outbox" && (valueOrEmpty(row["event_id"]) != inner.ID || valueOrEmpty(row["org_id"]) != fmt.Sprint(fact.OrgID)) {
		fact.Invalid = true
	}
	return fact
}

// Decode exactly one document; shared by new local resolver tests/adapters.
func sqlHistoricalStrictJSON(raw []byte, value any) error {
	if err := sqlHistoricalUniqueJSON(raw); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return ErrSQLHistoricalFactsInvalid
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return ErrSQLHistoricalFactsInvalid
	}
	return nil
}

func sqlHistoricalUniqueJSON(raw []byte) error {
	if len(raw) > 1<<20 {
		return ErrSQLHistoricalFactsBounds
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	tokens := 0
	var read func(int) error
	read = func(depth int) error {
		if depth >= 64 || tokens > 8192 {
			return ErrSQLHistoricalFactsBounds
		}
		tokens++
		token, err := d.Token()
		if err != nil {
			return ErrSQLHistoricalFactsInvalid
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				keys := map[string]bool{}
				for d.More() {
					key, err := d.Token()
					if err != nil {
						return ErrSQLHistoricalFactsInvalid
					}
					name, ok := key.(string)
					if !ok || keys[name] {
						return ErrSQLHistoricalFactsInvalid
					}
					keys[name] = true
					if err := read(depth + 1); err != nil {
						return err
					}
				}
				close, err := d.Token()
				if err != nil || close != json.Delim('}') {
					return ErrSQLHistoricalFactsInvalid
				}
			case '[':
				for d.More() {
					if err := read(depth + 1); err != nil {
						return err
					}
				}
				close, err := d.Token()
				if err != nil || close != json.Delim(']') {
					return ErrSQLHistoricalFactsInvalid
				}
			default:
				return ErrSQLHistoricalFactsInvalid
			}
		}
		return nil
	}
	if err := read(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrSQLHistoricalFactsInvalid
	}
	return nil
}
