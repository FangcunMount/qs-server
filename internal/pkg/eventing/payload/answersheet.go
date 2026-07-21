package eventpayload

import "time"

// AdmissionPurpose describes why an answersheet was accepted into Evaluation.
type AdmissionPurpose string

const (
	AdmissionPurposeIndependentQuestionnaire AdmissionPurpose = "independent_questionnaire"
	AdmissionPurposeAssessment               AdmissionPurpose = "assessment"
)

// AssessmentAdmission freezes the evaluation intent resolved at submit time (EV-R001).
type AssessmentAdmission struct {
	Purpose              AdmissionPurpose `json:"purpose"`
	QuestionnaireCode    string           `json:"questionnaire_code,omitempty"`
	QuestionnaireVersion string           `json:"questionnaire_version,omitempty"`
	ModelKind            string           `json:"model_kind,omitempty"`
	ModelSubKind         string           `json:"model_sub_kind,omitempty"`
	ModelAlgorithm       string           `json:"model_algorithm,omitempty"`
	ModelCode            string           `json:"model_code,omitempty"`
	ModelVersion         string           `json:"model_version,omitempty"`
	ModelTitle           string           `json:"model_title,omitempty"`
}

// AnswerSheetSubmittedData is the answer sheet submitted event body.
type AnswerSheetSubmittedData struct {
	AnswerSheetID        string               `json:"answersheet_id"`
	QuestionnaireCode    string               `json:"questionnaire_code"`
	QuestionnaireVersion string               `json:"questionnaire_version"`
	TesteeID             uint64               `json:"testee_id"`
	OrgID                uint64               `json:"org_id"`
	FillerID             uint64               `json:"filler_id"`
	FillerType           string               `json:"filler_type"`
	TaskID               string               `json:"task_id,omitempty"`
	RequestID            string               `json:"request_id,omitempty"`
	SubmittedAt          time.Time            `json:"submitted_at"`
	Admission            *AssessmentAdmission `json:"admission,omitempty"`
	Attribution          *AttributionSnapshot `json:"attribution,omitempty"`
}

type AttributionSnapshot struct {
	OriginType   string    `json:"origin_type"`
	OriginID     string    `json:"origin_id,omitempty"`
	ClinicianID  string    `json:"clinician_id,omitempty"`
	EntryID      string    `json:"entry_id,omitempty"`
	PlanID       string    `json:"plan_id,omitempty"`
	EnrollmentID string    `json:"enrollment_id,omitempty"`
	TaskID       string    `json:"task_id,omitempty"`
	CapturedAt   time.Time `json:"captured_at"`
	Version      uint32    `json:"version"`
	Mode         string    `json:"mode"`
}
