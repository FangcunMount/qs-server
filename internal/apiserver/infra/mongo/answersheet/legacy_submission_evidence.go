package answersheet

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"go.mongodb.org/mongo-driver/bson"
)

// LegacySubmissionBaseline deliberately has no durable-acceptance marker or
// original event ID: both must remain absent on a markerless historical sheet.
type LegacySubmissionBaseline struct {
	snapshot *retirementevidence.HistoricalSetSnapshot
	payload  eventpayload.AnswerSheetSubmittedData
	binding  string
}

func (b *LegacySubmissionBaseline) BindingSHA256() string {
	if b == nil {
		return ""
	}
	return b.binding
}
func (b *LegacySubmissionBaseline) Payload() eventpayload.AnswerSheetSubmittedData {
	if b == nil {
		return eventpayload.AnswerSheetSubmittedData{}
	}
	value := b.payload
	if value.Admission != nil {
		clone := *value.Admission
		value.Admission = &clone
	}
	if value.Attribution != nil {
		clone := *value.Attribution
		value.Attribution = &clone
	}
	return value
}

// LegacySubmissionPayload accepts only an independently frozen no-assessment
// intent. Missing Admission/Assessment is never evidence that work is closed.
// Assessment-intended rows require a separately trusted SQL closure adapter and
// remain unsupported here rather than accepting a caller-supplied terminal bool.
func LegacySubmissionPayload(row AnswerSheetPO) (eventpayload.AnswerSheetSubmittedData, error) {
	a := row.Admission
	if row.DomainID.IsZero() || row.DurableAcceptance != nil || row.OrgID == 0 || row.TesteeID == 0 || row.FillerID <= 0 || row.FillerType == "" || row.FilledAt.IsZero() || row.QuestionnaireCode == "" || row.QuestionnaireVersion == "" || a == nil || a.Purpose != string(eventpayload.AdmissionPurposeIndependentQuestionnaire) || a.QuestionnaireCode != row.QuestionnaireCode || a.QuestionnaireVersion != row.QuestionnaireVersion || a.ModelKind != "" || a.ModelSubKind != "" || a.ModelAlgorithm != "" || a.ModelCode != "" || a.ModelVersion != "" || a.ModelTitle != "" {
		return eventpayload.AnswerSheetSubmittedData{}, fmt.Errorf("%w: markerless submission lacks a trustworthy frozen independent admission", retirementevidence.ErrUnverifiable)
	}
	requestID := ""
	if row.SubmitMeta != nil {
		requestID = row.SubmitMeta.RequestID
	}
	p := eventpayload.AnswerSheetSubmittedData{AnswerSheetID: row.DomainID.String(), OrgID: row.OrgID, TesteeID: row.TesteeID, QuestionnaireCode: row.QuestionnaireCode, QuestionnaireVersion: row.QuestionnaireVersion, FillerID: uint64(row.FillerID), FillerType: row.FillerType, TaskID: row.TaskID, RequestID: requestID, SubmittedAt: row.FilledAt, Admission: &eventpayload.AssessmentAdmission{Purpose: eventpayload.AdmissionPurposeIndependentQuestionnaire, QuestionnaireCode: a.QuestionnaireCode, QuestionnaireVersion: a.QuestionnaireVersion}}
	if v := row.Attribution; v != nil {
		p.Attribution = &eventpayload.AttributionSnapshot{OriginType: v.OriginType, OriginID: v.OriginID, ClinicianID: v.ClinicianID, EntryID: v.EntryID, PlanID: v.PlanID, EnrollmentID: v.EnrollmentID, TaskID: v.TaskID, CapturedAt: v.CapturedAt, Version: v.Version, Mode: v.Mode}
	}
	return p, nil
}

func (s *RetirementEvidenceStore) PrepareLegacySubmission(ctx context.Context, id uint64) (*LegacySubmissionBaseline, error) {
	if s == nil || s.collection == nil || id == 0 {
		return nil, retirementevidence.ErrUnverifiable
	}
	snapshot, err := retirementevidence.ReadHistoricalSet(ctx, s.collection, bson.M{"domain_id": id, "deleted_at": nil}, "legacy_submission_evidence")
	if err != nil {
		return nil, err
	}
	var row AnswerSheetPO
	if err := snapshot.Decode(&row); err != nil {
		return nil, err
	}
	if row.DomainID.Uint64() != id {
		return nil, retirementevidence.ErrUnverifiable
	}
	payload, err := LegacySubmissionPayload(row)
	if err != nil {
		return nil, err
	}
	binding, err := eventevidencebinding.AnswerSheet(payload)
	if err != nil {
		return nil, err
	}
	return &LegacySubmissionBaseline{snapshot: snapshot, payload: payload, binding: binding}, nil
}

func (s *RetirementEvidenceStore) BackfillLegacySubmission(ctx context.Context, baseline *LegacySubmissionBaseline, entry evidence.HistoricalReferenceEntryV1) error {
	if s == nil || baseline == nil {
		return retirementevidence.ErrUnverifiable
	}
	if err := entry.Validate(); err != nil {
		return err
	}
	if entry.EventType != eventcatalog.AnswerSheetSubmitted || entry.Run != nil || entry.Proof.BusinessBindingSHA256 != baseline.binding {
		return retirementevidence.ErrConflict
	}
	return baseline.snapshot.Append(ctx, s.collection, entry)
}
