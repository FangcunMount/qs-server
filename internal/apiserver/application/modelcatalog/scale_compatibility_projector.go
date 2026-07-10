package modelcatalog

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

// ScaleCompatibilityProjector is the only compatibility boundary for legacy
// scale REST/gRPC views. It projects DefinitionV2 and never decodes payload.
type ScaleCompatibilityProjector struct{}

func (ScaleCompatibilityProjector) ProjectPublished(model *modelcatalogport.PublishedModel) (*shared.ScaleResult, error) {
	if model == nil || model.Kind != domain.KindScale {
		return nil, fmt.Errorf("published scale model is required")
	}
	if model.DefinitionV2 == nil {
		return nil, fmt.Errorf("published scale definition_v2 is required")
	}
	legacy, _ := modelcatalogport.LegacyScaleBindingFromPublished(model)
	version := legacy.ScaleVersion
	if version == "" {
		version = model.Version
	}
	snapshot := scalepayload.ScaleSnapshotFromDefinition(scalepayload.ExecutionEnvelope{
		ID:                   legacy.MedicalScaleID,
		Code:                 model.Code,
		ScaleVersion:         version,
		Title:                model.Title,
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Status:               model.Status,
	}, model.DefinitionV2)
	if snapshot == nil {
		return nil, fmt.Errorf("published scale definition_v2 cannot be projected")
	}
	return scaleResultFromProjection(model.Code, model.Title, model.Description, model.Category, model.Stages, model.ApplicableAges, model.Reporters, model.Tags, snapshot), nil
}

func (ScaleCompatibilityProjector) ProjectDraft(model *domain.AssessmentModel) (*shared.ScaleResult, error) {
	if model == nil || model.Kind != domain.KindScale {
		return nil, fmt.Errorf("draft scale model is required")
	}
	if model.DefinitionV2 == nil {
		return nil, fmt.Errorf("draft scale definition_v2 is required")
	}
	snapshot := scalepayload.ScaleSnapshotFromDefinition(scalepayload.ExecutionEnvelope{
		Code:                 model.Code,
		ScaleVersion:         "v" + fmt.Sprint(model.Revision()),
		Title:                model.Title,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status:               string(model.Status),
	}, model.DefinitionV2)
	if snapshot == nil {
		return nil, fmt.Errorf("draft scale definition_v2 cannot be projected")
	}
	return scaleResultFromProjection(model.Code, model.Title, model.Description, model.Category, model.Stages, model.ApplicableAges, model.Reporters, model.Tags, snapshot), nil
}

func scaleResultFromProjection(code, title, description, category string, stages, ages, reporters, tags []string, snapshot *scalepayload.ScaleSnapshot) *shared.ScaleResult {
	result := &shared.ScaleResult{
		Code:                 code,
		ScaleVersion:         snapshot.ScaleVersion,
		Title:                title,
		Description:          description,
		Category:             category,
		Stages:               append([]string(nil), stages...),
		ApplicableAges:       append([]string(nil), ages...),
		Reporters:            append([]string(nil), reporters...),
		Tags:                 append([]string(nil), tags...),
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Status:               snapshot.Status,
		Factors:              make([]shared.FactorResult, 0, len(snapshot.Factors)),
	}
	for _, factor := range snapshot.Factors {
		projected := shared.FactorResult{
			Code:            factor.Code,
			Title:           factor.Title,
			IsTotalScore:    factor.IsTotalScore,
			QuestionCodes:   append([]string(nil), factor.QuestionCodes...),
			ScoringStrategy: factor.ScoringStrategy,
			ScoringParams:   map[string]interface{}{},
			MaxScore:        cloneScaleFloat64(factor.MaxScore),
			RiskLevel:       "none",
			InterpretRules:  make([]shared.InterpretRuleResult, 0, len(factor.InterpretRules)),
		}
		if factor.ScoringStrategy == "cnt" && len(factor.ScoringParams.CntOptionContents) > 0 {
			projected.ScoringParams["cnt_option_contents"] = append([]string(nil), factor.ScoringParams.CntOptionContents...)
		}
		for index, rule := range factor.InterpretRules {
			projected.InterpretRules = append(projected.InterpretRules, shared.InterpretRuleResult{MinScore: rule.Min, MaxScore: rule.Max, RiskLevel: rule.RiskLevel, Conclusion: rule.Conclusion, Suggestion: rule.Suggestion})
			if index == 0 {
				projected.RiskLevel = rule.RiskLevel
			}
		}
		result.Factors = append(result.Factors, projected)
	}
	return result
}

func cloneScaleFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
