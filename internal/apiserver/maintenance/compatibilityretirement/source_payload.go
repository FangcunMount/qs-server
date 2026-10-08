package retirement

import (
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
)

// This shape is the actual eventcodec.EncodeDomainEvent output at dce4598a4^,
// preserved by wire/domain.EncodeEvent. Transport envelopes are unsupported.
type originalDomainEnvelope struct {
	ID            string          `json:"id"`
	EventType     string          `json:"eventType"`
	OccurredAt    time.Time       `json:"occurredAt"`
	AggregateType string          `json:"aggregateType"`
	AggregateID   string          `json:"aggregateID"`
	Data          json.RawMessage `json:"data"`
}

type outerEventFacts struct {
	engine, id, eventType, aggregateType, aggregateID string
	org                                               *int64
}

func decodeDomain(raw []byte, outer outerEventFacts) (*DecodedSourceEvent, error) {
	var envelope originalDomainEnvelope
	if err := strictTyped(raw, &envelope); err != nil {
		return nil, err
	}
	if !identifier(envelope.ID, 64) || !identifier(envelope.AggregateID, 64) || envelope.ID != outer.id || envelope.EventType != outer.eventType ||
		envelope.AggregateType != outer.aggregateType || envelope.AggregateID != outer.aggregateID {
		return nil, ErrSourceIdentity
	}
	v := &DecodedSourceEvent{SupportedSchema: "legacy-domain-json-v1", ContentDigest: evidence.Digest{Kind: ContentDigestKind, SHA256: sourceSHA(raw)},
		EventID: envelope.ID, EventType: envelope.EventType, AggregateType: envelope.AggregateType, AggregateID: envelope.AggregateID,
		OccurredAt: envelope.OccurredAt, BusinessIDs: map[string]string{}}
	if outer.org != nil {
		org := *outer.org
		v.OuterOrgID = &org
	}
	expectedAggregate := ""
	switch envelope.EventType {
	case "evaluation.requested", "evaluation.retry.requested":
		if outer.engine != "mysql" {
			return nil, ErrSourceEventType
		}
		p := new(eventpayload.EvaluationRequestedData)
		if err := strictTyped(envelope.Data, p); err != nil {
			return nil, err
		}
		if p.OrgID <= 0 || p.AssessmentID <= 0 || p.TesteeID == 0 || !identifier(p.AnswerSheetID, 128) || !identifier(p.QuestionnaireCode, 128) || !identifier(p.QuestionnaireVer, 128) ||
			p.ExpectedAttempt < 0 || uint64(p.ExpectedAttempt) > math.MaxUint32 {
			return nil, ErrSourceSchema
		}
		v.Requested, v.OrgID, v.BusinessAt = p, uint64(p.OrgID), p.RequestedAt
		v.BusinessIDs["assessment_id"] = strconv.FormatInt(p.AssessmentID, 10)
		v.BusinessIDs["testee_id"] = strconv.FormatUint(p.TesteeID, 10)
		v.BusinessIDs["answersheet_id"] = p.AnswerSheetID
		// expected_attempt is authorization for a future attempt, not proof of
		// which original execution happened. Preserve it only on Requested.
		v.OriginalRun.Missing = []string{"original_run_id", "original_attempt"}
		v.ResolverGaps = append(v.ResolverGaps, "trusted_original_run_resolution_required")
		if !p.HasModelIdentity() {
			v.ResolverGaps = append(v.ResolverGaps, "original_model_identity_absent")
		}
		expectedAggregate = strconv.FormatInt(p.AssessmentID, 10)
	case "evaluation.failed":
		if outer.engine != "mysql" {
			return nil, ErrSourceEventType
		}
		p := new(eventpayload.EvaluationFailedData)
		if err := strictTyped(envelope.Data, p); err != nil {
			return nil, err
		}
		if p.OrgID <= 0 || p.AssessmentID <= 0 || p.TesteeID == 0 {
			return nil, ErrSourceSchema
		}
		v.Failed, v.OrgID, v.BusinessAt = p, uint64(p.OrgID), p.FailedAt
		v.BusinessIDs["assessment_id"] = strconv.FormatInt(p.AssessmentID, 10)
		v.BusinessIDs["testee_id"] = strconv.FormatUint(p.TesteeID, 10)
		v.OriginalRun.Missing = []string{"original_run_id", "original_attempt"}
		v.ResolverGaps = append(v.ResolverGaps, "trusted_original_run_resolution_required")
		expectedAggregate = strconv.FormatInt(p.AssessmentID, 10)
	case "evaluation.outcome.committed":
		if outer.engine != "mysql" {
			return nil, ErrSourceEventType
		}
		p := new(eventpayload.EvaluationOutcomeCommittedData)
		if err := strictTyped(envelope.Data, p); err != nil {
			return nil, err
		}
		if p.OrgID <= 0 || p.AssessmentID <= 0 || p.TesteeID == 0 || !identifier(p.OutcomeID, 128) || !identifier(p.EvaluationRunID, 128) {
			return nil, ErrSourceSchema
		}
		v.OutcomeCommitted, v.OrgID, v.BusinessAt, v.OriginalRun.RunID = p, uint64(p.OrgID), p.CommittedAt, p.EvaluationRunID
		v.BusinessIDs["assessment_id"] = strconv.FormatInt(p.AssessmentID, 10)
		v.BusinessIDs["testee_id"] = strconv.FormatUint(p.TesteeID, 10)
		v.BusinessIDs["outcome_id"] = p.OutcomeID
		v.OriginalRun.Missing = []string{"original_attempt"}
		expectedAggregate = strconv.FormatInt(p.AssessmentID, 10)
	case "answersheet.submitted":
		if outer.engine != "mongodb" {
			return nil, ErrSourceEventType
		}
		p := new(eventpayload.AnswerSheetSubmittedData)
		if err := strictTyped(envelope.Data, p); err != nil {
			return nil, err
		}
		if p.OrgID == 0 || p.OrgID > math.MaxInt64 || p.TesteeID == 0 || !identifier(p.AnswerSheetID, 128) || !identifier(p.QuestionnaireCode, 128) || !identifier(p.QuestionnaireVersion, 128) {
			return nil, ErrSourceSchema
		}
		v.Submitted, v.OrgID, v.BusinessAt = p, p.OrgID, p.SubmittedAt
		v.BusinessIDs["answersheet_id"] = p.AnswerSheetID
		v.BusinessIDs["testee_id"] = strconv.FormatUint(p.TesteeID, 10)
		v.BusinessIDs["filler_id"] = strconv.FormatUint(p.FillerID, 10)
		if p.Admission == nil {
			v.ResolverGaps = append(v.ResolverGaps, "frozen_admission_absent")
		} else if p.Admission.Purpose != eventpayload.AdmissionPurposeAssessment && p.Admission.Purpose != eventpayload.AdmissionPurposeIndependentQuestionnaire {
			return nil, ErrSourceSchema
		}
		if p.Attribution != nil && p.Attribution.TaskID != "" && p.TaskID != p.Attribution.TaskID {
			return nil, ErrSourceIdentity
		}
		expectedAggregate = p.AnswerSheetID
	case "interpretation.report.generated":
		if outer.engine != "mongodb" {
			return nil, ErrSourceEventType
		}
		p := new(eventoutcome.ReportGeneratedPayload)
		if err := strictTyped(envelope.Data, p); err != nil {
			return nil, err
		}
		if p.OrgID <= 0 || p.TesteeID == 0 || p.Attempt == 0 || uint64(p.Attempt) > math.MaxUint32 || !identifier(p.GenerationID, 128) || !identifier(p.RunID, 128) ||
			!identifier(p.ReportID, 128) || !identifier(p.AssessmentID, 128) || !identifier(p.OutcomeID, 128) || !identifier(p.Model.Kind, 128) || !identifier(p.Model.Code, 128) ||
			!identifier(p.ReportType, 128) || !identifier(p.TemplateVersion, 128) || !identifier(p.BuilderIdentity, 128) || !identifier(p.ContentSchemaVersion, 128) {
			return nil, ErrSourceSchema
		}
		v.Generated, v.OrgID, v.BusinessAt, v.OriginalRun.RunID = p, uint64(p.OrgID), p.GeneratedAt, p.RunID
		attempt := uint32(p.Attempt)
		v.OriginalRun.Attempt = &attempt
		v.BusinessIDs["generation_id"] = p.GenerationID
		v.BusinessIDs["report_id"] = p.ReportID
		v.BusinessIDs["assessment_id"] = p.AssessmentID
		v.BusinessIDs["outcome_id"] = p.OutcomeID
		v.BusinessIDs["testee_id"] = strconv.FormatUint(p.TesteeID, 10)
		expectedAggregate = p.GenerationID
	default:
		return nil, ErrSourceEventType
	}
	expectedType := "Evaluation"
	if v.Submitted != nil {
		expectedType = "AnswerSheet"
	}
	if v.Generated != nil {
		expectedType = "ReportGeneration"
	}
	if v.AggregateType != expectedType || v.AggregateID != expectedAggregate {
		return nil, ErrSourceIdentity
	}
	if outer.org == nil {
		v.ResolverGaps = append(v.ResolverGaps, "outer_organization_absent")
	} else if *outer.org <= 0 || uint64(*outer.org) != v.OrgID {
		return nil, ErrSourceOrganization
	}
	return v, nil
}
