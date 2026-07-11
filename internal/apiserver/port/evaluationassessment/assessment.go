// Package evaluationassessment contains the temporary legacy Assessment read
// contract used only by compatibility report projections.
package evaluationassessment

import domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"

type (
	Assessment          = domainassessment.Assessment
	ID                  = domainassessment.ID
	EvaluationModelRef  = domainassessment.EvaluationModelRef
	EvaluationModelKind = domainassessment.EvaluationModelKind
	RiskLevel           = domainassessment.RiskLevel
)

const (
	EvaluationModelKindPersonality = domainassessment.EvaluationModelKindPersonality
	RiskLevelMedium                = domainassessment.RiskLevelMedium
)

var (
	NewEvaluationModelRefWithIdentity = domainassessment.NewEvaluationModelRefWithIdentity
	NewScaleEvaluationModelRef        = domainassessment.NewScaleEvaluationModelRef
	NewAssessment                     = domainassessment.NewAssessment
	NewQuestionnaireRefByCode         = domainassessment.NewQuestionnaireRefByCode
	NewAnswerSheetRef                 = domainassessment.NewAnswerSheetRef
	NewAdhocOrigin                    = domainassessment.NewAdhocOrigin
	WithID                            = domainassessment.WithID
	WithEvaluationModel               = domainassessment.WithEvaluationModel
	NewInvalidStatusError             = domainassessment.NewInvalidStatusError
	NewID                             = domainassessment.NewID
)
