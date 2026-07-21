package cognitive

import (
	"encoding/json"
	"fmt"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	catalognorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	taskperf "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/taskperformance"
	portmodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/normruntime"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	sharedpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/shared"
)

// Snapshot 是published cognitive 执行载荷 (default.v1 或 spm.v1)。
type Snapshot struct {
	Code                 string
	Version              string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []FactorSnapshot
	AbilityConclusions   []conclusion.AbilityConclusion
	SPM                  *SPMSpec

	// PublishedRuntime is evaluation-only metadata from AssessmentSnapshot; not JSON payload.
	PublishedRuntime *portmodelcatalog.PublishedRuntimeMeta
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
	SPM *spmExtension `json:"spm,omitempty"`
}

type spmExtension struct {
	TimeLimitSeconds   int                        `json:"time_limit_seconds,omitempty"`
	ItemSetCodes       []string                   `json:"item_set_codes,omitempty"`
	ItemSets           []spmItemSetPayload        `json:"item_sets,omitempty"`
	TotalFactorCode    string                     `json:"total_factor_code,omitempty"`
	NormTableVersion   string                     `json:"norm_table_version,omitempty"`
	AbilityConclusions []abilityConclusionPayload `json:"ability_conclusions,omitempty"`
}

// SPMSpec is the runtime view of canonical SPM execution rules.
type SPMSpec struct {
	TimeLimitSeconds int
	TotalFactorCode  string
	ItemSets         []SPMItemSet
	NormRequired     bool
	NormTables       *calcnorm.NormTables
}

// NormTablesFromCatalog converts the immutable catalog table into the
// calculation DTO used by native SPM execution.
func NormTablesFromCatalog(table *catalognorm.Norm) (*calcnorm.NormTables, error) {
	return normruntime.FromCatalog(table)
}

type SPMItemSet struct {
	Code  string
	Items []SPMItem
}

type SPMItem struct {
	QuestionCode      string
	CorrectOptionCode string
}

type spmItemSetPayload struct {
	Code  string           `json:"code"`
	Items []spmItemPayload `json:"items"`
}

type spmItemPayload struct {
	QuestionCode      string `json:"question_code"`
	CorrectOptionCode string `json:"correct_option_code"`
}

type abilityConclusionPayload struct {
	FactorCode string                   `json:"factor_code"`
	ScoreBasis string                   `json:"score_basis"`
	Primary    bool                     `json:"primary,omitempty"`
	Ranges     []abilityConclusionRange `json:"ranges"`
}

type abilityConclusionRange struct {
	MinScore     float64 `json:"min_score"`
	MaxScore     float64 `json:"max_score,omitempty"`
	MaxInclusive bool    `json:"max_inclusive,omitempty"`
	UnboundedMax bool    `json:"unbounded_max,omitempty"`
	Level        string  `json:"level,omitempty"`
	OutcomeCode  string  `json:"outcome_code,omitempty"`
	Title        string  `json:"title,omitempty"`
	Summary      string  `json:"summary,omitempty"`
	Description  string  `json:"description,omitempty"`
}

// ParseDefinitionPayload de编码 cognitive 载荷 body 为 运行时 快照。
func ParseDefinitionPayload(modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	return parseDefinitionPayload(modelCode, modelVersion, title, status, payload)
}

// ImportLegacyDefinition projects cognitive wire payload semantics to DefinitionV2.
func ImportLegacyDefinition(payload []byte) (sharedpayload.DefinitionMaterialization, error) {
	var body definitionPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return sharedpayload.DefinitionMaterialization{}, fmt.Errorf("decode cognitive definition: %w", err)
	}
	measure := sharedpayload.MeasureSpecFromDefinitionBody(body.DefinitionBody)
	calibration := definition.Calibration{}
	conclusions := make([]conclusion.Conclusion, 0)
	if body.SPM != nil {
		measure, calibration = taskperf.ApplyNormMetadata(measure, taskperf.MetadataContext{
			NormTableVersion: body.SPM.NormTableVersion,
			ItemSetCodes:     append([]string(nil), body.SPM.ItemSetCodes...),
		})
		conclusions = append(conclusions, abilityConclusionsFromPayload(body.SPM)...)
	}
	def := &definition.Definition{Measure: measure, Calibration: calibration, Conclusions: conclusions}
	if body.SPM != nil {
		def.Execution.SPM = definitionSPMSpecFromPayload(body.SPM, measure)
	}
	return sharedpayload.DefinitionMaterialization{Definition: def}, nil
}

// DefinitionFromLegacyPayload imports the cognitive wire payload as DefinitionV2.
func DefinitionFromLegacyPayload(payload []byte) (*definition.Definition, error) {
	materialized, err := ImportLegacyDefinition(payload)
	if err != nil {
		return nil, err
	}
	return materialized.Definition, nil
}

// PayloadFromDefinition projects canonical cognitive semantics to the legacy
// wire body. The norm table itself remains external; its version is retained
// through Calibration.NormRefs.
func PayloadFromDefinition(def *definition.Definition) ([]byte, error) {
	body := definitionPayload{DefinitionBody: sharedpayload.DefinitionBodyFromDefinition(def)}
	spm := spmExtensionFromDefinition(def)
	if spm != nil {
		body.SPM = spm
	}
	return json.Marshal(body)
}

func spmExtensionFromDefinition(def *definition.Definition) *spmExtension {
	if def == nil {
		return nil
	}
	ext := &spmExtension{}
	if spec := def.Execution.SPM; spec != nil {
		ext.TimeLimitSeconds = spec.TimeLimitSeconds
		ext.TotalFactorCode = spec.TotalFactorCode
		ext.ItemSets = spmItemSetsToPayload(spec.ItemSets)
		for _, set := range spec.ItemSets {
			ext.ItemSetCodes = append(ext.ItemSetCodes, set.Code)
		}
	}
	for _, ref := range def.Calibration.NormRefs {
		if ext.NormTableVersion == "" {
			ext.NormTableVersion = ref.NormTableVersion
		}
	}
	for _, item := range def.Measure.Factors {
		if item.ResolvedRole() == factor.FactorRoleTaskSet {
			ext.ItemSetCodes = append(ext.ItemSetCodes, item.Code)
		}
	}
	for _, item := range def.Conclusions {
		ability, ok := item.(conclusion.AbilityConclusion)
		if !ok {
			continue
		}
		entry := abilityConclusionPayload{FactorCode: ability.FactorCode, ScoreBasis: string(ability.ScoreBasis), Primary: ability.Primary, Ranges: make([]abilityConclusionRange, 0, len(ability.Rules))}
		for _, value := range ability.Rules {
			entry.Ranges = append(entry.Ranges, abilityConclusionRange{
				MinScore: value.MinScore, MaxScore: value.MaxScore, MaxInclusive: value.MaxInclusive, UnboundedMax: value.UnboundedMax,
				Level: value.Level, OutcomeCode: value.OutcomeCode, Title: value.Title, Summary: value.Summary, Description: value.Description,
			})
		}
		ext.AbilityConclusions = append(ext.AbilityConclusions, entry)
	}
	if ext.NormTableVersion == "" && len(ext.ItemSetCodes) == 0 && len(ext.ItemSets) == 0 && len(ext.AbilityConclusions) == 0 {
		return nil
	}
	return ext
}

func spmItemSetsToPayload(items []definition.SPMItemSet) []spmItemSetPayload {
	if items == nil {
		return nil
	}
	out := make([]spmItemSetPayload, 0, len(items))
	for _, set := range items {
		values := make([]spmItemPayload, 0, len(set.Items))
		for _, item := range set.Items {
			values = append(values, spmItemPayload{QuestionCode: item.QuestionCode, CorrectOptionCode: item.CorrectOptionCode})
		}
		out = append(out, spmItemSetPayload{Code: set.Code, Items: values})
	}
	return out
}

func definitionSPMSpecFromPayload(payload *spmExtension, measure definition.MeasureSpec) *definition.SPMSpec {
	if payload == nil {
		return nil
	}
	spec := &definition.SPMSpec{TimeLimitSeconds: payload.TimeLimitSeconds, TotalFactorCode: payload.TotalFactorCode}
	if spec.TotalFactorCode == "" {
		for _, item := range measure.Factors {
			if item.ResolvedRole() == factor.FactorRoleTotal {
				spec.TotalFactorCode = item.Code
				break
			}
		}
	}
	for _, set := range payload.ItemSets {
		out := definition.SPMItemSet{Code: set.Code, Items: make([]definition.SPMItem, 0, len(set.Items))}
		for _, item := range set.Items {
			out.Items = append(out.Items, definition.SPMItem{QuestionCode: item.QuestionCode, CorrectOptionCode: item.CorrectOptionCode})
		}
		spec.ItemSets = append(spec.ItemSets, out)
	}
	return spec
}

func abilityConclusionsFromPayload(spm *spmExtension) []conclusion.Conclusion {
	if spm == nil || len(spm.AbilityConclusions) == 0 {
		return nil
	}
	out := make([]conclusion.Conclusion, 0, len(spm.AbilityConclusions))
	for _, item := range spm.AbilityConclusions {
		ranges := make([]conclusion.ScoreRangeOutcome, 0, len(item.Ranges))
		for _, value := range item.Ranges {
			ranges = append(ranges, conclusion.ScoreRangeOutcome{
				MinScore: value.MinScore, MaxScore: value.MaxScore, MaxInclusive: value.MaxInclusive, UnboundedMax: value.UnboundedMax,
				Level: value.Level, OutcomeCode: value.OutcomeCode,
				Title: value.Title, Summary: value.Summary, Description: value.Description,
			})
		}
		out = append(out, conclusion.AbilityConclusion{
			FactorCode: item.FactorCode, ScoreBasis: conclusion.ScoreBasis(item.ScoreBasis), Primary: item.Primary, Rules: ranges,
		})
	}
	return out
}

// ParsePublishedPayload de编码 已发布快照 using its 载荷格式 label。
func ParsePublishedPayload(payloadFormat, modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	switch payloadFormat {
	case "", "assessmentmodel.cognitive.default.v1", "assessmentmodel.cognitive.spm.v1":
		return parseDefinitionPayload(modelCode, modelVersion, title, status, payload)
	default:
		return nil, fmt.Errorf("unsupported cognitive payload format: %s", payloadFormat)
	}
}

func parseDefinitionPayload(modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	var body definitionPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("decode cognitive payload: %w", err)
	}
	out := &Snapshot{
		Code:    modelCode,
		Version: modelVersion,
		Title:   title,
		Status:  status,
	}
	factors := factorSnapshotsFromDefinitionBody(body.Dimensions, body.InterpretRules)
	if body.SPM != nil {
		factors = applyNormMetadataToFactorSnapshots(factors, taskperf.MetadataContext{
			NormTableVersion: body.SPM.NormTableVersion,
			ItemSetCodes:     append([]string(nil), body.SPM.ItemSetCodes...),
		})
	}
	out.Factors = factors
	if body.SPM != nil {
		out.SPM = runtimeSPMSpecFromPayload(body.SPM, factors)
	}
	return out, nil
}

func runtimeSPMSpecFromPayload(payload *spmExtension, factors []FactorSnapshot) *SPMSpec {
	if payload == nil {
		return nil
	}
	spec := &SPMSpec{TimeLimitSeconds: payload.TimeLimitSeconds, TotalFactorCode: payload.TotalFactorCode}
	if spec.TotalFactorCode == "" {
		for _, item := range factors {
			if item.ResolvedRole() == factor.FactorRoleTotal {
				spec.TotalFactorCode = item.Code
				break
			}
		}
	}
	for _, item := range factors {
		if item.Code == spec.TotalFactorCode && item.Norm != nil && item.Norm.NormTableVersion != "" {
			spec.NormRequired = true
			break
		}
	}
	for _, set := range payload.ItemSets {
		out := SPMItemSet{Code: set.Code, Items: make([]SPMItem, 0, len(set.Items))}
		for _, item := range set.Items {
			out.Items = append(out.Items, SPMItem(item))
		}
		spec.ItemSets = append(spec.ItemSets, out)
	}
	return spec
}

func (s *Snapshot) IsPublished() bool {
	return s != nil && s.Status == "published"
}

// MeasureSpec projects the runtime cognitive snapshot back to the target
// measure layer for calculation graph validation/projection.
func (s *Snapshot) MeasureSpec() definition.MeasureSpec {
	if s == nil {
		return definition.MeasureSpec{}
	}
	return measureSpecFromFactorSnapshots(s.Factors)
}

// ToScaleSnapshot 投影cognitive 因子 为 scale execution 结构。
func (s *Snapshot) ToScaleSnapshot() *scalesnapshot.ScaleSnapshot {
	if s == nil {
		return nil
	}
	factors := make([]scalesnapshot.FactorSnapshot, 0, len(s.Factors))
	for _, item := range s.Factors {
		factors = append(factors, scaleFactorSnapshotFromCognitive(item))
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

func applyNormMetadataToFactorSnapshots(factors []FactorSnapshot, ctx taskperf.MetadataContext) []FactorSnapshot {
	if len(factors) == 0 {
		return factors
	}
	itemSetCodes := stringSet(ctx.ItemSetCodes)
	out := make([]FactorSnapshot, len(factors))
	for i, item := range factors {
		out[i] = cloneFactorSnapshot(item)
		if itemSetCodes[item.Code] {
			out[i].Role = factor.FactorRoleTaskSet
		}
		if ctx.NormTableVersion != "" && (item.ResolvedRole() == factor.FactorRoleTotal || itemSetCodes[item.Code]) {
			out[i].Norm = &catalognorm.Ref{FactorCode: item.Code, NormTableVersion: ctx.NormTableVersion}
		}
	}
	return out
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

func scaleFactorSnapshotFromCognitive(item FactorSnapshot) scalesnapshot.FactorSnapshot {
	rules := make([]scalesnapshot.InterpretRuleSnapshot, 0, len(item.InterpretRules))
	for _, rule := range item.InterpretRules {
		rules = append(rules, scalesnapshot.InterpretRuleSnapshot{
			Min:          rule.MinScore,
			Max:          rule.MaxScore,
			MaxInclusive: rule.MaxInclusive,
			UnboundedMax: rule.UnboundedMax,
			RiskLevel:    rule.Level,
			Conclusion:   rule.Conclusion,
			Suggestion:   rule.Suggestion,
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
