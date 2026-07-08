// Package catalog is a transitional compat facade; canonical definitions live in publishing.
package catalog

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
)

type (
	QuestionnaireBinding   = publishing.QuestionnaireBinding
	ModelDefinition        = publishing.ModelDefinition
	DecisionSpec           = publishing.DecisionSpec
	SourceRef              = publishing.SourceRef
	PublishedModelSnapshot = publishing.PublishedModelSnapshot

	AssessmentModel         = publishing.AssessmentModel
	NewAssessmentModelInput = publishing.NewAssessmentModelInput
	DefinitionPayload       = publishing.DefinitionPayload
	ModelStatus             = publishing.ModelStatus
	ValidationLevel         = publishing.ValidationLevel
	DomainValidationIssue   = publishing.DomainValidationIssue
	DomainValidationResult  = publishing.DomainValidationResult
)

const (
	SchemaVersionV1 = publishing.SchemaVersionV1
	SchemaVersionV2 = publishing.SchemaVersionV2

	ModelStatusDraft     = publishing.ModelStatusDraft
	ModelStatusPublished = publishing.ModelStatusPublished
	ModelStatusArchived  = publishing.ModelStatusArchived

	ValidationLevelError   = publishing.ValidationLevelError
	ValidationLevelWarning = publishing.ValidationLevelWarning
)

var (
	ErrInvalidArgument = publishing.ErrInvalidArgument
	ErrInvalidState    = publishing.ErrInvalidState

	NewAssessmentModel = publishing.NewAssessmentModel
	ParseModelStatus   = publishing.ParseModelStatus
)

// DecisionKindScoreRangeInterpretation kept for catalog callers decoding legacy snapshots.
const DecisionKindScoreRangeInterpretation binding.DecisionKind = "score_range_interpretation"
