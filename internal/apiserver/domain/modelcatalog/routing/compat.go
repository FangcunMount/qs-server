// Package routing is a transitional compat facade; canonical definitions live in publishing.
package routing

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"

type (
	AlgorithmFamily = publishing.AlgorithmFamily
	ExecutionPath   = publishing.ExecutionPath
)

const (
	AlgorithmFamilyFactorScoring        = publishing.AlgorithmFamilyFactorScoring
	AlgorithmFamilyFactorClassification = publishing.AlgorithmFamilyFactorClassification
	AlgorithmFamilyFactorNorm           = publishing.AlgorithmFamilyFactorNorm
	AlgorithmFamilyTaskPerformance      = publishing.AlgorithmFamilyTaskPerformance

	ExecutionPathNone                       = publishing.ExecutionPathNone
	ExecutionPathScaleDescriptor            = publishing.ExecutionPathScaleDescriptor
	ExecutionPathTypologyDescriptor         = publishing.ExecutionPathTypologyDescriptor
	ExecutionPathBehavioralRatingDescriptor = publishing.ExecutionPathBehavioralRatingDescriptor
	ExecutionPathCognitiveDescriptor        = publishing.ExecutionPathCognitiveDescriptor

	PayloadFormatAssessmentScaleV1         = publishing.PayloadFormatAssessmentScaleV1
	PayloadFormatPersonalityTypologyV1     = publishing.PayloadFormatPersonalityTypologyV1
	PayloadFormatBehavioralRatingDefaultV1 = publishing.PayloadFormatBehavioralRatingDefaultV1
	PayloadFormatBehavioralRatingBrief2V1  = publishing.PayloadFormatBehavioralRatingBrief2V1
	PayloadFormatCognitiveDefaultV1        = publishing.PayloadFormatCognitiveDefaultV1
	PayloadFormatCognitiveSPMV1            = publishing.PayloadFormatCognitiveSPMV1
	PayloadFormatScaleV1                   = publishing.PayloadFormatScaleV1
	PayloadFormatMBTIV1                    = publishing.PayloadFormatMBTIV1 //nolint:staticcheck // legacy decode data value
	PayloadFormatSBTIV1                    = publishing.PayloadFormatSBTIV1 //nolint:staticcheck // legacy decode data value
	PayloadFormatScaleV1Legacy             = publishing.PayloadFormatScaleV1Legacy
	PayloadFormatMBTIV1Legacy              = publishing.PayloadFormatMBTIV1Legacy
	PayloadFormatSBTIV1Legacy              = publishing.PayloadFormatSBTIV1Legacy
)

var (
	AlgorithmFamilyFromDecisionKind = publishing.AlgorithmFamilyFromDecisionKind
	DecisionKindForIdentity         = publishing.DecisionKindForIdentity
	AlgorithmFamilyFromIdentity     = publishing.AlgorithmFamilyFromIdentity
	AllAlgorithmFamilies            = publishing.AllAlgorithmFamilies

	IsScalePayloadFormat               = publishing.IsScalePayloadFormat
	IsMBTIPayloadFormat                = publishing.IsMBTIPayloadFormat
	IsSBTIPayloadFormat                = publishing.IsSBTIPayloadFormat
	IsPersonalityTypologyPayloadFormat = publishing.IsPersonalityTypologyPayloadFormat
	AlgorithmFromTypologyPayload       = publishing.AlgorithmFromTypologyPayload
	PayloadFormatForBehavioralRating   = publishing.PayloadFormatForBehavioralRating
	PayloadFormatForCognitive          = publishing.PayloadFormatForCognitive
	IsBehavioralRatingPayloadFormat    = publishing.IsBehavioralRatingPayloadFormat
	IsCognitivePayloadFormat           = publishing.IsCognitivePayloadFormat
	DraftPayloadFormatForModel         = publishing.DraftPayloadFormatForModel
)
