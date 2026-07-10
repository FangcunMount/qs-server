package factor

import (
	"fmt"
	"sort"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// definitionFromFactorDTOs constructs the canonical scale measure and risk
// conclusion layers directly from the legacy editor DTOs. It deliberately does
// not materialize a ScaleSnapshot; payload projection belongs to authoring.
func definitionFromFactorDTOs(dtos []shared.FactorDTO) (*definition.Definition, error) {
	if len(dtos) == 0 {
		return nil, fmt.Errorf("factor list cannot be empty")
	}
	result := &definition.Definition{
		Measure: definition.MeasureSpec{
			Factors: make([]factor.Factor, 0, len(dtos)),
			Scoring: make([]factor.Scoring, 0, len(dtos)),
		},
		Conclusions: make([]conclusion.Conclusion, 0, len(dtos)),
	}
	for _, dto := range dtos {
		if err := validateFactorSnapshotInput(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules, true); err != nil {
			return nil, pkgerrors.WrapC(err, errorCode.ErrInvalidArgument, "验证因子失败")
		}
		role := factor.FactorRoleDimension
		if dto.IsTotalScore {
			role = factor.FactorRoleTotal
		}
		result.Measure.Factors = append(result.Measure.Factors, factor.Factor{Code: dto.Code, Title: dto.Title, Role: role})
		sources := make([]factor.ScoringSource, 0, len(dto.QuestionCodes))
		for _, questionCode := range dto.QuestionCodes {
			sources = append(sources, factor.ScoringSource{Kind: factor.ScoringSourceQuestion, Code: questionCode})
		}
		var params *factor.ScoringParams
		if dto.ScoringParams != nil {
			params = &factor.ScoringParams{CntOptionContents: append([]string(nil), dto.ScoringParams.CntOptionContents...)}
		}
		result.Measure.Scoring = append(result.Measure.Scoring, factor.Scoring{FactorCode: dto.Code, Sources: sources, Strategy: factor.ScoringStrategy(resolvedScoringStrategy(dto.ScoringStrategy)), Params: params, MaxScore: cloneFloat64(dto.MaxScore)})
		rules := make([]conclusion.ScoreRangeOutcome, 0, len(dto.InterpretRules))
		for _, rule := range dto.InterpretRules {
			rules = append(rules, conclusion.ScoreRangeOutcome{MinScore: rule.MinScore, MaxScore: rule.MaxScore, Level: rule.RiskLevel, Summary: rule.Conclusion, Description: rule.Suggestion})
		}
		result.Conclusions = append(result.Conclusions, conclusion.RiskConclusion{FactorCode: dto.Code, Rules: rules})
	}
	if issues := definition.Validate(*result); len(issues) > 0 {
		return nil, fmt.Errorf("definition validation failed: %s", issues[0].Message)
	}
	return result, nil
}

func definitionWithInterpretRules(current *definition.Definition, dtos []shared.UpdateFactorInterpretRulesDTO) (*definition.Definition, error) {
	if current == nil {
		return nil, fmt.Errorf("definition_v2 is required")
	}
	next := *current
	updated := make(map[string]struct{}, len(dtos))
	for _, dto := range dtos {
		if dto.FactorCode == "" {
			return nil, fmt.Errorf("factor code cannot be empty")
		}
		if err := validateInterpretRules(dto.InterpretRules); err != nil {
			return nil, err
		}
		updated[dto.FactorCode] = struct{}{}
	}
	conclusions := make([]conclusion.Conclusion, 0, len(current.Conclusions)+len(dtos))
	for _, item := range current.Conclusions {
		if risk, ok := item.(conclusion.RiskConclusion); ok {
			if _, replace := updated[risk.FactorCode]; replace {
				continue
			}
		}
		conclusions = append(conclusions, item)
	}
	for _, dto := range dtos {
		rules := make([]conclusion.ScoreRangeOutcome, 0, len(dto.InterpretRules))
		for _, rule := range dto.InterpretRules {
			rules = append(rules, conclusion.ScoreRangeOutcome{MinScore: rule.MinScore, MaxScore: rule.MaxScore, Level: rule.RiskLevel, Summary: rule.Conclusion, Description: rule.Suggestion})
		}
		conclusions = append(conclusions, conclusion.RiskConclusion{FactorCode: dto.FactorCode, Rules: rules})
	}
	next.Conclusions = conclusions
	if issues := definition.Validate(next); len(issues) > 0 {
		return nil, fmt.Errorf("definition validation failed: %s", issues[0].Message)
	}
	return &next, nil
}

func toFactorSnapshot(
	code, title, factorType string,
	isTotalScore, _ bool,
	questionCodes []string,
	scoringStrategy string,
	scoringParams *shared.ScoringParamsDTO,
	maxScore *float64,
	interpretRules []shared.InterpretRuleDTO,
) (scalesnapshot.FactorSnapshot, error) {
	if err := validateFactorSnapshotInput(code, title, factorType, isTotalScore, questionCodes, scoringStrategy, scoringParams, maxScore, interpretRules, false); err != nil {
		return scalesnapshot.FactorSnapshot{}, pkgerrors.WrapC(err, errorCode.ErrInvalidArgument, "创建因子失败")
	}
	return scalesnapshot.FactorSnapshot{
		Code:            code,
		Title:           title,
		IsTotalScore:    isTotalScore,
		QuestionCodes:   append([]string(nil), questionCodes...),
		ScoringStrategy: resolvedScoringStrategy(scoringStrategy),
		ScoringParams:   scoringParamsSnapshotFromDTO(scoringParams),
		MaxScore:        cloneFloat64(maxScore),
		InterpretRules:  interpretRuleSnapshotsFromDTOs(interpretRules),
	}, nil
}

func validateFactorSnapshotForReplacement(factor scalesnapshot.FactorSnapshot) error {
	return validateFactorSnapshotInput(
		factor.Code,
		factor.Title,
		"",
		factor.IsTotalScore,
		factor.QuestionCodes,
		factor.ScoringStrategy,
		&shared.ScoringParamsDTO{CntOptionContents: factor.ScoringParams.CntOptionContents},
		factor.MaxScore,
		interpretRuleDTOsFromSnapshots(factor.InterpretRules),
		true,
	)
}

func validateFactorSnapshotInput(
	code, title, factorType string,
	isTotalScore bool,
	questionCodes []string,
	scoringStrategy string,
	scoringParams *shared.ScoringParamsDTO,
	maxScore *float64,
	interpretRules []shared.InterpretRuleDTO,
	requireInterpretRules bool,
) error {
	if code == "" {
		return fmt.Errorf("factor code cannot be empty")
	}
	if title == "" {
		return fmt.Errorf("factor title cannot be empty")
	}
	if !isValidFactorType(factorType) {
		return fmt.Errorf("invalid factor type: %s", factorType)
	}
	strategy := resolvedScoringStrategy(scoringStrategy)
	if !isValidScoringStrategy(strategy) {
		return fmt.Errorf("invalid scoring strategy: %s", strategy)
	}
	if maxScore != nil && *maxScore <= 0 {
		return fmt.Errorf("max score must be greater than 0")
	}
	if strategy == "cnt" && (scoringParams == nil || len(scoringParams.CntOptionContents) == 0) {
		return fmt.Errorf("cnt scoring strategy requires cnt_option_contents")
	}
	if err := validateQuestionCodes(isTotalScore, questionCodes); err != nil {
		return err
	}
	if requireInterpretRules && len(interpretRules) == 0 {
		return fmt.Errorf("factor[%s].interpretRules: 因子必须包含至少一个解读规则", code)
	}
	if err := validateInterpretRules(interpretRules); err != nil {
		return err
	}
	return nil
}

func scoringParamsSnapshotFromDTO(scoringParams *shared.ScoringParamsDTO) scalesnapshot.ScoringParamsSnapshot {
	if scoringParams == nil {
		return scalesnapshot.ScoringParamsSnapshot{}
	}
	return scalesnapshot.ScoringParamsSnapshot{
		CntOptionContents: append([]string(nil), scoringParams.CntOptionContents...),
	}
}

func interpretRuleSnapshotsFromDTOs(dtos []shared.InterpretRuleDTO) []scalesnapshot.InterpretRuleSnapshot {
	rules := interpretRuleSnapshotsFromDTOsInOrder(dtos)
	sort.SliceStable(rules, func(i, j int) bool {
		return rules[i].Min < rules[j].Min
	})
	return rules
}

func interpretRuleSnapshotsFromDTOsInOrder(dtos []shared.InterpretRuleDTO) []scalesnapshot.InterpretRuleSnapshot {
	rules := make([]scalesnapshot.InterpretRuleSnapshot, 0, len(dtos))
	for _, dto := range dtos {
		rules = append(rules, scalesnapshot.InterpretRuleSnapshot{
			Min:        dto.MinScore,
			Max:        dto.MaxScore,
			RiskLevel:  dto.RiskLevel,
			Conclusion: dto.Conclusion,
			Suggestion: dto.Suggestion,
		})
	}
	return rules
}

func interpretRuleDTOsFromSnapshots(rules []scalesnapshot.InterpretRuleSnapshot) []shared.InterpretRuleDTO {
	out := make([]shared.InterpretRuleDTO, 0, len(rules))
	for _, rule := range rules {
		out = append(out, shared.InterpretRuleDTO{
			MinScore:   rule.Min,
			MaxScore:   rule.Max,
			RiskLevel:  rule.RiskLevel,
			Conclusion: rule.Conclusion,
			Suggestion: rule.Suggestion,
		})
	}
	return out
}

func validateQuestionCodes(isTotalScore bool, codes []string) error {
	if !isTotalScore && len(codes) == 0 {
		return fmt.Errorf("non-total-score factor requires question codes")
	}
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if code == "" {
			return fmt.Errorf("question code cannot be empty")
		}
		if _, ok := seen[code]; ok {
			return fmt.Errorf("duplicate question code: %s", code)
		}
		seen[code] = struct{}{}
	}
	return nil
}

func validateInterpretRules(dtos []shared.InterpretRuleDTO) error {
	rules := append([]shared.InterpretRuleDTO(nil), dtos...)
	sort.SliceStable(rules, func(i, j int) bool {
		return rules[i].MinScore < rules[j].MinScore
	})
	for i, rule := range rules {
		if rule.MinScore >= rule.MaxScore || !isValidRiskLevel(rule.RiskLevel) {
			return fmt.Errorf("interpretation rule %d is invalid", i+1)
		}
		if i == 0 {
			continue
		}
		previous := rules[i-1]
		if previous.MaxScore > rule.MinScore {
			return fmt.Errorf(
				"interpretation rules overlap: [%.2f, %.2f) and [%.2f, %.2f)",
				previous.MinScore, previous.MaxScore,
				rule.MinScore, rule.MaxScore,
			)
		}
	}
	return nil
}

func isValidFactorType(raw string) bool {
	switch raw {
	case "", "primary", "first_grade", "multilevel", "second_grade", "multi_level":
		return true
	default:
		return false
	}
}

func resolvedScoringStrategy(raw string) string {
	if raw == "" {
		return "sum"
	}
	return raw
}

func isValidScoringStrategy(strategy string) bool {
	switch strategy {
	case "sum", "avg", "cnt":
		return true
	default:
		return false
	}
}

func isValidRiskLevel(level string) bool {
	switch level {
	case "none", "low", "medium", "high", "severe":
		return true
	default:
		return false
	}
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
