package assessmentmodel

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
)

type behaviorDefinitionPayloadDTO struct {
	Dimensions     []behaviorDimensionRule `json:"dimensions"`
	InterpretRules []behaviorInterpretRule `json:"interpret_rules"`
}

type behaviorDimensionRule struct {
	Code            string                 `json:"code"`
	Title           string                 `json:"title"`
	QuestionCodes   []string               `json:"question_codes"`
	ScoringStrategy string                 `json:"scoring_strategy"`
	ScoringParams   map[string]interface{} `json:"scoring_params,omitempty"`
	MaxScore        *float64               `json:"max_score,omitempty"`
	IsTotalScore    bool                   `json:"is_total_score,omitempty"`
	IsShow          bool                   `json:"is_show"`
}

type behaviorInterpretRule struct {
	DimensionCode string               `json:"dimension_code"`
	Ranges        []behaviorScoreRange `json:"ranges"`
}

type behaviorScoreRange struct {
	MinScore   float64 `json:"min_score"`
	MaxScore   float64 `json:"max_score"`
	Conclusion string  `json:"conclusion"`
	Suggestion string  `json:"suggestion,omitempty"`
	Level      string  `json:"level,omitempty"`
}

type personalityDefinitionPayload struct {
	Dimensions   []personalityDimensionDefinition `json:"dimensions"`
	Outcomes     []personalityOutcomeDefinition   `json:"outcomes"`
	ScoringRules map[string]interface{}           `json:"scoring_rules"`
}

type personalityDimensionDefinition struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	LeftPole    string `json:"left_pole,omitempty"`
	RightPole   string `json:"right_pole,omitempty"`
	Description string `json:"description,omitempty"`
}

type personalityOutcomeDefinition struct {
	Code        string                 `json:"code"`
	Title       string                 `json:"title"`
	Summary     string                 `json:"summary,omitempty"`
	Description string                 `json:"description,omitempty"`
	Suggestions []string               `json:"suggestions,omitempty"`
	Rarity      map[string]interface{} `json:"rarity,omitempty"`
}

func behaviorSummaryFromScale(result *scale.ScaleResult) *ModelSummary {
	if result == nil {
		return nil
	}
	summary := behaviorSummaryFromScaleFields(
		result.Code,
		result.Title,
		result.Description,
		result.Status,
		result.Category,
		result.Tags,
		result.QuestionnaireCode,
		result.QuestionnaireVersion,
		result.CreatedAt.Format("2006-01-02 15:04:05"),
		result.UpdatedAt.Format("2006-01-02 15:04:05"),
	)
	return &summary
}

func behaviorSummaryFromScaleSummary(result *scale.ScaleSummaryResult) ModelSummary {
	if result == nil {
		return ModelSummary{}
	}
	return behaviorSummaryFromScaleFields(
		result.Code,
		result.Title,
		result.Description,
		result.Status,
		result.Category,
		result.Tags,
		result.QuestionnaireCode,
		"",
		result.CreatedAt.Format("2006-01-02 15:04:05"),
		result.UpdatedAt.Format("2006-01-02 15:04:05"),
	)
}

func behaviorSummaryFromScaleFields(code, title, description, status, category string, tags []string, questionnaireCode, questionnaireVersion, createdAt, updatedAt string) ModelSummary {
	return ModelSummary{
		Code:                 code,
		Kind:                 KindBehaviorAbility,
		Title:                title,
		Description:          description,
		Status:               status,
		Category:             category,
		Tags:                 tags,
		QuestionnaireCode:    questionnaireCode,
		QuestionnaireVersion: questionnaireVersion,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
	}
}

func personalitySummaryFromSummary(result personalitymodel.PersonalityModelSummaryResult) ModelSummary {
	return ModelSummary{
		Code:                 result.Code,
		Kind:                 KindPersonality,
		Title:                result.Title,
		Description:          result.Description,
		Status:               StatusPublished,
		Category:             result.Algorithm,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
	}
}

func personalitySummaryFromDetail(result *personalitymodel.PersonalityModelResult) *ModelSummary {
	if result == nil {
		return nil
	}
	summary := personalitySummaryFromSummary(result.PersonalityModelSummaryResult)
	return &summary
}

func newBehaviorDefinitionPayload(result *scale.ScaleResult) behaviorDefinitionPayloadDTO {
	payload := behaviorDefinitionPayloadDTO{
		Dimensions:     make([]behaviorDimensionRule, 0, len(result.Factors)),
		InterpretRules: make([]behaviorInterpretRule, 0, len(result.Factors)),
	}
	for _, factor := range result.Factors {
		payload.Dimensions = append(payload.Dimensions, behaviorDimensionRule{
			Code:            factor.Code,
			Title:           factor.Title,
			QuestionCodes:   factor.QuestionCodes,
			ScoringStrategy: factor.ScoringStrategy,
			ScoringParams:   factor.ScoringParams,
			MaxScore:        factor.MaxScore,
			IsTotalScore:    factor.IsTotalScore,
			IsShow:          factor.IsShow,
		})
		rules := make([]behaviorScoreRange, 0, len(factor.InterpretRules))
		for _, rule := range factor.InterpretRules {
			rules = append(rules, behaviorScoreRange{
				MinScore:   rule.MinScore,
				MaxScore:   rule.MaxScore,
				Conclusion: rule.Conclusion,
				Suggestion: rule.Suggestion,
				Level:      rule.RiskLevel,
			})
		}
		payload.InterpretRules = append(payload.InterpretRules, behaviorInterpretRule{
			DimensionCode: factor.Code,
			Ranges:        rules,
		})
	}
	return payload
}

func newPersonalityDefinitionPayload(result *personalitymodel.PersonalityModelResult) personalityDefinitionPayload {
	payload := personalityDefinitionPayload{
		Dimensions:   make([]personalityDimensionDefinition, 0, len(result.Dimensions)),
		Outcomes:     make([]personalityOutcomeDefinition, 0, len(result.Outcomes)),
		ScoringRules: map[string]interface{}{},
	}
	for _, dimension := range result.Dimensions {
		payload.Dimensions = append(payload.Dimensions, personalityDimensionDefinition{
			Code:      dimension.Code,
			Title:     dimension.Name,
			LeftPole:  dimension.LeftPole,
			RightPole: dimension.RightPole,
		})
	}
	for _, outcome := range result.Outcomes {
		payload.Outcomes = append(payload.Outcomes, personalityOutcomeDefinition{
			Code:    outcome.Code,
			Title:   outcome.Name,
			Summary: outcome.OneLiner,
		})
	}
	return payload
}

func containsFold(value, keyword string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(keyword))
}
