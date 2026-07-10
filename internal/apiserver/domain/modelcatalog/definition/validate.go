package definition

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
)

// ValidationIssue reports one Definition-level invariant violation.
type ValidationIssue struct {
	Field   string
	Code    string
	Message string
}

// Validate checks references and semantic configuration across all Definition layers.
func Validate(def Definition) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	for _, issue := range ValidateMeasureSpec(def.Measure) {
		issues = append(issues, ValidationIssue{Field: "measure." + issue.Field, Code: issue.Code, Message: issue.Message})
	}
	factorCodes := makeStringSet()
	for _, item := range def.Measure.Factors {
		if item.Code != "" {
			factorCodes[item.Code] = struct{}{}
		}
	}
	issues = append(issues, validateCalibration(def.Calibration, factorCodes)...)
	outcomeCodes, outcomeIssues := validateOutcomes(def.Outcomes)
	issues = append(issues, outcomeIssues...)
	issues = append(issues, validateConclusions(def.Conclusions, factorCodes, outcomeCodes)...)
	issues = append(issues, validateReportMap(def.ReportMap)...)
	return issues
}

func validateCalibration(calibration Calibration, factorCodes map[string]struct{}) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	seen := makeStringSet()
	for _, ref := range calibration.NormRefs {
		key := ref.FactorCode + "@" + ref.NormTableVersion
		if ref.FactorCode == "" {
			issues = append(issues, ValidationIssue{Field: "calibration.norm_refs", Code: "norm_ref.factor.required", Message: "norm ref factor_code is required"})
		}
		if ref.NormTableVersion == "" {
			issues = append(issues, ValidationIssue{Field: "calibration.norm_refs", Code: "norm_ref.version.required", Message: "norm ref table version is required"})
		}
		if _, ok := factorCodes[ref.FactorCode]; ref.FactorCode != "" && !ok {
			issues = append(issues, ValidationIssue{Field: "calibration.norm_refs", Code: "norm_ref.factor.not_found", Message: fmt.Sprintf("norm ref factor %s is not defined", ref.FactorCode)})
		}
		if _, ok := seen[key]; key != "@" && ok {
			issues = append(issues, ValidationIssue{Field: "calibration.norm_refs", Code: "norm_ref.duplicate", Message: fmt.Sprintf("norm ref %s is duplicated", key)})
		}
		seen[key] = struct{}{}
	}
	return issues
}

func validateOutcomes(outcomes []conclusion.Outcome) (map[string]struct{}, []ValidationIssue) {
	codes := makeStringSet()
	issues := make([]ValidationIssue, 0)
	for _, item := range outcomes {
		if item.Code == "" {
			issues = append(issues, ValidationIssue{Field: "outcomes", Code: "outcome.code.required", Message: "outcome code is required"})
			continue
		}
		if _, ok := codes[item.Code]; ok {
			issues = append(issues, ValidationIssue{Field: "outcomes", Code: "outcome.code.duplicate", Message: fmt.Sprintf("outcome code %s is duplicated", item.Code)})
		}
		codes[item.Code] = struct{}{}
	}
	return codes, issues
}

func validateConclusions(items []conclusion.Conclusion, factorCodes, outcomeCodes map[string]struct{}) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	for _, item := range items {
		switch typed := item.(type) {
		case conclusion.RiskConclusion:
			issues = append(issues, validateFactorConclusion("risk", typed.FactorCode, "", typed.Rules, factorCodes, outcomeCodes)...)
		case conclusion.NormConclusion:
			issues = append(issues, validateFactorConclusion("norm", typed.FactorCode, typed.ScoreBasis, typed.Rules, factorCodes, outcomeCodes)...)
		case conclusion.AbilityConclusion:
			issues = append(issues, validateFactorConclusion("ability", typed.FactorCode, typed.ScoreBasis, typed.Rules, factorCodes, outcomeCodes)...)
		case conclusion.TypeConclusion:
			issues = append(issues, validateTypeConclusion(typed, factorCodes, outcomeCodes)...)
		default:
			issues = append(issues, ValidationIssue{Field: "conclusions", Code: "conclusion.kind.unsupported", Message: fmt.Sprintf("unsupported conclusion type %T", item)})
		}
	}
	return issues
}

func validateFactorConclusion(kind, factorCode string, basis conclusion.ScoreBasis, rules []conclusion.ScoreRangeOutcome, factorCodes, outcomeCodes map[string]struct{}) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	prefix := "conclusions." + kind
	if _, ok := factorCodes[factorCode]; factorCode == "" || !ok {
		issues = append(issues, ValidationIssue{Field: prefix + ".factor_code", Code: "conclusion.factor.not_found", Message: fmt.Sprintf("conclusion factor %s is not defined", factorCode)})
	}
	if kind != "risk" && !validScoreBasis(basis) {
		issues = append(issues, ValidationIssue{Field: prefix + ".score_basis", Code: "conclusion.score_basis.invalid", Message: "conclusion score basis is invalid"})
	}
	for _, rule := range rules {
		if rule.MinScore > rule.MaxScore {
			issues = append(issues, ValidationIssue{Field: prefix + ".rules", Code: "conclusion.range.invalid", Message: "conclusion range min_score must not exceed max_score"})
		}
		if rule.OutcomeCode != "" {
			if _, ok := outcomeCodes[rule.OutcomeCode]; !ok {
				issues = append(issues, ValidationIssue{Field: prefix + ".rules", Code: "conclusion.outcome.not_found", Message: fmt.Sprintf("outcome %s is not defined", rule.OutcomeCode)})
			}
		}
	}
	return issues
}

func validateTypeConclusion(item conclusion.TypeConclusion, factorCodes, outcomeCodes map[string]struct{}) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	for _, code := range item.FactorCodes {
		if _, ok := factorCodes[code]; code == "" || !ok {
			issues = append(issues, ValidationIssue{Field: "conclusions.type.factor_codes", Code: "conclusion.factor.not_found", Message: fmt.Sprintf("conclusion factor %s is not defined", code)})
		}
	}
	if !validTypeDecision(item.Decision.Kind) {
		issues = append(issues, ValidationIssue{Field: "conclusions.type.decision.kind", Code: "type_conclusion.decision.invalid", Message: "type conclusion decision kind is invalid"})
	}
	for _, pole := range item.Decision.Poles {
		if _, ok := factorCodes[pole.FactorCode]; pole.FactorCode == "" || !ok {
			issues = append(issues, ValidationIssue{Field: "conclusions.type.decision.poles", Code: "conclusion.factor.not_found", Message: fmt.Sprintf("pole factor %s is not defined", pole.FactorCode)})
		}
	}
	for _, rule := range item.SpecialRules {
		if rule.Code == "" || rule.Kind == "" || rule.Phase == "" {
			issues = append(issues, ValidationIssue{Field: "conclusions.type.special_rules", Code: "type_conclusion.special_rule.invalid", Message: "type special rule code, kind and phase are required"})
		}
		if rule.OutcomeCode != "" {
			if _, ok := outcomeCodes[rule.OutcomeCode]; !ok {
				issues = append(issues, ValidationIssue{Field: "conclusions.type.special_rules", Code: "conclusion.outcome.not_found", Message: fmt.Sprintf("outcome %s is not defined", rule.OutcomeCode)})
			}
		}
	}
	for _, profile := range item.Profiles {
		if _, ok := outcomeCodes[profile.OutcomeCode]; profile.OutcomeCode == "" || !ok {
			issues = append(issues, ValidationIssue{Field: "conclusions.type.profiles", Code: "conclusion.outcome.not_found", Message: fmt.Sprintf("outcome %s is not defined", profile.OutcomeCode)})
		}
	}
	return issues
}

func validateReportMap(reportMap ReportMap) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	seen := makeStringSet()
	for _, section := range reportMap.Sections {
		if section.Code == "" {
			issues = append(issues, ValidationIssue{Field: "report_map.sections", Code: "report_section.code.required", Message: "report section code is required"})
			continue
		}
		if _, ok := seen[section.Code]; ok {
			issues = append(issues, ValidationIssue{Field: "report_map.sections", Code: "report_section.code.duplicate", Message: fmt.Sprintf("report section %s is duplicated", section.Code)})
		}
		seen[section.Code] = struct{}{}
	}
	return issues
}

func validScoreBasis(value conclusion.ScoreBasis) bool {
	switch value {
	case conclusion.ScoreBasisRaw, conclusion.ScoreBasisTScore, conclusion.ScoreBasisPercentile:
		return true
	default:
		return false
	}
}

func validTypeDecision(kind binding.DecisionKind) bool {
	switch kind {
	case binding.DecisionKindPoleComposition, binding.DecisionKindTraitProfile, binding.DecisionKindNearestPattern:
		return true
	default:
		return false
	}
}

func makeStringSet() map[string]struct{} { return make(map[string]struct{}) }
