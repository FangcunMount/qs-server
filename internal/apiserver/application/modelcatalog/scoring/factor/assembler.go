package factor

import (
	"fmt"
	"sort"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

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
