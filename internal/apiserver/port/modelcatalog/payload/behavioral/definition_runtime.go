package behavioral

import (
	"fmt"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	catalognorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/normruntime"
)

// DefinitionEnvelope carries published metadata while a behavioral Definition is projected to execution DTO.
type DefinitionEnvelope struct {
	Code                 string
	Version              string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
}

// SnapshotFromDefinition builds the behavioral execution DTO without parsing published payload JSON.
func SnapshotFromDefinition(env DefinitionEnvelope, def *definition.Definition, normTables map[string]*catalognorm.Norm) (*Snapshot, error) {
	if def == nil {
		return nil, fmt.Errorf("behavioral definition is nil")
	}
	factors := factorSnapshotsFromMeasure(def.Measure)
	out := &Snapshot{
		Code: env.Code, Version: env.Version, Title: env.Title, QuestionnaireCode: env.QuestionnaireCode,
		QuestionnaireVersion: env.QuestionnaireVersion, Status: env.Status, Factors: factors,
	}
	profile, err := normingProfileFromDefinition(def, normTables)
	if err != nil {
		return nil, err
	}
	out.Norming = profile
	return out, nil
}

func factorSnapshotsFromMeasure(measure definition.MeasureSpec) []FactorSnapshot {
	if measure.Factors == nil {
		return nil
	}
	levels := measure.FactorGraph.Levels()
	scoring := make(map[string]factor.Scoring, len(measure.Scoring))
	for _, item := range measure.Scoring {
		scoring[item.FactorCode] = item
	}
	out := make([]FactorSnapshot, 0, len(measure.Factors))
	for _, item := range measure.Factors {
		result := FactorSnapshot{Code: item.Code, Title: item.Title, Role: item.Role, ParentCode: measure.FactorGraph.ParentCode(item.Code), SortOrder: measure.FactorGraph.SortOrders[item.Code], Level: levels[item.Code]}
		if rule, ok := scoring[item.Code]; ok {
			result.ScoringStrategy = string(rule.Strategy)
			result.ScoringParams = cloneScoringParams(rule.Params)
			result.MaxScore = cloneFloat64(rule.MaxScore)
			if hasFactorSources(rule.Sources) {
				children := factorSourceCodes(rule.Sources)
				result.ChildrenPolicy = &factor.ChildrenPolicy{Strategy: factor.ChildrenAggregationStrategy(rule.Strategy), Children: children, Weights: cloneWeights(rule.Weights)}
			} else {
				result.QuestionCodes = questionSourceCodes(rule.Sources)
			}
		}
		out = append(out, result)
	}
	return out
}

func normingProfileFromDefinition(def *definition.Definition, tables map[string]*catalognorm.Norm) (*NormingProfile, error) {
	if def == nil {
		return nil, nil
	}
	conclusions := make([]conclusion.NormConclusion, 0)
	for _, item := range def.Conclusions {
		if typed, ok := item.(conclusion.NormConclusion); ok {
			conclusions = append(conclusions, typed)
		}
	}
	if len(def.Calibration.NormRefs) == 0 && len(conclusions) == 0 {
		return nil, nil
	}
	version := ""
	for _, ref := range def.Calibration.NormRefs {
		if ref.NormTableVersion != "" {
			version = ref.NormTableVersion
			break
		}
	}
	if version == "" {
		return nil, fmt.Errorf("behavioral norm table version is required")
	}
	table := tables[version]
	if table == nil {
		return nil, fmt.Errorf("behavioral norm table %s is not available", version)
	}
	calcTables, err := calcNormTables(table, conclusions)
	if err != nil {
		return nil, err
	}
	primary := ""
	for _, item := range conclusions {
		if item.Primary {
			primary = item.FactorCode
			break
		}
	}
	required := make([]string, 0, len(def.Calibration.NormRefs))
	seenRequired := make(map[string]struct{}, len(def.Calibration.NormRefs))
	for _, ref := range def.Calibration.NormRefs {
		if ref.FactorCode == "" {
			continue
		}
		if _, exists := seenRequired[ref.FactorCode]; exists {
			continue
		}
		seenRequired[ref.FactorCode] = struct{}{}
		required = append(required, ref.FactorCode)
	}
	return &NormingProfile{Variant: table.FormVariant, NormTableVersion: version, PrimaryDimensionCode: primary, RequiredFactorCodes: required, NormTables: calcTables}, nil
}

func calcNormTables(table *catalognorm.Norm, conclusions []conclusion.NormConclusion) (*calcnorm.NormTables, error) {
	out, err := normruntime.FromCatalog(table)
	if err != nil {
		return nil, err
	}
	out.TScoreRules = make([]calcnorm.TScoreInterpretRule, 0, len(conclusions))
	for _, item := range conclusions {
		if item.ScoreBasis != conclusion.ScoreBasisTScore {
			continue
		}
		rule := calcnorm.TScoreInterpretRule{FactorCode: item.FactorCode, Ranges: make([]calcnorm.TScoreRange, 0, len(item.Rules))}
		for _, item := range item.Rules {
			rule.Ranges = append(rule.Ranges, calcnorm.TScoreRange{
				MinT: item.MinScore, MaxT: item.MaxScore, MaxInclusive: item.MaxInclusive, UnboundedMax: item.UnboundedMax,
				// TScoreRange.Level is retained for the runtime/ReportInput JSON
				// contract, but current-only execution stores canonical OutcomeCode.
				Level: item.OutcomeCode, Conclusion: item.Summary, Suggestion: item.Description,
			})
		}
		out.TScoreRules = append(out.TScoreRules, rule)
	}
	return out, nil
}

func hasFactorSources(items []factor.ScoringSource) bool {
	for _, item := range items {
		if item.Kind == factor.ScoringSourceFactor {
			return true
		}
	}
	return false
}
func factorSourceCodes(items []factor.ScoringSource) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.Kind == factor.ScoringSourceFactor {
			out = append(out, item.Code)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
func questionSourceCodes(items []factor.ScoringSource) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.Kind == factor.ScoringSourceQuestion {
			out = append(out, item.Code)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
