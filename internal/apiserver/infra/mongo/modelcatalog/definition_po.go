package modelcatalog

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// DefinitionPO is the BSON shape for the target ModelCatalog definition.
type DefinitionPO struct {
	Measure     MeasureSpecPO   `bson:"measure,omitempty"`
	Calibration CalibrationPO   `bson:"calibration,omitempty"`
	Execution   ExecutionSpecPO `bson:"execution,omitempty"`
	Conclusions []ConclusionPO  `bson:"conclusions,omitempty"`
	Outcomes    []OutcomePO     `bson:"outcomes,omitempty"`
	ReportMap   ReportMapPO     `bson:"report_map,omitempty"`
}

type ExecutionSpecPO struct {
	Brief2 *Brief2SpecPO `bson:"brief2,omitempty"`
	SPM    *SPMSpecPO    `bson:"spm,omitempty"`
}

type Brief2SpecPO struct {
	FormVariant         string   `bson:"form_variant,omitempty"`
	PrimaryFactorCode   string   `bson:"primary_factor_code,omitempty"`
	IndexFactorCodes    []string `bson:"index_factor_codes,omitempty"`
	ValidityFactorCodes []string `bson:"validity_factor_codes,omitempty"`
}

type SPMSpecPO struct {
	TimeLimitSeconds int            `bson:"time_limit_seconds,omitempty"`
	TotalFactorCode  string         `bson:"total_factor_code,omitempty"`
	ItemSets         []SPMItemSetPO `bson:"item_sets,omitempty"`
}

type SPMItemSetPO struct {
	Code  string      `bson:"code,omitempty"`
	Items []SPMItemPO `bson:"items,omitempty"`
}

type SPMItemPO struct {
	QuestionCode      string `bson:"question_code,omitempty"`
	CorrectOptionCode string `bson:"correct_option_code,omitempty"`
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
	FactorCode    string             `bson:"factor_code"`
	Sources       []ScoringSourcePO  `bson:"sources,omitempty"`
	Strategy      string             `bson:"strategy,omitempty"`
	Params        *ScoringParamsPO   `bson:"params,omitempty"`
	MaxScore      *float64           `bson:"max_score,omitempty"`
	Weights       map[string]float64 `bson:"weights,omitempty"`
	Constant      float64            `bson:"constant,omitempty"`
	OptionScoring string             `bson:"option_scoring,omitempty"`
}

type ScoringSourcePO struct {
	Kind         string             `bson:"kind"`
	Code         string             `bson:"code"`
	ScoringMode  string             `bson:"scoring_mode,omitempty"`
	Sign         float64            `bson:"sign,omitempty"`
	Weight       float64            `bson:"weight,omitempty"`
	OptionScores map[string]float64 `bson:"option_scores,omitempty"`
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
	Kind           string                 `bson:"kind"`
	FactorCode     string                 `bson:"factor_code,omitempty"`
	FactorCodes    []string               `bson:"factor_codes,omitempty"`
	ScoreBasis     string                 `bson:"score_basis,omitempty"`
	Primary        bool                   `bson:"primary,omitempty"`
	Rules          []ScoreRangeOutcomePO  `bson:"rules,omitempty"`
	Outcomes       []OutcomePO            `bson:"outcomes,omitempty"`
	TypeDecision   *TypeDecisionPO        `bson:"type_decision,omitempty"`
	SpecialRules   []TypeSpecialRulePO    `bson:"special_rules,omitempty"`
	OutcomeMapping *TypeOutcomeMappingPO  `bson:"outcome_mapping,omitempty"`
	Profiles       []TypeOutcomeProfilePO `bson:"profiles,omitempty"`
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
	Level       string  `bson:"level,omitempty"`
	OutcomeCode string  `bson:"outcome_code,omitempty"`
	Title       string  `bson:"title,omitempty"`
	Summary     string  `bson:"summary,omitempty"`
	Description string  `bson:"description,omitempty"`
}

type TypeDecisionPO struct {
	Kind                        string           `bson:"kind,omitempty"`
	FallbackSimilarityThreshold float64          `bson:"fallback_similarity_threshold,omitempty"`
	FallbackCode                string           `bson:"fallback_code,omitempty"`
	LevelRule                   *TypeLevelRulePO `bson:"level_rule,omitempty"`
	Poles                       []TypePolePO     `bson:"poles,omitempty"`
}

type TypeLevelRulePO struct {
	LowMax  float64 `bson:"low_max,omitempty"`
	HighMin float64 `bson:"high_min,omitempty"`
}

type TypePolePO struct {
	FactorCode string  `bson:"factor_code"`
	LeftPole   string  `bson:"left_pole,omitempty"`
	RightPole  string  `bson:"right_pole,omitempty"`
	Threshold  float64 `bson:"threshold,omitempty"`
	Model      string  `bson:"model,omitempty"`
}

type TypeSpecialRulePO struct {
	Code          string   `bson:"code"`
	Kind          string   `bson:"kind,omitempty"`
	Phase         string   `bson:"phase,omitempty"`
	Trigger       string   `bson:"trigger,omitempty"`
	OutcomeCode   string   `bson:"outcome_code,omitempty"`
	QuestionCodes []string `bson:"question_codes,omitempty"`
	OptionValues  []string `bson:"option_values,omitempty"`
}

type TypeOutcomeMappingPO struct {
	DetailKind       string `bson:"detail_kind,omitempty"`
	DetailAdapterKey string `bson:"detail_adapter_key,omitempty"`
	Algorithm        string `bson:"algorithm,omitempty"`
}

type TypeOutcomeProfilePO struct {
	OutcomeCode string   `bson:"outcome_code"`
	Pattern     string   `bson:"pattern,omitempty"`
	Traits      []string `bson:"traits,omitempty"`
	Strengths   []string `bson:"strengths,omitempty"`
	Weaknesses  []string `bson:"weaknesses,omitempty"`
	Suggestions []string `bson:"suggestions,omitempty"`
	ImageURL    string   `bson:"image_url,omitempty"`
	Image       string   `bson:"image,omitempty"`
	Rarity      RarityPO `bson:"rarity,omitempty"`
	IsSpecial   bool     `bson:"is_special,omitempty"`
	Trigger     string   `bson:"trigger,omitempty"`
	Commentary  string   `bson:"commentary,omitempty"`
}

type RarityPO struct {
	Percent float64 `bson:"percent,omitempty"`
	Label   string  `bson:"label,omitempty"`
	OneInX  int     `bson:"one_in_x,omitempty"`
}

type ReportMapPO struct {
	Sections []ReportSectionPO `bson:"sections,omitempty"`
}

type ReportSectionPO struct {
	Code          string   `bson:"code"`
	Title         string   `bson:"title,omitempty"`
	SourceRefs    []string `bson:"source_refs,omitempty"`
	Kind          string   `bson:"kind,omitempty"`
	AdapterKey    string   `bson:"adapter_key,omitempty"`
	TemplateID    string   `bson:"template_id,omitempty"`
	CategoryLabel string   `bson:"category_label,omitempty"`
}

func definitionToPO(def *domain.Definition) *DefinitionPO {
	if def == nil {
		return nil
	}
	return &DefinitionPO{
		Measure:     measureSpecToPO(def.Measure),
		Calibration: calibrationToPO(def.Calibration),
		Execution:   executionSpecToPO(def.Execution),
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
		Execution:   executionSpecFromPO(po.Execution),
		Conclusions: conclusionsFromPO(po.Conclusions),
		Outcomes:    outcomesFromPO(po.Outcomes),
		ReportMap:   reportMapFromPO(po.ReportMap),
	}
}

func executionSpecToPO(spec domain.ExecutionSpec) ExecutionSpecPO {
	out := ExecutionSpecPO{}
	if spec.Brief2 != nil {
		out.Brief2 = &Brief2SpecPO{FormVariant: spec.Brief2.FormVariant, PrimaryFactorCode: spec.Brief2.PrimaryFactorCode, IndexFactorCodes: append([]string(nil), spec.Brief2.IndexFactorCodes...), ValidityFactorCodes: append([]string(nil), spec.Brief2.ValidityFactorCodes...)}
	}
	if spec.SPM != nil {
		spm := &SPMSpecPO{TimeLimitSeconds: spec.SPM.TimeLimitSeconds, TotalFactorCode: spec.SPM.TotalFactorCode, ItemSets: make([]SPMItemSetPO, 0, len(spec.SPM.ItemSets))}
		for _, set := range spec.SPM.ItemSets {
			items := make([]SPMItemPO, 0, len(set.Items))
			for _, item := range set.Items {
				items = append(items, SPMItemPO{QuestionCode: item.QuestionCode, CorrectOptionCode: item.CorrectOptionCode})
			}
			spm.ItemSets = append(spm.ItemSets, SPMItemSetPO{Code: set.Code, Items: items})
		}
		out.SPM = spm
	}
	return out
}

func executionSpecFromPO(po ExecutionSpecPO) domain.ExecutionSpec {
	out := domain.ExecutionSpec{}
	if po.Brief2 != nil {
		out.Brief2 = &domain.Brief2Spec{FormVariant: po.Brief2.FormVariant, PrimaryFactorCode: po.Brief2.PrimaryFactorCode, IndexFactorCodes: append([]string(nil), po.Brief2.IndexFactorCodes...), ValidityFactorCodes: append([]string(nil), po.Brief2.ValidityFactorCodes...)}
	}
	if po.SPM != nil {
		spm := &domain.SPMSpec{TimeLimitSeconds: po.SPM.TimeLimitSeconds, TotalFactorCode: po.SPM.TotalFactorCode, ItemSets: make([]domain.SPMItemSet, 0, len(po.SPM.ItemSets))}
		for _, set := range po.SPM.ItemSets {
			items := make([]domain.SPMItem, 0, len(set.Items))
			for _, item := range set.Items {
				items = append(items, domain.SPMItem{QuestionCode: item.QuestionCode, CorrectOptionCode: item.CorrectOptionCode})
			}
			spm.ItemSets = append(spm.ItemSets, domain.SPMItemSet{Code: set.Code, Items: items})
		}
		out.SPM = spm
	}
	return out
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
			FactorCode:    item.FactorCode,
			Sources:       scoringSourcesToPO(item.Sources),
			Strategy:      item.Strategy.String(),
			Params:        scoringParamsToPO(item.Params),
			MaxScore:      cloneFloat64(item.MaxScore),
			Weights:       cloneFloat64Map(item.Weights),
			Constant:      item.Constant,
			OptionScoring: string(item.OptionScoring),
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
			FactorCode:    item.FactorCode,
			Sources:       scoringSourcesFromPO(item.Sources),
			Strategy:      factor.ScoringStrategy(item.Strategy),
			Params:        scoringParamsFromPO(item.Params),
			MaxScore:      cloneFloat64(item.MaxScore),
			Weights:       cloneFloat64Map(item.Weights),
			Constant:      item.Constant,
			OptionScoring: factor.OptionScoring(item.OptionScoring),
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
		out = append(out, ScoringSourcePO{Kind: string(source.Kind), Code: source.Code, ScoringMode: string(source.ScoringMode), Sign: source.Sign, Weight: source.Weight, OptionScores: cloneFloat64Map(source.OptionScores)})
	}
	return out
}

func scoringSourcesFromPO(items []ScoringSourcePO) []factor.ScoringSource {
	if items == nil {
		return nil
	}
	out := make([]factor.ScoringSource, 0, len(items))
	for _, item := range items {
		out = append(out, factor.ScoringSource{Kind: factor.ScoringSourceKind(item.Kind), Code: item.Code, ScoringMode: factor.QuestionScoringMode(item.ScoringMode), Sign: item.Sign, Weight: item.Weight, OptionScores: cloneFloat64Map(item.OptionScores)})
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
			out = append(out, ConclusionPO{
				Kind:           string(conclusion.KindType),
				FactorCodes:    append([]string(nil), typed.FactorCodes...),
				Outcomes:       outcomesToPO(typed.Outcomes),
				TypeDecision:   typeDecisionToPO(typed.Decision),
				SpecialRules:   typeSpecialRulesToPO(typed.SpecialRules),
				OutcomeMapping: typeOutcomeMappingToPO(typed.OutcomeMapping),
				Profiles:       typeOutcomeProfilesToPO(typed.Profiles),
			})
		case domain.NormConclusion:
			out = append(out, ConclusionPO{
				Kind: string(conclusion.KindNorm), FactorCode: typed.FactorCode, ScoreBasis: string(typed.ScoreBasis), Primary: typed.Primary,
				Rules: scoreRangeOutcomesToPO(typed.Rules), Outcomes: outcomesToPO(typed.Outcomes),
			})
		case domain.AbilityConclusion:
			out = append(out, ConclusionPO{
				Kind: string(conclusion.KindAbility), FactorCode: typed.FactorCode, ScoreBasis: string(typed.ScoreBasis),
				Rules: scoreRangeOutcomesToPO(typed.Rules), Outcomes: outcomesToPO(typed.Outcomes),
			})
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
			out = append(out, domain.TypeConclusion{
				FactorCodes:    append([]string(nil), item.FactorCodes...),
				Outcomes:       outcomesFromPO(item.Outcomes),
				Decision:       typeDecisionFromPO(item.TypeDecision),
				SpecialRules:   typeSpecialRulesFromPO(item.SpecialRules),
				OutcomeMapping: typeOutcomeMappingFromPO(item.OutcomeMapping),
				Profiles:       typeOutcomeProfilesFromPO(item.Profiles),
			})
		case conclusion.KindNorm:
			out = append(out, domain.NormConclusion{FactorCode: item.FactorCode, ScoreBasis: conclusion.ScoreBasis(item.ScoreBasis), Primary: item.Primary, Rules: scoreRangeOutcomesFromPO(item.Rules), Outcomes: outcomesFromPO(item.Outcomes)})
		case conclusion.KindAbility:
			out = append(out, domain.AbilityConclusion{FactorCode: item.FactorCode, ScoreBasis: conclusion.ScoreBasis(item.ScoreBasis), Rules: scoreRangeOutcomesFromPO(item.Rules), Outcomes: outcomesFromPO(item.Outcomes)})
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
			Level:       item.Level,
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
			Level:       item.Level,
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
			Code:          section.Code,
			Title:         section.Title,
			SourceRefs:    append([]string(nil), section.SourceRefs...),
			Kind:          section.Kind,
			AdapterKey:    section.AdapterKey,
			TemplateID:    section.TemplateID,
			CategoryLabel: section.CategoryLabel,
		})
	}
	return ReportMapPO{Sections: sections}
}

func reportMapFromPO(po ReportMapPO) domain.ReportMap {
	sections := make([]domain.ReportSection, 0, len(po.Sections))
	for _, section := range po.Sections {
		sections = append(sections, domain.ReportSection{
			Code:          section.Code,
			Title:         section.Title,
			SourceRefs:    append([]string(nil), section.SourceRefs...),
			Kind:          section.Kind,
			AdapterKey:    section.AdapterKey,
			TemplateID:    section.TemplateID,
			CategoryLabel: section.CategoryLabel,
		})
	}
	return domain.ReportMap{Sections: sections}
}

func typeDecisionToPO(value conclusion.TypeDecision) *TypeDecisionPO {
	if value.Kind == "" && value.LevelRule == nil && len(value.Poles) == 0 && value.FallbackCode == "" && value.FallbackSimilarityThreshold == 0 {
		return nil
	}
	out := &TypeDecisionPO{
		Kind: string(value.Kind), FallbackSimilarityThreshold: value.FallbackSimilarityThreshold, FallbackCode: value.FallbackCode,
		Poles: typePolesToPO(value.Poles),
	}
	if value.LevelRule != nil {
		out.LevelRule = &TypeLevelRulePO{LowMax: value.LevelRule.LowMax, HighMin: value.LevelRule.HighMin}
	}
	return out
}

func typeDecisionFromPO(value *TypeDecisionPO) conclusion.TypeDecision {
	if value == nil {
		return conclusion.TypeDecision{}
	}
	out := conclusion.TypeDecision{
		Kind: domain.DecisionKind(value.Kind), FallbackSimilarityThreshold: value.FallbackSimilarityThreshold, FallbackCode: value.FallbackCode,
		Poles: typePolesFromPO(value.Poles),
	}
	if value.LevelRule != nil {
		out.LevelRule = &conclusion.TypeLevelRule{LowMax: value.LevelRule.LowMax, HighMin: value.LevelRule.HighMin}
	}
	return out
}

func typePolesToPO(items []conclusion.TypePole) []TypePolePO {
	if items == nil {
		return nil
	}
	out := make([]TypePolePO, 0, len(items))
	for _, item := range items {
		out = append(out, TypePolePO{FactorCode: item.FactorCode, LeftPole: item.LeftPole, RightPole: item.RightPole, Threshold: item.Threshold, Model: item.Model})
	}
	return out
}

func typePolesFromPO(items []TypePolePO) []conclusion.TypePole {
	if items == nil {
		return nil
	}
	out := make([]conclusion.TypePole, 0, len(items))
	for _, item := range items {
		out = append(out, conclusion.TypePole{FactorCode: item.FactorCode, LeftPole: item.LeftPole, RightPole: item.RightPole, Threshold: item.Threshold, Model: item.Model})
	}
	return out
}

func typeSpecialRulesToPO(items []conclusion.TypeSpecialRule) []TypeSpecialRulePO {
	if items == nil {
		return nil
	}
	out := make([]TypeSpecialRulePO, 0, len(items))
	for _, item := range items {
		out = append(out, TypeSpecialRulePO{Code: item.Code, Kind: string(item.Kind), Phase: string(item.Phase), Trigger: item.Trigger, OutcomeCode: item.OutcomeCode, QuestionCodes: append([]string(nil), item.QuestionCodes...), OptionValues: append([]string(nil), item.OptionValues...)})
	}
	return out
}

func typeSpecialRulesFromPO(items []TypeSpecialRulePO) []conclusion.TypeSpecialRule {
	if items == nil {
		return nil
	}
	out := make([]conclusion.TypeSpecialRule, 0, len(items))
	for _, item := range items {
		out = append(out, conclusion.TypeSpecialRule{Code: item.Code, Kind: conclusion.TypeSpecialRuleKind(item.Kind), Phase: conclusion.TypeSpecialRulePhase(item.Phase), Trigger: item.Trigger, OutcomeCode: item.OutcomeCode, QuestionCodes: append([]string(nil), item.QuestionCodes...), OptionValues: append([]string(nil), item.OptionValues...)})
	}
	return out
}

func typeOutcomeMappingToPO(value conclusion.TypeOutcomeMapping) *TypeOutcomeMappingPO {
	if value.DetailKind == "" && value.DetailAdapterKey == "" && value.Algorithm == "" {
		return nil
	}
	return &TypeOutcomeMappingPO{DetailKind: value.DetailKind, DetailAdapterKey: value.DetailAdapterKey, Algorithm: string(value.Algorithm)}
}

func typeOutcomeMappingFromPO(value *TypeOutcomeMappingPO) conclusion.TypeOutcomeMapping {
	if value == nil {
		return conclusion.TypeOutcomeMapping{}
	}
	return conclusion.TypeOutcomeMapping{DetailKind: value.DetailKind, DetailAdapterKey: value.DetailAdapterKey, Algorithm: domain.Algorithm(value.Algorithm)}
}

func typeOutcomeProfilesToPO(items []conclusion.TypeOutcomeProfile) []TypeOutcomeProfilePO {
	if items == nil {
		return nil
	}
	out := make([]TypeOutcomeProfilePO, 0, len(items))
	for _, item := range items {
		out = append(out, TypeOutcomeProfilePO{OutcomeCode: item.OutcomeCode, Pattern: item.Pattern, Traits: append([]string(nil), item.Traits...), Strengths: append([]string(nil), item.Strengths...), Weaknesses: append([]string(nil), item.Weaknesses...), Suggestions: append([]string(nil), item.Suggestions...), ImageURL: item.ImageURL, Image: item.Image, Rarity: RarityPO{Percent: item.Rarity.Percent, Label: item.Rarity.Label, OneInX: item.Rarity.OneInX}, IsSpecial: item.IsSpecial, Trigger: item.Trigger, Commentary: item.Commentary})
	}
	return out
}

func typeOutcomeProfilesFromPO(items []TypeOutcomeProfilePO) []conclusion.TypeOutcomeProfile {
	if items == nil {
		return nil
	}
	out := make([]conclusion.TypeOutcomeProfile, 0, len(items))
	for _, item := range items {
		out = append(out, conclusion.TypeOutcomeProfile{OutcomeCode: item.OutcomeCode, Pattern: item.Pattern, Traits: append([]string(nil), item.Traits...), Strengths: append([]string(nil), item.Strengths...), Weaknesses: append([]string(nil), item.Weaknesses...), Suggestions: append([]string(nil), item.Suggestions...), ImageURL: item.ImageURL, Image: item.Image, Rarity: conclusion.Rarity{Percent: item.Rarity.Percent, Label: item.Rarity.Label, OneInX: item.Rarity.OneInX}, IsSpecial: item.IsSpecial, Trigger: item.Trigger, Commentary: item.Commentary})
	}
	return out
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
