package answersheet

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
)

// AdmissionPurpose aliases the durable event purpose codes.
type AdmissionPurpose = eventpayload.AdmissionPurpose

const (
	AdmissionPurposeIndependentQuestionnaire = eventpayload.AdmissionPurposeIndependentQuestionnaire
	AdmissionPurposeAssessment               = eventpayload.AdmissionPurposeAssessment
)

// Admission freezes evaluation intent at answersheet accept time (EV-R001).
type Admission struct {
	purpose              AdmissionPurpose
	questionnaireCode    string
	questionnaireVersion string
	modelKind            string
	modelSubKind         string
	modelAlgorithm       string
	modelCode            string
	modelVersion         string
	modelTitle           string
}

// NewIndependentAdmission marks a submission that must not create an Assessment.
func NewIndependentAdmission(questionnaireCode, questionnaireVersion string) (Admission, error) {
	a := Admission{
		purpose:              AdmissionPurposeIndependentQuestionnaire,
		questionnaireCode:    strings.TrimSpace(questionnaireCode),
		questionnaireVersion: strings.TrimSpace(questionnaireVersion),
	}
	if err := a.Validate(); err != nil {
		return Admission{}, err
	}
	return a, nil
}

// NewAssessmentAdmission freezes an exact model release for this submission.
func NewAssessmentAdmission(
	questionnaireCode, questionnaireVersion string,
	modelKind, modelSubKind, modelAlgorithm, modelCode, modelVersion, modelTitle string,
) (Admission, error) {
	a := Admission{
		purpose:              AdmissionPurposeAssessment,
		questionnaireCode:    strings.TrimSpace(questionnaireCode),
		questionnaireVersion: strings.TrimSpace(questionnaireVersion),
		modelKind:            strings.TrimSpace(modelKind),
		modelSubKind:         strings.TrimSpace(modelSubKind),
		modelAlgorithm:       strings.TrimSpace(modelAlgorithm),
		modelCode:            strings.TrimSpace(modelCode),
		modelVersion:         strings.TrimSpace(modelVersion),
		modelTitle:           strings.TrimSpace(modelTitle),
	}
	if err := a.Validate(); err != nil {
		return Admission{}, err
	}
	return a, nil
}

func (a Admission) Validate() error {
	if a.purpose != AdmissionPurposeIndependentQuestionnaire && a.purpose != AdmissionPurposeAssessment {
		return fmt.Errorf("admission purpose is required")
	}
	if a.questionnaireCode == "" || a.questionnaireVersion == "" {
		return fmt.Errorf("admission questionnaire ref is required")
	}
	if a.purpose == AdmissionPurposeAssessment {
		if a.modelKind == "" || a.modelCode == "" || a.modelVersion == "" {
			return fmt.Errorf("assessment admission requires model kind/code/version")
		}
	}
	return nil
}

func (a Admission) IsZero() bool {
	return a.purpose == ""
}

func (a Admission) RequiresAssessment() bool {
	return a.purpose == AdmissionPurposeAssessment
}

func (a Admission) Purpose() AdmissionPurpose    { return a.purpose }
func (a Admission) QuestionnaireCode() string    { return a.questionnaireCode }
func (a Admission) QuestionnaireVersion() string { return a.questionnaireVersion }
func (a Admission) ModelKind() string            { return a.modelKind }
func (a Admission) ModelSubKind() string         { return a.modelSubKind }
func (a Admission) ModelAlgorithm() string       { return a.modelAlgorithm }
func (a Admission) ModelCode() string            { return a.modelCode }
func (a Admission) ModelVersion() string         { return a.modelVersion }
func (a Admission) ModelTitle() string           { return a.modelTitle }

// ToEventPayload projects the domain admission into the durable event body.
func (a Admission) ToEventPayload() *eventpayload.AssessmentAdmission {
	if a.IsZero() {
		return nil
	}
	return &eventpayload.AssessmentAdmission{
		Purpose:              a.purpose,
		QuestionnaireCode:    a.questionnaireCode,
		QuestionnaireVersion: a.questionnaireVersion,
		ModelKind:            a.modelKind,
		ModelAlgorithm:       a.modelAlgorithm,
		ModelCode:            a.modelCode,
		ModelVersion:         a.modelVersion,
		ModelTitle:           a.modelTitle,
	}
}

// AdmissionFromEventPayload rebuilds domain admission from a durable event payload.
func AdmissionFromEventPayload(p *eventpayload.AssessmentAdmission) (Admission, error) {
	if p == nil {
		return Admission{}, nil
	}
	switch p.Purpose {
	case AdmissionPurposeIndependentQuestionnaire:
		return NewIndependentAdmission(p.QuestionnaireCode, p.QuestionnaireVersion)
	case AdmissionPurposeAssessment:
		return NewAssessmentAdmission(
			p.QuestionnaireCode, p.QuestionnaireVersion,
			p.ModelKind, string(modelcatalog.CanonicalSubKindFor(modelcatalog.Kind(p.ModelKind))), p.ModelAlgorithm,
			p.ModelCode, p.ModelVersion, p.ModelTitle,
		)
	default:
		return Admission{}, fmt.Errorf("unknown admission purpose %q", p.Purpose)
	}
}
