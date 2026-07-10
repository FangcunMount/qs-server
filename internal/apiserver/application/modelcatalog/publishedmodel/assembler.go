package publishedmodel

import (
	"fmt"
	"strconv"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type familyBuilder func(*domain.AssessmentModel) (*port.AssessmentSnapshot, error)

var buildersByKind = map[domain.Kind]familyBuilder{
	domain.KindScale:            buildScoring,
	domain.KindTypology:         buildTypology,
	domain.KindBehavioralRating: buildNorming,
	domain.KindCognitive:        buildTaskPerformance,
}

// BuildAssessmentSnapshot materializes an immutable execution snapshot from
// the single editable AssessmentModel aggregate.
func BuildAssessmentSnapshot(model *domain.AssessmentModel) (*port.AssessmentSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	builder, ok := buildersByKind[model.Kind]
	if !ok {
		return nil, fmt.Errorf("unsupported model kind %s for publishing record builder", model.Kind)
	}
	return builder(model)
}

func recordFromModel(model *domain.AssessmentModel, kind domain.Kind, subKind domain.SubKind, algorithm domain.Algorithm, payloadFormat string, decisionKind domain.DecisionKind, payload []byte) *port.AssessmentSnapshot {
	return &port.AssessmentSnapshot{
		SchemaVersion:        domain.SchemaVersionV2,
		PayloadFormat:        payloadFormat,
		ProductChannel:       domain.ResolveProductChannel(model.Kind, model.ProductChannel),
		Kind:                 kind,
		SubKind:              subKind,
		Algorithm:            algorithm,
		Code:                 model.Code,
		Version:              modelVersionString(model),
		Title:                model.Title,
		Description:          model.Description,
		Category:             model.Category,
		Stages:               append([]string(nil), model.Stages...),
		ApplicableAges:       append([]string(nil), model.ApplicableAges...),
		Reporters:            append([]string(nil), model.Reporters...),
		Tags:                 append([]string(nil), model.Tags...),
		Status:               string(domain.ModelStatusPublished),
		DecisionKind:         decisionKind,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Source:               map[string]any{},
		Payload:              payload,
		DefinitionV2:         model.DefinitionV2,
	}
}

func modelVersionString(model *domain.AssessmentModel) string {
	return "v" + strconv.FormatInt(model.Revision(), 10)
}
