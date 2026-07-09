package definition

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// Definition 是测评模型定义主体，组合测量、校准、结论和报告映射。
type Definition struct {
	Measure     MeasureSpec
	Calibration Calibration
	Conclusions []conclusion.Conclusion
	Outcomes    []conclusion.Outcome
	ReportMap   ReportMap
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

type ReportSection struct {
	Code       string
	Title      string
	SourceRefs []string
}
