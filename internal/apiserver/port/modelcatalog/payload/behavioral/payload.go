package behavioral

import (
	"encoding/json"
	"fmt"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	catalognorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	sharedpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/shared"
)

// Snapshot 是published behavioral_rating 执行载荷 (默认.v1 或 brief2.v1)。
type Snapshot struct {
	Code                 string
	Version              string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []FactorSnapshot
	Norming              *NormingProfile
}

// NormingProfile 携带常模化配置 beyond score_range 计分（机制中性，解析自 brief2 等扩展）。
type NormingProfile struct {
	Variant              string
	NormTableVersion     string
	IndexCodes           []string
	ValidityCodes        []string
	PrimaryDimensionCode string
	NormTables           *calcnorm.NormTables
}

// NormTablesOrNil 返回 parsed 常模表 when 常模配置存在。
func (p *NormingProfile) NormTablesOrNil() *calcnorm.NormTables {
	if p == nil {
		return nil
	}
	return p.NormTables
}

type FactorSnapshot struct {
	Code            string
	Title           string
	Role            factor.FactorRole
	ParentCode      string
	SortOrder       int
	Level           int
	IsTotalScore    bool
	QuestionCodes   []string
	ScoringStrategy string
	ScoringParams   *factor.ScoringParams
	MaxScore        *float64
	InterpretRules  []InterpretRuleSnapshot
	Norm            *catalognorm.Ref
	ChildrenPolicy  *factor.ChildrenPolicy
}

type InterpretRuleSnapshot = sharedpayload.ScoreRangeRule

func (f FactorSnapshot) ResolvedRole() factor.FactorRole {
	if f.Role != "" {
		return f.Role
	}
	if f.IsTotalScore {
		return factor.FactorRoleTotal
	}
	return factor.FactorRoleDimension
}

type definitionPayload struct {
	sharedpayload.DefinitionBody
	Brief2 *brief2Extension `json:"brief2,omitempty"`
}

type brief2Extension struct {
	FormVariant          string                 `json:"form_variant,omitempty"`
	NormTableVersion     string                 `json:"norm_table_version,omitempty"`
	IndexCodes           []string               `json:"index_codes,omitempty"`
	ValidityCodes        []string               `json:"validity_codes,omitempty"`
	PrimaryDimensionCode string                 `json:"primary_dimension_code,omitempty"`
	CompositeIndexes     []brief2CompositeIndex `json:"composite_indexes,omitempty"`
	Norms                []brief2FactorPayload  `json:"norms,omitempty"`
	TScoreRules          []brief2TScoreRule     `json:"t_score_rules,omitempty"`
}

type brief2CompositeIndex struct {
	Code       string   `json:"code"`
	Strategy   string   `json:"strategy,omitempty"`
	Children   []string `json:"children"`
	ParentCode string   `json:"parent_code,omitempty"`
}

type brief2FactorPayload struct {
	FactorCode string              `json:"factor_code"`
	Bands      []brief2NormBand    `json:"bands,omitempty"`
	Lookup     []brief2LookupEntry `json:"lookup,omitempty"`
}

type brief2NormBand struct {
	MinAgeMonths int      `json:"min_age_months,omitempty"`
	MaxAgeMonths int      `json:"max_age_months,omitempty"`
	Gender       string   `json:"gender,omitempty"`
	Mean         *float64 `json:"mean,omitempty"`
	StdDev       *float64 `json:"std_dev,omitempty"`
}

type brief2LookupEntry struct {
	RawMin     float64 `json:"raw_min"`
	RawMax     float64 `json:"raw_max"`
	TScore     float64 `json:"t_score"`
	Percentile float64 `json:"percentile"`
}

type brief2TScoreRule struct {
	FactorCode string              `json:"factor_code"`
	Ranges     []brief2TScoreRange `json:"ranges"`
}

type brief2TScoreRange struct {
	MinT       float64 `json:"min_t"`
	MaxT       float64 `json:"max_t"`
	Level      string  `json:"level,omitempty"`
	Conclusion string  `json:"conclusion,omitempty"`
	Suggestion string  `json:"suggestion,omitempty"`
}

// ParseDefinitionPayload de编码 behavioral_rating 载荷 body 为 运行时 快照。
func ParseDefinitionPayload(modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	return parseDefinitionPayload(modelCode, modelVersion, title, status, payload)
}

// ParsePublishedPayload de编码 已发布快照 using its 载荷格式 label。
func ParsePublishedPayload(payloadFormat, modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	switch payloadFormat {
	case "", "assessmentmodel.behavioral_rating.default.v1", "assessmentmodel.behavioral_rating.brief2.v1":
		return parseDefinitionPayload(modelCode, modelVersion, title, status, payload)
	default:
		return nil, fmt.Errorf("unsupported behavioral_rating payload format: %s", payloadFormat)
	}
}

func parseDefinitionPayload(modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	var body definitionPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("decode behavioral_rating payload: %w", err)
	}
	out := &Snapshot{
		Code:    modelCode,
		Version: modelVersion,
		Title:   title,
		Status:  status,
	}
	factors := factorSnapshotsFromDefinitionBody(body.Dimensions, body.InterpretRules)
	if body.Brief2 != nil {
		factors = applyBrief2NormMetadataToFactorSnapshots(factors, brief2MetadataContext{
			NormTableVersion: body.Brief2.NormTableVersion,
			IndexCodes:       append([]string(nil), body.Brief2.IndexCodes...),
			ValidityCodes:    append([]string(nil), body.Brief2.ValidityCodes...),
			NormFactorCodes:  normFactorCodesFromPayload(body.Brief2),
		})
		factors = applyCompositeMetadataToFactorSnapshots(factors, compositeSpecsFromPayload(body.Brief2))
	}
	out.Factors = factors
	if body.Brief2 != nil {
		out.Norming = &NormingProfile{
			Variant:              body.Brief2.FormVariant,
			NormTableVersion:     body.Brief2.NormTableVersion,
			IndexCodes:           append([]string(nil), body.Brief2.IndexCodes...),
			ValidityCodes:        append([]string(nil), body.Brief2.ValidityCodes...),
			PrimaryDimensionCode: legacyPrimaryDimensionCode(body.Brief2, factors),
			NormTables:           normTablesFromPayload(body.Brief2),
		}
	}
	return out, nil
}

func normTablesFromPayload(body *brief2Extension) *calcnorm.NormTables {
	if body == nil || (len(body.Norms) == 0 && len(body.TScoreRules) == 0) {
		return nil
	}
	tables := &calcnorm.NormTables{
		FormVariant:      body.FormVariant,
		NormTableVersion: body.NormTableVersion,
		Factors:          make([]calcnorm.FactorNormTable, 0, len(body.Norms)),
		TScoreRules:      make([]calcnorm.TScoreInterpretRule, 0, len(body.TScoreRules)),
	}
	for _, factor := range body.Norms {
		table := calcnorm.FactorNormTable{
			FactorCode: factor.FactorCode,
			Bands:      make([]calcnorm.NormBand, 0, len(factor.Bands)),
			Lookup:     make([]calcnorm.NormLookupEntry, 0, len(factor.Lookup)),
		}
		for _, band := range factor.Bands {
			table.Bands = append(table.Bands, calcnorm.NormBand{
				MinAgeMonths: band.MinAgeMonths,
				MaxAgeMonths: band.MaxAgeMonths,
				Gender:       band.Gender,
				Mean:         band.Mean,
				StdDev:       band.StdDev,
			})
		}
		for _, entry := range factor.Lookup {
			table.Lookup = append(table.Lookup, calcnorm.NormLookupEntry(entry))
		}
		tables.Factors = append(tables.Factors, table)
	}
	for _, rule := range body.TScoreRules {
		converted := calcnorm.TScoreInterpretRule{FactorCode: rule.FactorCode}
		for _, item := range rule.Ranges {
			converted.Ranges = append(converted.Ranges, calcnorm.TScoreRange(item))
		}
		tables.TScoreRules = append(tables.TScoreRules, converted)
	}
	return tables
}

func normFromPayload(body *brief2Extension) *catalognorm.Norm {
	if body == nil || body.NormTableVersion == "" || len(body.Norms) == 0 {
		return nil
	}
	out := &catalognorm.Norm{
		TableVersion: body.NormTableVersion,
		FormVariant:  body.FormVariant,
		Factors:      make([]catalognorm.FactorTable, 0, len(body.Norms)),
	}
	for _, factorPayload := range body.Norms {
		factorTable := catalognorm.FactorTable{
			FactorCode: factorPayload.FactorCode,
			Bands:      make([]catalognorm.Band, 0, len(factorPayload.Bands)),
			Lookup:     make([]catalognorm.LookupEntry, 0, len(factorPayload.Lookup)),
		}
		for _, band := range factorPayload.Bands {
			factorTable.Bands = append(factorTable.Bands, catalognorm.Band{
				MinAgeMonths: band.MinAgeMonths, MaxAgeMonths: band.MaxAgeMonths, Gender: band.Gender,
				Mean: cloneFloat64(band.Mean), StdDev: cloneFloat64(band.StdDev),
			})
		}
		for _, entry := range factorPayload.Lookup {
			factorTable.Lookup = append(factorTable.Lookup, catalognorm.LookupEntry{
				RawScoreMin: entry.RawMin, RawScoreMax: entry.RawMax, TScore: entry.TScore, Percentile: entry.Percentile,
			})
		}
		out.Factors = append(out.Factors, factorTable)
	}
	return out
}

func normConclusionsFromPayload(body *brief2Extension) []conclusion.Conclusion {
	if body == nil {
		return nil
	}
	items := make([]conclusion.Conclusion, 0, len(body.TScoreRules)+1)
	seen := make(map[string]struct{}, len(body.TScoreRules))
	for _, rule := range body.TScoreRules {
		if rule.FactorCode == "" {
			continue
		}
		ranges := make([]conclusion.ScoreRangeOutcome, 0, len(rule.Ranges))
		for _, item := range rule.Ranges {
			ranges = append(ranges, conclusion.ScoreRangeOutcome{
				MinScore: item.MinT, MaxScore: item.MaxT, Level: item.Level, Summary: item.Conclusion, Description: item.Suggestion,
			})
		}
		items = append(items, conclusion.NormConclusion{
			FactorCode: rule.FactorCode, ScoreBasis: conclusion.ScoreBasisTScore,
			Primary: rule.FactorCode == body.PrimaryDimensionCode, Rules: ranges,
		})
		seen[rule.FactorCode] = struct{}{}
	}
	if body.PrimaryDimensionCode != "" {
		if _, ok := seen[body.PrimaryDimensionCode]; !ok {
			items = append(items, conclusion.NormConclusion{FactorCode: body.PrimaryDimensionCode, ScoreBasis: conclusion.ScoreBasisTScore, Primary: true})
		}
	}
	return items
}

func normFactorCodesFromPayload(body *brief2Extension) []string {
	if body == nil || len(body.Norms) == 0 {
		return nil
	}
	codes := make([]string, 0, len(body.Norms))
	for _, item := range body.Norms {
		if item.FactorCode != "" {
			codes = append(codes, item.FactorCode)
		}
	}
	return codes
}

func compositeSpecsFromPayload(body *brief2Extension) []brief2CompositeIndexSpec {
	if body == nil || len(body.CompositeIndexes) == 0 {
		return nil
	}
	specs := make([]brief2CompositeIndexSpec, 0, len(body.CompositeIndexes))
	for _, item := range body.CompositeIndexes {
		if item.Code == "" || len(item.Children) == 0 {
			continue
		}
		specs = append(specs, brief2CompositeIndexSpec{
			Code:       item.Code,
			Strategy:   factor.ChildrenAggregationStrategy(item.Strategy),
			Children:   append([]string(nil), item.Children...),
			ParentCode: item.ParentCode,
		})
	}
	return specs
}

func (s *Snapshot) IsPublished() bool {
	return s != nil && s.Status == "published"
}

func legacyPrimaryDimensionCode(body *brief2Extension, factors []FactorSnapshot) string {
	if body == nil {
		return ""
	}
	if body.PrimaryDimensionCode != "" {
		return body.PrimaryDimensionCode
	}
	for _, code := range []string{"gec", "total"} {
		for _, factor := range factors {
			if factor.Code == code {
				return code
			}
		}
	}
	return ""
}

// MeasureSpec projects the runtime behavioral snapshot back to the target
// measure layer for calculation graph validation/projection.
func (s *Snapshot) MeasureSpec() definition.MeasureSpec {
	if s == nil {
		return definition.MeasureSpec{}
	}
	return measureSpecFromFactorSnapshots(s.Factors)
}

// ToScaleSnapshot 投影behavioral_rating 因子 为 scale execution 结构。
func (s *Snapshot) ToScaleSnapshot() *scalesnapshot.ScaleSnapshot {
	if s == nil {
		return nil
	}
	factors := make([]scalesnapshot.FactorSnapshot, 0, len(s.Factors))
	for _, item := range s.Factors {
		factors = append(factors, scaleFactorSnapshotFromBehavioral(item))
	}
	return &scalesnapshot.ScaleSnapshot{
		Code:                 s.Code,
		ScaleVersion:         s.Version,
		Title:                s.Title,
		QuestionnaireCode:    s.QuestionnaireCode,
		QuestionnaireVersion: s.QuestionnaireVersion,
		Status:               s.Status,
		Factors:              factors,
	}
}

func factorSnapshotsFromDefinitionBody(dimensions []sharedpayload.DimensionRule, interpretRules []sharedpayload.InterpretRule) []FactorSnapshot {
	if dimensions == nil {
		return nil
	}
	rulesByDimension := make(map[string][]InterpretRuleSnapshot, len(interpretRules))
	for _, rule := range interpretRules {
		rulesByDimension[rule.DimensionCode] = cloneInterpretRules(rule.Ranges)
	}
	out := make([]FactorSnapshot, 0, len(dimensions))
	for _, item := range dimensions {
		role := factor.FactorRole(item.Role)
		if role != "" && !role.IsValid() {
			role = ""
		}
		out = append(out, FactorSnapshot{
			Code:            item.Code,
			Title:           item.Title,
			Role:            role,
			ParentCode:      item.ParentCode,
			SortOrder:       item.SortOrder,
			Level:           item.Level,
			IsTotalScore:    item.IsTotalScore,
			QuestionCodes:   cloneStrings(item.QuestionCodes),
			ScoringStrategy: item.ScoringStrategy,
			ScoringParams:   scoringParamsFromPayload(item.ScoringParams),
			MaxScore:        cloneFloat64(item.MaxScore),
			InterpretRules:  cloneInterpretRules(rulesByDimension[item.Code]),
			ChildrenPolicy:  childrenPolicyFromPayload(item.ChildrenPolicy),
		})
	}
	return out
}

func applyBrief2NormMetadataToFactorSnapshots(factors []FactorSnapshot, ctx brief2MetadataContext) []FactorSnapshot {
	if len(factors) == 0 {
		return factors
	}
	indexCodes := stringSet(ctx.IndexCodes)
	validityCodes := stringSet(ctx.ValidityCodes)
	normFactorCodes := stringSet(ctx.NormFactorCodes)
	out := make([]FactorSnapshot, len(factors))
	for i, item := range factors {
		out[i] = cloneFactorSnapshot(item)
		switch {
		case indexCodes[item.Code]:
			out[i].Role = factor.FactorRoleIndex
		case validityCodes[item.Code]:
			out[i].Role = factor.FactorRoleValidity
		}
		if normFactorCodes[item.Code] && ctx.NormTableVersion != "" {
			out[i].Norm = &catalognorm.Ref{FactorCode: item.Code, NormTableVersion: ctx.NormTableVersion}
		}
	}
	return out
}

func applyCompositeMetadataToFactorSnapshots(factors []FactorSnapshot, specs []brief2CompositeIndexSpec) []FactorSnapshot {
	if len(factors) == 0 || len(specs) == 0 {
		return factors
	}
	out := make([]FactorSnapshot, len(factors))
	for i, item := range factors {
		out[i] = cloneFactorSnapshot(item)
	}
	indexPos := make(map[string]int, len(out))
	for i, item := range out {
		indexPos[item.Code] = i
	}
	for _, spec := range specs {
		pos, ok := indexPos[spec.Code]
		if !ok || len(spec.Children) == 0 {
			continue
		}
		strategy := spec.Strategy
		if strategy == "" {
			strategy = factor.ChildrenAggregationSum
		}
		out[pos].ChildrenPolicy = &factor.ChildrenPolicy{
			Strategy: strategy,
			Children: cloneStrings(spec.Children),
		}
		if spec.ParentCode != "" {
			out[pos].ParentCode = spec.ParentCode
		}
		for _, childCode := range spec.Children {
			childPos, ok := indexPos[childCode]
			if !ok || out[childPos].ParentCode != "" {
				continue
			}
			out[childPos].ParentCode = spec.Code
		}
	}
	return deriveFactorSnapshotLevels(out)
}

func cloneFactorSnapshot(item FactorSnapshot) FactorSnapshot {
	return FactorSnapshot{
		Code:            item.Code,
		Title:           item.Title,
		Role:            item.Role,
		ParentCode:      item.ParentCode,
		SortOrder:       item.SortOrder,
		Level:           item.Level,
		IsTotalScore:    item.IsTotalScore,
		QuestionCodes:   cloneStrings(item.QuestionCodes),
		ScoringStrategy: item.ScoringStrategy,
		ScoringParams:   cloneScoringParams(item.ScoringParams),
		MaxScore:        cloneFloat64(item.MaxScore),
		InterpretRules:  cloneInterpretRules(item.InterpretRules),
		Norm:            cloneNormRef(item.Norm),
		ChildrenPolicy:  cloneChildrenPolicy(item.ChildrenPolicy),
	}
}

func deriveFactorSnapshotLevels(factors []FactorSnapshot) []FactorSnapshot {
	if len(factors) == 0 {
		return nil
	}
	byCode := make(map[string]FactorSnapshot, len(factors))
	for _, item := range factors {
		byCode[item.Code] = item
	}
	out := make([]FactorSnapshot, len(factors))
	for i, item := range factors {
		out[i] = cloneFactorSnapshot(item)
	}
	memo := make(map[string]int, len(factors))
	var walk func(code string) int
	walk = func(code string) int {
		if level, ok := memo[code]; ok {
			return level
		}
		item, ok := byCode[code]
		if !ok {
			return 0
		}
		if item.Level > 0 {
			memo[code] = item.Level
			return item.Level
		}
		if item.ParentCode == "" {
			memo[code] = 1
			return 1
		}
		level := walk(item.ParentCode) + 1
		memo[code] = level
		return level
	}
	for i := range out {
		out[i].Level = walk(out[i].Code)
	}
	return out
}

func measureSpecFromFactorSnapshots(factors []FactorSnapshot) definition.MeasureSpec {
	if factors == nil {
		return definition.MeasureSpec{}
	}
	domainFactors := make([]factor.Factor, 0, len(factors))
	scoring := make([]factor.Scoring, 0, len(factors))
	graph := factor.FactorGraph{
		Roots:      make([]string, 0, len(factors)),
		Edges:      make([]factor.FactorEdge, 0, len(factors)),
		SortOrders: make(map[string]int),
	}
	hasParent := make(map[string]bool, len(factors))
	seenEdges := make(map[factor.FactorEdge]struct{})
	for _, item := range factors {
		domainFactors = append(domainFactors, factor.Factor{
			Code:  item.Code,
			Title: item.Title,
			Role:  item.ResolvedRole(),
		})
		if item.SortOrder != 0 {
			graph.SortOrders[item.Code] = item.SortOrder
		}
		if item.ParentCode != "" {
			edge := factor.FactorEdge{ParentCode: item.ParentCode, ChildCode: item.Code}
			if _, ok := seenEdges[edge]; !ok {
				graph.Edges = append(graph.Edges, edge)
				seenEdges[edge] = struct{}{}
			}
			hasParent[item.Code] = true
		}
		if item.ChildrenPolicy != nil && len(item.ChildrenPolicy.Children) > 0 {
			sources := make([]factor.ScoringSource, 0, len(item.ChildrenPolicy.Children))
			for _, childCode := range item.ChildrenPolicy.Children {
				edge := factor.FactorEdge{ParentCode: item.Code, ChildCode: childCode}
				if _, ok := seenEdges[edge]; !ok {
					graph.Edges = append(graph.Edges, edge)
					seenEdges[edge] = struct{}{}
				}
				hasParent[childCode] = true
				sources = append(sources, factor.ScoringSource{Kind: factor.ScoringSourceFactor, Code: childCode})
			}
			scoring = append(scoring, factor.Scoring{
				FactorCode: item.Code,
				Sources:    sources,
				Strategy:   factor.ScoringStrategy(item.ChildrenPolicy.Strategy),
				Params:     cloneScoringParams(item.ScoringParams),
				MaxScore:   cloneFloat64(item.MaxScore),
				Weights:    cloneWeights(item.ChildrenPolicy.Weights),
			})
			continue
		}
		if len(item.QuestionCodes) > 0 || item.ScoringStrategy != "" || item.ScoringParams != nil || item.MaxScore != nil {
			scoring = append(scoring, factor.Scoring{
				FactorCode: item.Code,
				Sources:    questionSources(item.QuestionCodes),
				Strategy:   factor.ScoringStrategy(item.ScoringStrategy),
				Params:     cloneScoringParams(item.ScoringParams),
				MaxScore:   cloneFloat64(item.MaxScore),
			})
		}
	}
	for _, item := range factors {
		if !hasParent[item.Code] {
			graph.Roots = append(graph.Roots, item.Code)
		}
	}
	if len(graph.SortOrders) == 0 {
		graph.SortOrders = nil
	}
	return definition.MeasureSpec{
		Factors:     domainFactors,
		FactorGraph: graph,
		Scoring:     scoring,
	}
}

func questionSources(codes []string) []factor.ScoringSource {
	if len(codes) == 0 {
		return nil
	}
	out := make([]factor.ScoringSource, 0, len(codes))
	for _, code := range codes {
		out = append(out, factor.ScoringSource{Kind: factor.ScoringSourceQuestion, Code: code})
	}
	return out
}

func scoringParamsFromPayload(payload *sharedpayload.ScoringParamsPayload) *factor.ScoringParams {
	if payload == nil || len(payload.CntOptionContents) == 0 {
		return nil
	}
	return &factor.ScoringParams{CntOptionContents: cloneStrings(payload.CntOptionContents)}
}

func childrenPolicyFromPayload(payload *sharedpayload.ChildrenPolicyPayload) *factor.ChildrenPolicy {
	if payload == nil {
		return nil
	}
	return &factor.ChildrenPolicy{
		Strategy: factor.ChildrenAggregationStrategy(payload.Strategy),
		Children: cloneStrings(payload.Children),
		Weights:  cloneWeights(payload.Weights),
	}
}

func stringSet(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		set[value] = true
	}
	return set
}

func cloneStrings(items []string) []string {
	if items == nil {
		return nil
	}
	return append([]string(nil), items...)
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneScoringParams(params *factor.ScoringParams) *factor.ScoringParams {
	if params == nil {
		return nil
	}
	return &factor.ScoringParams{CntOptionContents: cloneStrings(params.CntOptionContents)}
}

func cloneInterpretRules(rules []InterpretRuleSnapshot) []InterpretRuleSnapshot {
	if rules == nil {
		return nil
	}
	return append([]InterpretRuleSnapshot(nil), rules...)
}

func cloneNormRef(ref *catalognorm.Ref) *catalognorm.Ref {
	if ref == nil {
		return nil
	}
	cloned := *ref
	return &cloned
}

func cloneChildrenPolicy(policy *factor.ChildrenPolicy) *factor.ChildrenPolicy {
	if policy == nil {
		return nil
	}
	return &factor.ChildrenPolicy{
		Strategy: policy.Strategy,
		Children: cloneStrings(policy.Children),
		Weights:  cloneWeights(policy.Weights),
	}
}

func cloneWeights(weights map[string]float64) map[string]float64 {
	if weights == nil {
		return nil
	}
	out := make(map[string]float64, len(weights))
	for key, value := range weights {
		out[key] = value
	}
	return out
}

func scaleFactorSnapshotFromBehavioral(item FactorSnapshot) scalesnapshot.FactorSnapshot {
	rules := make([]scalesnapshot.InterpretRuleSnapshot, 0, len(item.InterpretRules))
	for _, rule := range item.InterpretRules {
		rules = append(rules, scalesnapshot.InterpretRuleSnapshot{
			Min:        rule.MinScore,
			Max:        rule.MaxScore,
			RiskLevel:  rule.Level,
			Conclusion: rule.Conclusion,
			Suggestion: rule.Suggestion,
		})
	}
	var params scalesnapshot.ScoringParamsSnapshot
	if item.ScoringParams != nil {
		params.CntOptionContents = append([]string(nil), item.ScoringParams.CntOptionContents...)
	}
	return scalesnapshot.FactorSnapshot{
		Code:            item.Code,
		Title:           item.Title,
		IsTotalScore:    item.ResolvedRole() == factor.FactorRoleTotal,
		QuestionCodes:   append([]string(nil), item.QuestionCodes...),
		ScoringStrategy: item.ScoringStrategy,
		ScoringParams:   params,
		MaxScore:        item.MaxScore,
		InterpretRules:  rules,
	}
}
