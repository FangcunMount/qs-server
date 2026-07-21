package cognitive

import (
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	catalognorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	portmodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/normruntime"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	sharedpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/shared"
)

// Snapshot is a transient cognitive runtime projection of DefinitionV2.
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

func (s *Snapshot) IsPublished() bool {
	return s != nil && s.Status == "published"
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
