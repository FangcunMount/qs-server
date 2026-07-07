package modelcatalog

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/personality"
	personalityconsumer "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/personality/consumer"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

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

func summaryFromBehavior(result *behavior.Model) *ModelSummary {
	if result == nil {
		return nil
	}
	summary := summaryFromBehaviorValue(*result)
	return &summary
}

func summaryFromBehaviorValue(result behavior.Model) ModelSummary {
	summary := ModelSummary{
		Code:                 result.Code,
		Kind:                 KindBehaviorAbility,
		SubKind:              SubKindScale,
		Algorithm:            AlgorithmScoreRange,
		Title:                result.Title,
		Description:          result.Description,
		Status:               result.Status,
		Category:             result.Category,
		Tags:                 result.Tags,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		CreatedAt:            result.CreatedAt,
		UpdatedAt:            result.UpdatedAt,
	}
	populateModelSummaryIdentity(&summary, domain.KindBehaviorAbility, domain.SubKind(SubKindScale), domain.AlgorithmScaleDefault, domain.ProductChannelBehaviorAbility) //nolint:staticcheck // SA1019: behavior_ability legacy product-channel compatibility
	return summary
}

func personalitySummaryFromSummary(result personalityconsumer.PersonalityModelSummaryResult) ModelSummary {
	summary := ModelSummary{
		Code:                 result.Code,
		Kind:                 KindPersonality,
		SubKind:              SubKindTypology,
		Algorithm:            result.Algorithm,
		Title:                result.Title,
		Description:          result.Description,
		Status:               StatusPublished,
		Category:             result.Algorithm,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
	}
	populateModelSummaryIdentity(&summary, domain.KindPersonality, domain.SubKindTypology, domain.Algorithm(result.Algorithm), domain.ProductChannelPersonality)
	return summary
}

func personalitySummaryFromDetail(result *personalityconsumer.PersonalityModelResult) *ModelSummary {
	if result == nil {
		return nil
	}
	summary := personalitySummaryFromSummary(result.PersonalityModelSummaryResult)
	return &summary
}

func newPersonalityDefinitionPayload(result *personalityconsumer.PersonalityModelResult) personalityDefinitionPayload {
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

func definitionFromBehavior(result *behavior.Definition) *DefinitionDTO {
	if result == nil {
		return nil
	}
	return &DefinitionDTO{
		Kind:          result.Kind,
		SubKind:       result.SubKind,
		Algorithm:     result.Algorithm,
		PayloadFormat: result.PayloadFormat,
		Payload:       result.Payload,
	}
}

func previewFromPersonality(result *personality.PreviewReportResult) *PreviewReportResult {
	if result == nil {
		return nil
	}
	sections := make([]PreviewReportSection, len(result.ReportSections))
	for i, section := range result.ReportSections {
		sections[i] = PreviewReportSection{
			Title:   section.Title,
			Content: section.Content,
			Kind:    section.Kind,
		}
	}
	issues := make([]ValidationIssue, len(result.Issues))
	for i, issue := range result.Issues {
		issues[i] = ValidationIssue{
			Field:   issue.Field,
			Message: issue.Message,
			Code:    issue.Code,
			Level:   issue.Level,
		}
	}
	return &PreviewReportResult{
		Outcome: PreviewOutcome{
			Code:  result.Outcome.Code,
			Title: result.Outcome.Title,
		},
		ScoreDetail:    result.ScoreDetail,
		ReportSections: sections,
		Issues:         issues,
		RawReport:      result.RawReport,
	}
}

func containsFold(value, keyword string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(keyword))
}
