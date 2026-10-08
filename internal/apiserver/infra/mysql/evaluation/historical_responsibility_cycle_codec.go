package evaluation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	hostoutbox "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

type sqlResponsibilityLink struct {
	requestID, store, action, code string
	authorized                     bool
	before, after, expected        uint64
	ordinal                        uint64
}

func cycleDecode(store string, row historicalSQLRow) SQLResponsibilityObservation {
	v := SQLResponsibilityObservation{Store: store, ScopeClass: "coordination_required"}
	switch store {
	case "rm_outbox", "retry_event_hold", "event_delivery_dead_letter":
		return cycleDecodeMessage(store, row)
	case "qs_rm_evaluation_request_ref":
		v.EventID = valueOrEmpty(row["event_id"])
		v.AssessmentID = cyclePositive(row, "assessment_id")
		v.OrgID = cyclePositive(row, "org_id")
		v.State = "retained_business_reference"
		v.ScopeClass = "retirement_related"
		if v.EventID == "" || v.AssessmentID == 0 || v.OrgID == 0 {
			cycleReason(&v, "business_reference_identity_invalid")
		}
	case "qs_rm_gap_recovery_request":
		v.EventID = valueOrEmpty(row["event_id"])
		v.AssessmentID = cyclePositive(row, "assessment_id")
		v.OrgID = cyclePositive(row, "org_id")
		v.State = "retained_gap_decision"
		v.ScopeClass = "retirement_related"
		v.link.requestID = valueOrEmpty(row["request_id"])
		v.link.code = valueOrEmpty(row["result_code"])
		v.link.authorized = valueOrEmpty(row["authorized"]) == "1"
		if v.EventID == "" || v.AssessmentID == 0 || v.OrgID == 0 || v.link.requestID == "" || valueOrEmpty(row["authorized"]) != "0" && valueOrEmpty(row["authorized"]) != "1" {
			cycleReason(&v, "gap_decision_identity_invalid")
		}
		if v.link.code == "" {
			v.Unfinished = true
			cycleReason(&v, "gap_decision_unfinished")
		} else if v.link.authorized {
			var ok1, ok2, ok3 bool
			v.link.before, ok1 = cycleSafeUint(row, "outbox_version_before")
			v.link.after, ok2 = cycleSafeUint(row, "outbox_version_after")
			v.link.expected, ok3 = cycleSafeUint(row, "expected_version")
			if !ok1 || !ok2 || !ok3 || v.link.code != "authorized" || v.link.before != v.link.expected || v.link.after <= v.link.before {
				cycleReason(&v, "gap_decision_versions_invalid")
			}
		} else if !sqlHistoricalGapDenial(v.link.code) || row["outbox_version_before"] != nil || row["outbox_version_after"] != nil {
			cycleReason(&v, "gap_decision_result_unknown")
		}
	case "qs_rm_replay_requests":
		v.OrgID = cyclePositive(row, "org_id")
		v.link.requestID = valueOrEmpty(row["request_id"])
		v.link.store = valueOrEmpty(row["store_name"])
		v.State = "retained_authorization"
		if v.OrgID == 0 || v.link.requestID == "" || len(valueOrEmpty(row["input_hash"])) != 32 {
			cycleReason(&v, "replay_request_identity_invalid")
		}
		if v.link.store != "assessment-mysql-outbox" && v.link.store != "mongo-domain-events" {
			cycleReason(&v, "replay_store_unknown")
		}
	case "qs_rm_replay_items":
		v.OrgID = cyclePositive(row, "org_id")
		v.EventID = valueOrEmpty(row["event_id"])
		v.link.requestID = valueOrEmpty(row["request_id"])
		v.link.authorized = valueOrEmpty(row["authorized"]) == "1"
		v.State = "retained_authorization"
		var ok bool
		v.link.ordinal, ok = cycleSafeUint(row, "ordinal")
		_, failureOK := cycleSafeUint(row, "expected_failure_count")
		if v.OrgID == 0 || v.EventID == "" || v.link.requestID == "" || !ok || !failureOK || valueOrEmpty(row["authorized"]) != "0" && valueOrEmpty(row["authorized"]) != "1" {
			cycleReason(&v, "replay_item_identity_invalid")
		}
		v.link.code = valueOrEmpty(row["reason"])
		if v.link.authorized {
			if v.link.code != "" {
				cycleReason(&v, "replay_authorized_reason_invalid")
			}
		} else {
			switch v.link.code {
			case "not_found", "ambiguous_identity", "organization_mismatch", "not_manual_required", "attempt_conflict":
			default:
				cycleReason(&v, "replay_denial_reason_unknown")
			}
		}
	case "system_governance_action_runs":
		v.OrgID = cyclePositive(row, "org_id")
		v.State = valueOrEmpty(row["status"])
		v.link.action = valueOrEmpty(row["action_id"])
		v.link.requestID = valueOrEmpty(row["request_id"])
		if v.OrgID == 0 || v.link.action == "" || v.link.requestID == "" || sqlHistoricalUniqueJSON([]byte(valueOrEmpty(row["input_json"]))) != nil {
			cycleReason(&v, "governance_action_identity_invalid")
		}
		switch v.State {
		case "succeeded", "failed", "timeout":
			if row["finished_at"] == nil {
				cycleReason(&v, "governance_terminal_clock_missing")
			}
		case "running", "pending_reconciliation":
			v.Unfinished = true
		default:
			cycleReason(&v, "governance_state_unknown")
		}
		switch v.link.action {
		case "events.replay_pending", "events.replay_delivery", "evaluation.retry", "evaluation.force_retry", "interpretation.retry", "interpretation.force_retry", "interpretation.readmit_outcome", "interpretation.catalog_repair":
			v.ScopeClass = "retirement_related"
		case "cache.reload_policy", "cache.manual_warmup", "cache.repair_complete", "interpretation.report_template_publish", "interpretation.report_template_disable", "resilience.release_lock", "resilience.tune_rate_limit":
			v.ScopeClass = "scope_outside_retirement"
		default:
			v.ScopeClass = "coordination_required"
			v.OwnerUnproven = true
		}
	default:
		cycleReason(&v, "ledger_unknown")
	}
	return v
}

func cycleDecodeMessage(store string, row historicalSQLRow) SQLResponsibilityObservation {
	v := SQLResponsibilityObservation{Store: store, ScopeClass: "coordination_required", Unfinished: true}
	var inner *domainwire.Envelope
	var err error
	if store == "rm_outbox" {
		in := message.Input{Producer: valueOrEmpty(row["producer"]), ID: valueOrEmpty(row["message_id"]), Destination: valueOrEmpty(row["destination"]), EventType: valueOrEmpty(row["event_type"]), SchemaVersion: valueOrEmpty(row["schema_version"]), Scope: valueOrEmpty(row["scope"]), ContentType: valueOrEmpty(row["content_type"]), OccurredAt: valueOrEmpty(row["occurred_at"]), Payload: []byte(valueOrEmpty(row["payload"]))}
		v.EventID, v.EventType = in.ID, in.EventType
		msg, e := message.New(in)
		if e != nil {
			cycleReason(&v, "sdk_identity_invalid")
			return v
		}
		ref := hostoutbox.ReferenceFromMessage(msg)
		v.SDKFingerprintSHA256 = hex.EncodeToString([]byte(valueOrEmpty(row["fingerprint"])))
		inner, err = hostoutbox.VerifyReference(in, v.SDKFingerprintSHA256, ref)
		v.State = valueOrEmpty(row["state"])
		v.LeasePresent = row["lease_until"] != nil || row["claim_token"] != nil
		switch v.State {
		case "published":
			v.Unfinished = row["transport_confirmed_at"] == nil || v.LeasePresent
		case "pending", "retry_wait", "publishing", "quarantined":
		default:
			cycleReason(&v, "sdk_state_unknown")
		}
		if _, ok := cycleSafeUint(row, "version"); !ok {
			cycleReason(&v, "sdk_version_invalid")
		}
	} else {
		raw := []byte(valueOrEmpty(row["payload_json"]))
		outer, recognized, e := legacy.Decode(raw)
		if e != nil {
			cycleReason(&v, "domain_wire_invalid")
			return v
		}
		if recognized {
			encoded, e := legacy.Encode(outer, legacy.Revision2)
			if e != nil || !bytes.Equal(encoded, raw) {
				cycleReason(&v, "transport_revision_invalid")
				return v
			}
			inner, err = domainwire.DecodeEnvelope(outer.Payload)
			if err == nil && (outer.UUID != inner.ID || outer.Metadata["event_type"] != inner.EventType || outer.Metadata["aggregate_type"] != inner.AggregateType || outer.Metadata["aggregate_id"] != inner.AggregateID || outer.Metadata["source"] == "" || outer.Metadata["occurred_at"] != inner.OccurredAt.Format(domainwire.OccurredAtLayout)) {
				cycleReason(&v, "transport_domain_identity_conflict")
			}
			raw = outer.Payload
		} else {
			inner, err = domainwire.DecodeEnvelope(raw)
		}
		if sqlHistoricalUniqueJSON(raw) != nil {
			cycleReason(&v, "domain_json_ambiguous")
		}
		if store == "retry_event_hold" {
			v.State = valueOrEmpty(row["status"])
			v.LeasePresent = row["claim_expires_at"] != nil || row["claim_token"] != nil
			v.Unfinished = v.State != "replayed" || v.LeasePresent || row["replayed_at"] == nil
			switch v.State {
			case "blocked", "failed", "replaying", "replayed":
			default:
				cycleReason(&v, "held_state_unknown")
			}
		} else {
			v.State = valueOrEmpty(row["retry_disposition"])
			v.Unfinished = v.State != "resolved_verified"
			switch v.State {
			case "manual_required", "automatic", "terminal", "resolved_verified":
			default:
				cycleReason(&v, "deadletter_state_unknown")
			}
		}
	}
	if err != nil || inner == nil {
		cycleReason(&v, "sdk_fingerprint_or_wire_conflict")
		return v
	}
	var strict domainwire.Envelope
	if sqlHistoricalStrictJSON(mustCycleInner(row, store), &strict) != nil {
		cycleReason(&v, "domain_wire_ambiguous")
	}
	v.EventID, v.EventType = inner.ID, inner.EventType
	if store != "rm_outbox" && valueOrEmpty(row["event_id"]) != inner.ID {
		cycleReason(&v, "stored_event_identity_conflict")
	}
	cyclePayloadOwner(&v, inner)
	if store != "rm_outbox" && cyclePositive(row, "org_id") != v.OrgID {
		cycleReason(&v, "stored_organization_conflict")
	}
	h := sha256.Sum256(inner.Data)
	v.payloadSHA256 = hex.EncodeToString(h[:])
	return v
}

func mustCycleInner(row historicalSQLRow, store string) []byte {
	column := "payload_json"
	if store == "rm_outbox" {
		column = "payload"
	}
	raw := []byte(valueOrEmpty(row[column]))
	outer, recognized, err := legacy.Decode(raw)
	if err == nil && recognized {
		return outer.Payload
	}
	return raw
}

func cyclePayloadOwner(v *SQLResponsibilityObservation, inner *domainwire.Envelope) {
	v.OwnerKind, v.OwnerID = inner.AggregateType, inner.AggregateID
	if inner.ID == "" || len(inner.ID) > 128 || inner.OccurredAt.IsZero() {
		cycleReason(v, "domain_identity_invalid")
	}
	switch inner.EventType {
	case "evaluation.requested", "evaluation.retry.requested", "evaluation.failed", "evaluation.outcome.committed":
		owner, ok := sqlHistoricalEnvelopeOwner(inner)
		v.ScopeClass = "retirement_related"
		if !ok {
			cycleReason(v, "evaluation_owner_payload_invalid")
			return
		}
		v.OrgID, v.AssessmentID, v.TesteeID = uint64(owner.OrgID), uint64(owner.AssessmentID), owner.TesteeID
		if inner.EventType == "evaluation.outcome.committed" {
			var p eventpayload.EvaluationOutcomeCommittedData
			if sqlHistoricalStrictJSON(inner.Data, &p) != nil {
				cycleReason(v, "outcome_payload_invalid")
				return
			}
			v.outcomeID, v.runID = cyclePayloadID(p.OutcomeID), p.EvaluationRunID
			if v.outcomeID == 0 || v.runID == "" || p.CommittedAt.IsZero() {
				cycleReason(v, "original_outcome_identity_absent")
			}
		}
	case "answersheet.submitted":
		var p eventpayload.AnswerSheetSubmittedData
		if sqlHistoricalStrictJSON(inner.Data, &p) != nil || p.OrgID == 0 || p.TesteeID == 0 || p.AnswerSheetID == "" || p.SubmittedAt.IsZero() || inner.AggregateType != "AnswerSheet" || inner.AggregateID != p.AnswerSheetID {
			cycleReason(v, "answersheet_owner_payload_invalid")
			return
		}
		v.OrgID, v.TesteeID = p.OrgID, p.TesteeID
		v.ScopeClass = "retirement_related"
		v.OwnerUnproven = true
	case "interpretation.report.generated":
		var p eventoutcome.ReportGeneratedPayload
		if sqlHistoricalStrictJSON(inner.Data, &p) != nil || p.OrgID <= 0 || p.TesteeID == 0 || p.ReportID == "" || p.GenerationID == "" || p.RunID == "" || p.OutcomeID == "" || p.Attempt == 0 || p.GeneratedAt.IsZero() || inner.AggregateType != "ReportGeneration" || inner.AggregateID != p.GenerationID {
			cycleReason(v, "report_owner_payload_invalid")
			return
		}
		v.OrgID, v.TesteeID = uint64(p.OrgID), p.TesteeID
		v.AssessmentID = cyclePayloadID(p.AssessmentID)
		if v.AssessmentID == 0 {
			cycleReason(v, "report_assessment_identity_invalid")
		}
		v.ScopeClass = "retirement_related"
		v.OwnerUnproven = true
	case "interpretation.report.failed":
		var p eventoutcome.ReportFailedPayload
		if sqlHistoricalStrictJSON(inner.Data, &p) != nil || p.OrgID <= 0 || p.TesteeID == 0 || p.GenerationID == "" || p.RunID == "" || p.Attempt == 0 || p.FailedAt.IsZero() || inner.AggregateType != "ReportGeneration" || inner.AggregateID != p.GenerationID {
			cycleReason(v, "report_failed_owner_payload_invalid")
			return
		}
		v.OrgID, v.TesteeID = uint64(p.OrgID), p.TesteeID
		v.ScopeClass = "scope_outside_retirement"
		v.OwnerUnproven = true
	case "interpretation.retry.requested":
		var p eventoutcome.InterpretationRetryRequestedPayload
		if sqlHistoricalStrictJSON(inner.Data, &p) != nil || p.OrgID <= 0 || p.TesteeID == 0 || p.GenerationID == "" || p.RunID == "" || p.ExpectedAttempt <= 0 || p.RequestedAt.IsZero() || inner.AggregateType != "ReportGeneration" || inner.AggregateID != p.GenerationID {
			cycleReason(v, "report_retry_owner_payload_invalid")
			return
		}
		v.OrgID, v.TesteeID = uint64(p.OrgID), p.TesteeID
		v.ScopeClass = "scope_outside_retirement"
		v.OwnerUnproven = true
	case "task.opened.reminder.requested":
		var p eventpayload.TaskOpenedReminderRequestedData
		if sqlHistoricalStrictJSON(inner.Data, &p) != nil || p.OrgID <= 0 || p.TaskID == "" || p.PlanID == "" || p.TesteeID == "" || p.ScheduleRevision == 0 || p.OpenAt.IsZero() || inner.AggregateType != "AssessmentTask" || inner.AggregateID != p.TaskID {
			cycleReason(v, "task_owner_payload_invalid")
			return
		}
		v.OrgID = uint64(p.OrgID)
		v.ScopeClass = "scope_outside_retirement"
		v.OwnerUnproven = true
	default:
		cycleReason(v, "current_event_type_unknown")
	}
}

func cyclePayloadID(value string) uint64 {
	n, e := strconv.ParseUint(value, 10, 64)
	if e != nil || n == 0 || strconv.FormatUint(n, 10) != value {
		return 0
	}
	return n
}
