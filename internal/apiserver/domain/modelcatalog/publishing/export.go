// Package publishing is the mechanism-oriented home for publish/snapshot governance (§19).
package publishing

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/capability"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

type (
	AlgorithmFamily = routing.AlgorithmFamily
	ExecutionPath   = routing.ExecutionPath

	ModelDefinition        = catalog.ModelDefinition
	DecisionSpec           = catalog.DecisionSpec
	SourceRef              = catalog.SourceRef
	PublishedModelSnapshot = catalog.PublishedModelSnapshot

	AssessmentModel         = catalog.AssessmentModel
	NewAssessmentModelInput = catalog.NewAssessmentModelInput
	DefinitionPayload       = catalog.DefinitionPayload
	ModelStatus             = catalog.ModelStatus
	ValidationLevel         = catalog.ValidationLevel
	DomainValidationIssue   = catalog.DomainValidationIssue
	DomainValidationResult  = catalog.DomainValidationResult

	CatalogOperation = capability.CatalogOperation
)

const (
	AlgorithmFamilyFactorScoring        = routing.AlgorithmFamilyFactorScoring
	AlgorithmFamilyFactorClassification = routing.AlgorithmFamilyFactorClassification
	AlgorithmFamilyFactorNorm           = routing.AlgorithmFamilyFactorNorm
	AlgorithmFamilyTaskPerformance      = routing.AlgorithmFamilyTaskPerformance

	ExecutionPathNone                       = routing.ExecutionPathNone
	ExecutionPathScaleDescriptor            = routing.ExecutionPathScaleDescriptor
	ExecutionPathTypologyDescriptor         = routing.ExecutionPathTypologyDescriptor
	ExecutionPathBehavioralRatingDescriptor = routing.ExecutionPathBehavioralRatingDescriptor
	ExecutionPathCognitiveDescriptor        = routing.ExecutionPathCognitiveDescriptor

	PayloadFormatAssessmentScaleV1         = routing.PayloadFormatAssessmentScaleV1
	PayloadFormatPersonalityTypologyV1     = routing.PayloadFormatPersonalityTypologyV1
	PayloadFormatBehavioralRatingDefaultV1 = routing.PayloadFormatBehavioralRatingDefaultV1
	PayloadFormatBehavioralRatingBrief2V1  = routing.PayloadFormatBehavioralRatingBrief2V1
	PayloadFormatCognitiveDefaultV1        = routing.PayloadFormatCognitiveDefaultV1
	PayloadFormatCognitiveSPMV1            = routing.PayloadFormatCognitiveSPMV1
	PayloadFormatScaleV1                   = routing.PayloadFormatScaleV1
	PayloadFormatMBTIV1                    = routing.PayloadFormatMBTIV1 //nolint:staticcheck // legacy decode data value
	PayloadFormatSBTIV1                    = routing.PayloadFormatSBTIV1 //nolint:staticcheck // legacy decode data value
	PayloadFormatScaleV1Legacy             = routing.PayloadFormatScaleV1Legacy
	PayloadFormatMBTIV1Legacy              = routing.PayloadFormatMBTIV1Legacy
	PayloadFormatSBTIV1Legacy              = routing.PayloadFormatSBTIV1Legacy

	CatalogOpCreate            = capability.CatalogOpCreate
	CatalogOpList              = capability.CatalogOpList
	CatalogOpUpdateBasicInfo   = capability.CatalogOpUpdateBasicInfo
	CatalogOpDelete            = capability.CatalogOpDelete
	CatalogOpPublish           = capability.CatalogOpPublish
	CatalogOpUnpublish         = capability.CatalogOpUnpublish
	CatalogOpArchive           = capability.CatalogOpArchive
	CatalogOpBindQuestionnaire = capability.CatalogOpBindQuestionnaire
	CatalogOpUpdateDefinition  = capability.CatalogOpUpdateDefinition
	CatalogOpPreview           = capability.CatalogOpPreview
	CatalogOpQRCode            = capability.CatalogOpQRCode

	SchemaVersionV1 = catalog.SchemaVersionV1
	SchemaVersionV2 = catalog.SchemaVersionV2

	ModelStatusDraft     = catalog.ModelStatusDraft
	ModelStatusPublished = catalog.ModelStatusPublished
	ModelStatusArchived  = catalog.ModelStatusArchived

	ValidationLevelError   = catalog.ValidationLevelError
	ValidationLevelWarning = catalog.ValidationLevelWarning
)

var (
	AlgorithmFamilyFromDecisionKind = routing.AlgorithmFamilyFromDecisionKind
	DecisionKindForIdentity         = routing.DecisionKindForIdentity
	AlgorithmFamilyFromIdentity     = routing.AlgorithmFamilyFromIdentity
	AllAlgorithmFamilies            = routing.AllAlgorithmFamilies

	IsScalePayloadFormat               = routing.IsScalePayloadFormat
	IsMBTIPayloadFormat                = routing.IsMBTIPayloadFormat
	IsSBTIPayloadFormat                = routing.IsSBTIPayloadFormat
	IsPersonalityTypologyPayloadFormat = routing.IsPersonalityTypologyPayloadFormat
	AlgorithmFromTypologyPayload       = routing.AlgorithmFromTypologyPayload
	PayloadFormatForBehavioralRating   = routing.PayloadFormatForBehavioralRating
	PayloadFormatForCognitive          = routing.PayloadFormatForCognitive
	IsBehavioralRatingPayloadFormat    = routing.IsBehavioralRatingPayloadFormat
	IsCognitivePayloadFormat           = routing.IsCognitivePayloadFormat
	DraftPayloadFormatForModel         = routing.DraftPayloadFormatForModel

	NewAssessmentModel = catalog.NewAssessmentModel
	ParseModelStatus   = catalog.ParseModelStatus
)

// DecisionKindScoreRangeInterpretation is a publishing-side decision kind alias.
const DecisionKindScoreRangeInterpretation identity.DecisionKind = "score_range_interpretation"
