package response

import "github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel"

type AssessmentModelResponse = assessmentmodel.ModelSummary
type AssessmentModelListResponse = assessmentmodel.ModelListResult
type AssessmentModelQuestionnaireResponse = assessmentmodel.QuestionnaireBindingResult
type AssessmentModelDefinitionResponse = assessmentmodel.DefinitionDTO
type AssessmentModelOptionsResponse = assessmentmodel.OptionsResult
type AssessmentModelValidationResponse = assessmentmodel.ValidationResult
type AssessmentModelPreviewReportResponse = assessmentmodel.PreviewReportResult

type AssessmentModelCodesResponse struct {
	Codes []string `json:"codes"`
}
