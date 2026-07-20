package definition

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/decision"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// Definition 是测评模型定义主体，组合测量、校准、结论和报告映射。
type Definition struct {
	Measure              MeasureSpec
	Calibration          Calibration
	Execution            ExecutionSpec
	Conclusions          []conclusion.Conclusion
	Outcomes             []conclusion.Outcome
	ReportMap            ReportMap
	DecisionSpec         decision.Spec                 `json:"DecisionSpec,omitempty"`
	InterpretationAssets interpretationassets.Assets     `json:"InterpretationAssets,omitempty"`
}

// ExecutionSpec carries algorithm-specific semantics that cannot be expressed
// as generic factor scoring. Its populated branch must match model identity.
type ExecutionSpec struct {
	Brief2 *Brief2Spec
	SPM    *SPMSpec
}

// Brief2Spec declares the form and factor roles used by a BRIEF-2 model.
// Norm table versions remain in Calibration so they can be versioned centrally.
type Brief2Spec struct {
	FormVariant         string
	PrimaryFactorCode   string
	IndexFactorCodes    []string
	ValidityFactorCodes []string
}

// SPMSpec is the immutable Raven SPM execution contract published with a
// model. TimeLimitSeconds is supplied to the test-taking client only; this
// service does not reject a submitted answer sheet based on elapsed time.
type SPMSpec struct {
	TimeLimitSeconds int
	TotalFactorCode  string
	ItemSets         []SPMItemSet
}

type SPMItemSet struct {
	Code  string
	Items []SPMItem
}

type SPMItem struct {
	QuestionCode      string
	CorrectOptionCode string
}

// MeasureSpec 描述测什么以及如何从题目得到因子分。
type MeasureSpec struct {
	Factors     []factor.Factor
	FactorGraph factor.FactorGraph
	Scoring     []factor.Scoring
}

// Calibration 描述测量结果进入结论前需要使用的校准资料。
type Calibration struct {
	NormRefs []norm.Ref
}

// ReportMap 描述模型配置和 evaluation 结果如何映射为报告展示。
type ReportMap struct {
	Sections []ReportSection
}

const ReportSectionKindFactorScores = "factor_scores"

type ReportSection struct {
	Code          string
	Title         string
	SourceRefs    []string
	Kind          string
	AdapterKey    string
	TemplateID    string
	CategoryLabel string
}

// FactorScoreSources returns the explicitly configured report-visible factor
// codes. The bool distinguishes an absent mapping from an intentional empty
// mapping that hides every factor score.
func (m ReportMap) FactorScoreSources() ([]string, bool) {
	for _, section := range m.Sections {
		if section.Kind != ReportSectionKindFactorScores {
			continue
		}
		return append([]string(nil), section.SourceRefs...), true
	}
	return nil, false
}
