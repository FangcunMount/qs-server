package response

import "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"

type AssessmentModelResponse = modelcatalog.ModelSummary
type AssessmentModelListResponse = modelcatalog.ModelListResult
type AssessmentModelQuestionnaireResponse = modelcatalog.QuestionnaireBindingResult
type AssessmentModelDefinitionResponse = modelcatalog.DefinitionDTO
type AssessmentModelOptionsResponse = modelcatalog.OptionsResult
type AssessmentModelValidationResponse = modelcatalog.ValidationResult
type AssessmentModelPreviewReportResponse = modelcatalog.PreviewReportResult

type AssessmentModelCodesResponse struct {
	Codes []string `json:"codes"`
}
