// Package builder 负责面向机制 报告构建器 和 注册表。
package builder

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
	typology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ReportBuilder composes InterpretReport 从 机制无关 input。
type ReportBuilder = domainreport.ReportBuilder

// MechanismFamily 标识which 报告构建器 机制 到 use。
type MechanismFamily = modelcatalog.AlgorithmFamily

const (
	MechanismFactorScoring        = modelcatalog.AlgorithmFamilyFactorScoring
	MechanismFactorClassification = modelcatalog.AlgorithmFamilyFactorClassification
	MechanismFactorNorm           = modelcatalog.AlgorithmFamilyFactorNorm
	MechanismTaskPerformance      = modelcatalog.AlgorithmFamilyTaskPerformance
)

// FactorScoringBuilder 构建score-range reports。
func FactorScoringBuilder(composer ReportBuilder, input reportscore.ScaleReportInput) (*domainreport.InterpretReport, error) {
	return reportscore.BuildScaleReport(composer, input)
}

// TypologyBuilder 构建因子-分类 reports via 机制 templates。
var (
	BuildPersonalityTypeReport = typology.BuildPersonalityTypeReport
	BuildTraitProfileReport    = typology.BuildTraitProfileReport
)

// Registry 解析机制 builders 按 算法家族。
type Registry struct {
	byFamily map[MechanismFamily]ReportBuilder
}

// NewRegistry 创建空 机制 builder 注册表。
func NewRegistry() *Registry {
	return &Registry{byFamily: make(map[MechanismFamily]ReportBuilder)}
}

// Register 添加builder 用于 机制家族。
func (r *Registry) Register(family MechanismFamily, builder ReportBuilder) {
	if r == nil {
		return
	}
	r.byFamily[family] = builder
}

// Resolve 返回builder 用于 机制家族。
func (r *Registry) Resolve(family MechanismFamily) (ReportBuilder, bool) {
	if r == nil {
		return nil, false
	}
	builder, ok := r.byFamily[family]
	return builder, ok
}
