package modelcatalog

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// DefinitionPO is the BSON shape for the target ModelCatalog definition.
type DefinitionPO struct {
	Measure     MeasureSpecPO  `bson:"measure,omitempty"`
	Calibration CalibrationPO  `bson:"calibration,omitempty"`
	Conclusions []ConclusionPO `bson:"conclusions,omitempty"`
	Outcomes    []OutcomePO    `bson:"outcomes,omitempty"`
	ReportMap   ReportMapPO    `bson:"report_map,omitempty"`
}

type MeasureSpecPO struct {
	Factors     []FactorPO    `bson:"factors,omitempty"`
	FactorGraph FactorGraphPO `bson:"factor_graph,omitempty"`
	Scoring     []ScoringPO   `bson:"scoring,omitempty"`
}

type FactorPO struct {
	Code  string `bson:"code"`
	Title string `bson:"title,omitempty"`
	Role  string `bson:"role,omitempty"`
}

type FactorGraphPO struct {
	Roots      []string       `bson:"roots,omitempty"`
	Edges      []FactorEdgePO `bson:"edges,omitempty"`
	SortOrders map[string]int `bson:"sort_orders,omitempty"`
}

type FactorEdgePO struct {
	ParentCode string `bson:"parent_code"`
	ChildCode  string `bson:"child_code"`
}

type ScoringPO struct {
	FactorCode string             `bson:"factor_code"`
	Sources    []ScoringSourcePO  `bson:"sources,omitempty"`
	Strategy   string             `bson:"strategy,omitempty"`
	Params     *ScoringParamsPO   `bson:"params,omitempty"`
	MaxScore   *float64           `bson:"max_score,omitempty"`
	Weights    map[string]float64 `bson:"weights,omitempty"`
}

type ScoringSourcePO struct {
	Kind string `bson:"kind"`
	Code string `bson:"code"`
}

type ScoringParamsPO struct {
	CntOptionContents []string `bson:"cnt_option_contents,omitempty"`
}

type CalibrationPO struct {
	NormRefs []NormRefPO `bson:"norm_refs,omitempty"`
}

type NormRefPO struct {
	FactorCode       string `bson:"factor_code"`
	NormTableVersion string `bson:"norm_table_version"`
}

type ConclusionPO struct {
	Kind        string                `bson:"kind"`
	FactorCode  string                `bson:"factor_code,omitempty"`
	FactorCodes []string              `bson:"factor_codes,omitempty"`
	Rules       []ScoreRangeOutcomePO `bson:"rules,omitempty"`
	Outcomes    []OutcomePO           `bson:"outcomes,omitempty"`
}

type OutcomePO struct {
	Code        string `bson:"code"`
	Title       string `bson:"title,omitempty"`
	Summary     string `bson:"summary,omitempty"`
	Description string `bson:"description,omitempty"`
}

type ScoreRangeOutcomePO struct {
	MinScore    float64 `bson:"min_score"`
	MaxScore    float64 `bson:"max_score"`
	OutcomeCode string  `bson:"outcome_code,omitempty"`
	Title       string  `bson:"title,omitempty"`
	Summary     string  `bson:"summary,omitempty"`
	Description string  `bson:"description,omitempty"`
}

type ReportMapPO struct {
	Sections []ReportSectionPO `bson:"sections,omitempty"`
}

type ReportSectionPO struct {
	Code       string   `bson:"code"`
	Title      string   `bson:"title,omitempty"`
	SourceRefs []string `bson:"source_refs,omitempty"`
}

func definitionToPO(def *domain.Definition) *DefinitionPO {
	if def == nil {
		return nil
	}
	return &DefinitionPO{
		Measure:     measureSpecToPO(def.Measure),
		Calibration: calibrationToPO(def.Calibration),
		Conclusions: conclusionsToPO(def.Conclusions),
		Outcomes:    outcomesToPO(def.Outcomes),
		ReportMap:   reportMapToPO(def.ReportMap),
	}
}

func definitionSchemaVersion(def *domain.Definition) string {
	if def == nil {
		return ""
	}
	return domain.SchemaVersionV2
}

func definitionFromPO(po *DefinitionPO) *domain.Definition {
	if po == nil {
		return nil
	}
	return &domain.Definition{
		Measure:     measureSpecFromPO(po.Measure),
		Calibration: calibrationFromPO(po.Calibration),
		Conclusions: conclusionsFromPO(po.Conclusions),
		Outcomes:    outcomesFromPO(po.Outcomes),
		ReportMap:   reportMapFromPO(po.ReportMap),
	}
}

func measureSpecToPO(measure domain.MeasureSpec) MeasureSpecPO {
	return MeasureSpecPO{
		Factors:     factorsToPO(measure.Factors),
		FactorGraph: factorGraphToPO(measure.FactorGraph),
		Scoring:     scoringToPO(measure.Scoring),
	}
}

func measureSpecFromPO(po MeasureSpecPO) domain.MeasureSpec {
	return domain.MeasureSpec{
		Factors:     factorsFromPO(po.Factors),
		FactorGraph: factorGraphFromPO(po.FactorGraph),
		Scoring:     scoringFromPO(po.Scoring),
	}
}

func factorsToPO(factors []domain.Factor) []FactorPO {
	if factors == nil {
		return nil
	}
	out := make([]FactorPO, 0, len(factors))
	for _, item := range factors {
		out = append(out, FactorPO{Code: item.Code, Title: item.Title, Role: string(item.Role)})
	}
	return out
}

func factorsFromPO(items []FactorPO) []domain.Factor {
	if items == nil {
		return nil
	}
	out := make([]domain.Factor, 0, len(items))
	for _, item := range items {
		out = append(out, domain.Factor{Code: item.Code, Title: item.Title, Role: domain.FactorRole(item.Role)})
	}
	return out
}

func factorGraphToPO(graph factor.FactorGraph) FactorGraphPO {
	edges := make([]FactorEdgePO, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		edges = append(edges, FactorEdgePO{ParentCode: edge.ParentCode, ChildCode: edge.ChildCode})
	}
	return FactorGraphPO{
		Roots:      append([]string(nil), graph.Roots...),
		Edges:      edges,
		SortOrders: cloneIntMap(graph.SortOrders),
	}
}

func factorGraphFromPO(po FactorGraphPO) factor.FactorGraph {
	edges := make([]factor.FactorEdge, 0, len(po.Edges))
	for _, edge := range po.Edges {
		edges = append(edges, factor.FactorEdge{ParentCode: edge.ParentCode, ChildCode: edge.ChildCode})
	}
	return factor.FactorGraph{
		Roots:      append([]string(nil), po.Roots...),
		Edges:      edges,
		SortOrders: cloneIntMap(po.SortOrders),
	}
}

func scoringToPO(scoring []factor.Scoring) []ScoringPO {
	if scoring == nil {
		return nil
	}
	out := make([]ScoringPO, 0, len(scoring))
	for _, item := range scoring {
		out = append(out, ScoringPO{
			FactorCode: item.FactorCode,
			Sources:    scoringSourcesToPO(item.Sources),
			Strategy:   item.Strategy.String(),
			Params:     scoringParamsToPO(item.Params),
			MaxScore:   cloneFloat64(item.MaxScore),
			Weights:    cloneFloat64Map(item.Weights),
		})
	}
	return out
}

func scoringFromPO(items []ScoringPO) []factor.Scoring {
	if items == nil {
		return nil
	}
	out := make([]factor.Scoring, 0, len(items))
	for _, item := range items {
		out = append(out, factor.Scoring{
			FactorCode: item.FactorCode,
			Sources:    scoringSourcesFromPO(item.Sources),
			Strategy:   factor.ScoringStrategy(item.Strategy),
			Params:     scoringParamsFromPO(item.Params),
			MaxScore:   cloneFloat64(item.MaxScore),
			Weights:    cloneFloat64Map(item.Weights),
		})
	}
	return out
}

func scoringSourcesToPO(sources []factor.ScoringSource) []ScoringSourcePO {
	if sources == nil {
		return nil
	}
	out := make([]ScoringSourcePO, 0, len(sources))
	for _, source := range sources {
		out = append(out, ScoringSourcePO{Kind: string(source.Kind), Code: source.Code})
	}
	return out
}

func scoringSourcesFromPO(items []ScoringSourcePO) []factor.ScoringSource {
	if items == nil {
		return nil
	}
	out := make([]factor.ScoringSource, 0, len(items))
	for _, item := range items {
		out = append(out, factor.ScoringSource{Kind: factor.ScoringSourceKind(item.Kind), Code: item.Code})
	}
	return out
}

func scoringParamsToPO(params *factor.ScoringParams) *ScoringParamsPO {
	if params == nil {
		return nil
	}
	return &ScoringParamsPO{CntOptionContents: append([]string(nil), params.CntOptionContents...)}
}

func scoringParamsFromPO(po *ScoringParamsPO) *factor.ScoringParams {
	if po == nil {
		return nil
	}
	return &factor.ScoringParams{CntOptionContents: append([]string(nil), po.CntOptionContents...)}
}

func calibrationToPO(calibration domain.Calibration) CalibrationPO {
	refs := make([]NormRefPO, 0, len(calibration.NormRefs))
	for _, ref := range calibration.NormRefs {
		refs = append(refs, NormRefPO{FactorCode: ref.FactorCode, NormTableVersion: ref.NormTableVersion})
	}
	return CalibrationPO{NormRefs: refs}
}

func calibrationFromPO(po CalibrationPO) domain.Calibration {
	refs := make([]domain.NormRef, 0, len(po.NormRefs))
	for _, ref := range po.NormRefs {
		refs = append(refs, domain.NormRef{FactorCode: ref.FactorCode, NormTableVersion: ref.NormTableVersion})
	}
	return domain.Calibration{NormRefs: refs}
}

func conclusionsToPO(conclusions []domain.Conclusion) []ConclusionPO {
	if conclusions == nil {
		return nil
	}
	out := make([]ConclusionPO, 0, len(conclusions))
	for _, item := range conclusions {
		switch typed := item.(type) {
		case domain.RiskConclusion:
			out = append(out, ConclusionPO{
				Kind:       string(conclusion.KindRisk),
				FactorCode: typed.FactorCode,
				Rules:      scoreRangeOutcomesToPO(typed.Rules),
				Outcomes:   outcomesToPO(typed.Outcomes),
			})
		case domain.TypeConclusion:
			out = append(out, ConclusionPO{Kind: string(conclusion.KindType), FactorCodes: append([]string(nil), typed.FactorCodes...), Outcomes: outcomesToPO(typed.Outcomes)})
		case domain.NormConclusion:
			out = append(out, ConclusionPO{Kind: string(conclusion.KindNorm), FactorCode: typed.FactorCode, Outcomes: outcomesToPO(typed.Outcomes)})
		case domain.AbilityConclusion:
			out = append(out, ConclusionPO{Kind: string(conclusion.KindAbility), FactorCode: typed.FactorCode, Outcomes: outcomesToPO(typed.Outcomes)})
		}
	}
	return out
}

func conclusionsFromPO(items []ConclusionPO) []domain.Conclusion {
	if items == nil {
		return nil
	}
	out := make([]domain.Conclusion, 0, len(items))
	for _, item := range items {
		switch conclusion.Kind(item.Kind) {
		case conclusion.KindRisk:
			out = append(out, domain.RiskConclusion{
				FactorCode: item.FactorCode,
				Rules:      scoreRangeOutcomesFromPO(item.Rules),
				Outcomes:   outcomesFromPO(item.Outcomes),
			})
		case conclusion.KindType:
			out = append(out, domain.TypeConclusion{FactorCodes: append([]string(nil), item.FactorCodes...), Outcomes: outcomesFromPO(item.Outcomes)})
		case conclusion.KindNorm:
			out = append(out, domain.NormConclusion{FactorCode: item.FactorCode, Outcomes: outcomesFromPO(item.Outcomes)})
		case conclusion.KindAbility:
			out = append(out, domain.AbilityConclusion{FactorCode: item.FactorCode, Outcomes: outcomesFromPO(item.Outcomes)})
		}
	}
	return out
}

func outcomesToPO(outcomes []domain.Outcome) []OutcomePO {
	if outcomes == nil {
		return nil
	}
	out := make([]OutcomePO, 0, len(outcomes))
	for _, item := range outcomes {
		out = append(out, OutcomePO{
			Code:        item.Code,
			Title:       item.Title,
			Summary:     item.Summary,
			Description: item.Description,
		})
	}
	return out
}

func outcomesFromPO(items []OutcomePO) []domain.Outcome {
	if items == nil {
		return nil
	}
	out := make([]domain.Outcome, 0, len(items))
	for _, item := range items {
		out = append(out, domain.Outcome{
			Code:        item.Code,
			Title:       item.Title,
			Summary:     item.Summary,
			Description: item.Description,
		})
	}
	return out
}

func scoreRangeOutcomesToPO(rules []conclusion.ScoreRangeOutcome) []ScoreRangeOutcomePO {
	if rules == nil {
		return nil
	}
	out := make([]ScoreRangeOutcomePO, 0, len(rules))
	for _, item := range rules {
		out = append(out, ScoreRangeOutcomePO{
			MinScore:    item.MinScore,
			MaxScore:    item.MaxScore,
			OutcomeCode: item.OutcomeCode,
			Title:       item.Title,
			Summary:     item.Summary,
			Description: item.Description,
		})
	}
	return out
}

func scoreRangeOutcomesFromPO(items []ScoreRangeOutcomePO) []conclusion.ScoreRangeOutcome {
	if items == nil {
		return nil
	}
	out := make([]conclusion.ScoreRangeOutcome, 0, len(items))
	for _, item := range items {
		out = append(out, conclusion.ScoreRangeOutcome{
			MinScore:    item.MinScore,
			MaxScore:    item.MaxScore,
			OutcomeCode: item.OutcomeCode,
			Title:       item.Title,
			Summary:     item.Summary,
			Description: item.Description,
		})
	}
	return out
}

func reportMapToPO(reportMap domain.ReportMap) ReportMapPO {
	sections := make([]ReportSectionPO, 0, len(reportMap.Sections))
	for _, section := range reportMap.Sections {
		sections = append(sections, ReportSectionPO{
			Code:       section.Code,
			Title:      section.Title,
			SourceRefs: append([]string(nil), section.SourceRefs...),
		})
	}
	return ReportMapPO{Sections: sections}
}

func reportMapFromPO(po ReportMapPO) domain.ReportMap {
	sections := make([]domain.ReportSection, 0, len(po.Sections))
	for _, section := range po.Sections {
		sections = append(sections, domain.ReportSection{
			Code:       section.Code,
			Title:      section.Title,
			SourceRefs: append([]string(nil), section.SourceRefs...),
		})
	}
	return domain.ReportMap{Sections: sections}
}

func cloneIntMap(values map[string]int) map[string]int {
	if values == nil {
		return nil
	}
	out := make(map[string]int, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneFloat64Map(values map[string]float64) map[string]float64 {
	if values == nil {
		return nil
	}
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
