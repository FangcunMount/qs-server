package ruleset

import (
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
	rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/mbti"
	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/sbti"
	rulesetscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/codec"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleset"
)

func SBTIRuleSetSnapshot(model *rulesetsbti.ModelSnapshot) (*domain.RuleSetSnapshot, error) {
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
		Definition: domain.RuleSetDefinition{
			Kind:    domain.RuleSetKindSBTI,
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

func MBTIRuleSetSnapshot(model *rulesetmbti.ModelSnapshot) (*domain.RuleSetSnapshot, error) {
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
		Definition: domain.RuleSetDefinition{
			Kind:    domain.RuleSetKindMBTI,
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

func ScaleRuleSetSnapshot(model *rulesetscale.ScaleSnapshot) (*domain.RuleSetSnapshot, error) {
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
		Definition: domain.RuleSetDefinition{
			Kind:    domain.RuleSetKindScale,
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

func RuleSetRefFromSnapshot(snapshot *domain.RuleSetSnapshot) port.RuleSetRef {
	if snapshot == nil {
		return port.RuleSetRef{}
	}
	return port.RuleSetRef{
		Kind:    snapshot.Definition.Kind,
		Code:    snapshot.Definition.Code,
		Version: snapshot.Definition.Version,
		Title:   snapshot.Definition.Title,
	}
}

func sbtiRuleSetRef(model *rulesetsbti.ModelSnapshot) port.RuleSetRef {
	if model == nil {
		return port.RuleSetRef{}
	}
	return port.RuleSetRef{
		Kind:    domain.RuleSetKindSBTI,
		Code:    model.Code,
		Version: model.Version,
		Title:   model.Title,
	}
}

func mbtiRuleSetRef(model *rulesetmbti.ModelSnapshot) port.RuleSetRef {
	if model == nil {
		return port.RuleSetRef{}
	}
	return port.RuleSetRef{
		Kind:    domain.RuleSetKindMBTI,
		Code:    model.Code,
		Version: model.Version,
		Title:   model.Title,
	}
}
