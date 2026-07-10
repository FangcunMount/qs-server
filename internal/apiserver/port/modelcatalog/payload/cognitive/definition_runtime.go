package cognitive

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// DefinitionEnvelope carries published metadata while a cognitive Definition is projected to execution DTO.
type DefinitionEnvelope struct {
	Code                 string
	Version              string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
}

// SnapshotFromDefinition builds the cognitive execution DTO without parsing published payload JSON.
func SnapshotFromDefinition(env DefinitionEnvelope, def *definition.Definition) (*Snapshot, error) {
	if def == nil {
		return nil, fmt.Errorf("cognitive definition is nil")
	}
	levels := def.Measure.FactorGraph.Levels()
	scoring := make(map[string]factor.Scoring, len(def.Measure.Scoring))
	for _, item := range def.Measure.Scoring {
		scoring[item.FactorCode] = item
	}
	factors := make([]FactorSnapshot, 0, len(def.Measure.Factors))
	for _, item := range def.Measure.Factors {
		result := FactorSnapshot{Code: item.Code, Title: item.Title, Role: item.Role, ParentCode: def.Measure.FactorGraph.ParentCode(item.Code), SortOrder: def.Measure.FactorGraph.SortOrders[item.Code], Level: levels[item.Code]}
		if ref, ok := normRefByFactor(def.Calibration.NormRefs, item.Code); ok {
			result.Norm = &ref
		}
		if rule, ok := scoring[item.Code]; ok {
			result.ScoringStrategy = string(rule.Strategy)
			result.ScoringParams = cloneScoringParams(rule.Params)
			result.MaxScore = cloneFloat64(rule.MaxScore)
			if hasFactorSources(rule.Sources) {
				result.ChildrenPolicy = &factor.ChildrenPolicy{Strategy: factor.ChildrenAggregationStrategy(rule.Strategy), Children: factorSourceCodes(rule.Sources), Weights: cloneWeights(rule.Weights)}
			} else {
				result.QuestionCodes = questionSourceCodes(rule.Sources)
			}
		}
		factors = append(factors, result)
	}
	return &Snapshot{Code: env.Code, Version: env.Version, Title: env.Title, QuestionnaireCode: env.QuestionnaireCode, QuestionnaireVersion: env.QuestionnaireVersion, Status: env.Status, Factors: factors, AbilityConclusions: AbilityConclusions(def)}, nil
}

func normRefByFactor(refs []norm.Ref, factorCode string) (norm.Ref, bool) {
	for _, ref := range refs {
		if ref.FactorCode == factorCode {
			return ref, true
		}
	}
	return norm.Ref{}, false
}

// AbilityConclusions returns the configured cognitive interpretation rules.
func AbilityConclusions(def *definition.Definition) []conclusion.AbilityConclusion {
	if def == nil {
		return nil
	}
	out := make([]conclusion.AbilityConclusion, 0)
	for _, item := range def.Conclusions {
		if typed, ok := item.(conclusion.AbilityConclusion); ok {
			out = append(out, typed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
