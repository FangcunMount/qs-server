package answersheet

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
)

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
