package interpretationmodel

import (
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretationmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/interpretationmodel/codec"
	evaluationinputPort "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationmodel"
)

func SBTIRuleSetSnapshot(model *evaluationinputPort.SBTIModelSnapshot) (*domain.RuleSetSnapshot, error) {
	payload, format, err := codec.EncodeSBTI(model)
	if err != nil {
		return nil, err
	}
	status := model.Status
	if status == "" {
		status = "published"
	}
	return &domain.RuleSetSnapshot{
		SchemaVersion: domain.RuleSetSchemaVersionV1,
		PayloadFormat: format,
		Definition: domain.ModelDefinition{
			Kind:    domain.ModelKindSBTI,
			Code:    model.Code,
			Version: model.Version,
			Title:   model.Title,
			Status:  status,
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    model.QuestionnaireCode,
			QuestionnaireVersion: model.QuestionnaireVersion,
		},
		DecisionKind: domain.DecisionKindNearestPattern,
		Source: map[string]any{
			"wiki_repo":      model.Source.WikiRepo,
			"source_site":    model.Source.SourceSite,
			"license":        model.Source.License,
			"attribution":    model.Source.Attribution,
			"non_commercial": model.Source.NonCommercial,
			"image_base_url": model.Source.ImageBaseURL,
		},
		Payload: payload,
	}, nil
}

func MBTIRuleSetSnapshot(model *evaluationinputPort.MBTIModelSnapshot) (*domain.RuleSetSnapshot, error) {
	payload, format, err := codec.EncodeMBTI(model)
	if err != nil {
		return nil, err
	}
	status := model.Status
	if status == "" {
		status = "published"
	}
	return &domain.RuleSetSnapshot{
		SchemaVersion: domain.RuleSetSchemaVersionV1,
		PayloadFormat: format,
		Definition: domain.ModelDefinition{
			Kind:    domain.ModelKindMBTI,
			Code:    model.Code,
			Version: model.Version,
			Title:   model.Title,
			Status:  status,
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    model.QuestionnaireCode,
			QuestionnaireVersion: model.QuestionnaireVersion,
		},
		DecisionKind: domain.DecisionKindPoleComposition,
		Source: map[string]any{
			"questions_repo": model.Source.QuestionsRepo,
			"source_site":    model.Source.SourceSite,
			"license":        model.Source.License,
			"attribution":    model.Source.Attribution,
			"non_commercial": model.Source.NonCommercial,
		},
		Payload: payload,
	}, nil
}

func ScaleRuleSetSnapshot(model *evaluationinputPort.ScaleSnapshot) (*domain.RuleSetSnapshot, error) {
	if model == nil {
		return nil, fmt.Errorf("scale model is nil")
	}
	payload, format, err := codec.EncodeScale(model)
	if err != nil {
		return nil, err
	}
	status := model.Status
	if status == "" {
		status = "published"
	}
	version := model.ScaleVersion
	if version == "" {
		version = model.QuestionnaireVersion
	}
	return &domain.RuleSetSnapshot{
		SchemaVersion: domain.RuleSetSchemaVersionV1,
		PayloadFormat: format,
		Definition: domain.ModelDefinition{
			Kind:    domain.ModelKindScale,
			Code:    model.Code,
			Version: version,
			Title:   model.Title,
			Status:  status,
		},
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    model.QuestionnaireCode,
			QuestionnaireVersion: model.QuestionnaireVersion,
		},
		DecisionKind: domain.DecisionKindScoreRangeInterpretation,
		Source:       map[string]any{},
		Payload:      payload,
	}, nil
}

func ModelRefFromSnapshot(snapshot *domain.RuleSetSnapshot) port.ModelRef {
	if snapshot == nil {
		return port.ModelRef{}
	}
	return port.ModelRef{
		Kind:    snapshot.Definition.Kind,
		Code:    snapshot.Definition.Code,
		Version: snapshot.Definition.Version,
		Title:   snapshot.Definition.Title,
	}
}

func sbtiModelRef(model *evaluationinputPort.SBTIModelSnapshot) port.ModelRef {
	if model == nil {
		return port.ModelRef{}
	}
	return port.ModelRef{
		Kind:    domain.ModelKindSBTI,
		Code:    model.Code,
		Version: model.Version,
		Title:   model.Title,
	}
}

func mbtiModelRef(model *evaluationinputPort.MBTIModelSnapshot) port.ModelRef {
	if model == nil {
		return port.ModelRef{}
	}
	return port.ModelRef{
		Kind:    domain.ModelKindMBTI,
		Code:    model.Code,
		Version: model.Version,
		Title:   model.Title,
	}
}
