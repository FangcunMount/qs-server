package response

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type AssessmentModelResponse = modelcatalog.ModelSummary
type AssessmentModelListResponse = modelcatalog.ModelListResult
type AssessmentModelQuestionnaireResponse = modelcatalog.QuestionnaireBindingResult
type AssessmentModelDefinitionResponse = domain.Definition
type AssessmentModelOptionsResponse = modelcatalog.OptionsResult
type AssessmentModelValidationResponse = modelcatalog.ValidationResult
type AssessmentModelPreviewReportResponse = modelcatalog.PreviewReportResult
type PublishedAssessmentModelResponse = modelcatalog.PublishedModelDetail
type PublishedAssessmentModelListResponse = modelcatalog.PublishedModelListResult
type HotAssessmentModelListResponse = modelcatalog.HotModelListResult

type AssessmentModelCodesResponse struct {
	Codes []string `json:"codes"`
}

type AssessmentModelImageUploadResponse = modelcatalog.OutcomeImageUploadResult
