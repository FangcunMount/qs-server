package reporting

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type reportBuilderKey struct {
	key        evaluation.ExecutionIdentity
	reportType domainReport.ReportType
}

// ReportBuilderRegistry 解析报告构建器 按 评估器键 和 report type。
type ReportBuilderRegistry interface {
	Resolve(key evaluation.ExecutionIdentity, reportType domainReport.ReportType) (ReportBuilder, error)
	ResolveByMechanism(key MechanismReportBuilderKey) (ReportBuilder, error)
}

type mutableReportBuilderRegistry struct {
	items          map[reportBuilderKey]ReportBuilder
	mechanismItems map[MechanismReportBuilderKey]ReportBuilder
}

// NewReportBuilderRegistry 创建注册表 从 given builders。
func NewReportBuilderRegistry(builders ...ReportBuilder) (*mutableReportBuilderRegistry, error) {
	registry := &mutableReportBuilderRegistry{
		items:          make(map[reportBuilderKey]ReportBuilder),
		mechanismItems: make(map[MechanismReportBuilderKey]ReportBuilder),
	}
	for _, builder := range builders {
		if err := registry.Register(builder); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *mutableReportBuilderRegistry) Register(builder ReportBuilder) error {
	if builder == nil {
		return fmt.Errorf("interpretation report builder is nil")
	}
	keyed, ok := builder.(MechanismKeyedReportBuilder)
	if !ok {
		return fmt.Errorf("interpretation report builder must implement MechanismKeyedReportBuilder")
	}
	reportType := builder.ReportType()
	if reportType == "" {
		return fmt.Errorf("interpretation report builder report type is empty")
	}
	mechanismKeys := []MechanismReportBuilderKey{keyed.MechanismKey()}
	if multi, ok := builder.(MultiMechanismKeyedReportBuilder); ok {
		mechanismKeys = multi.MechanismKeys()
	}
	for _, mechanismKey := range mechanismKeys {
		if mechanismKey.ReportType == "" {
			mechanismKey.ReportType = reportType
		}
		if _, exists := r.mechanismItems[mechanismKey]; exists {
			return fmt.Errorf("interpretation report builder already registered for mechanism %s", mechanismKey)
		}
		r.mechanismItems[mechanismKey] = builder
	}
	if id := builder.ExecutionIdentity(); !id.IsZero() {
		registryKey := reportBuilderKey{key: id, reportType: reportType}
		if _, exists := r.items[registryKey]; exists {
			return fmt.Errorf("interpretation report builder already registered for identity %s report type %s", id, reportType)
		}
		r.items[registryKey] = builder
	}
	return nil
}

func (r *mutableReportBuilderRegistry) Resolve(key evaluation.ExecutionIdentity, reportType domainReport.ReportType) (ReportBuilder, error) {
	if r == nil {
		return nil, fmt.Errorf("interpretation report builder registry is not configured")
	}
	if reportType == "" {
		reportType = domainReport.ReportTypeStandard
	}
	builder, err := r.resolveReportBuilder(key, reportType)
	if err != nil {
		return nil, fmt.Errorf("unsupported interpretation report builder key: %s report type: %s", key, reportType)
	}
	return builder, nil
}

func (r *mutableReportBuilderRegistry) ResolveByMechanism(key MechanismReportBuilderKey) (ReportBuilder, error) {
	if r == nil {
		return nil, fmt.Errorf("interpretation report builder registry is not configured")
	}
	if key.ReportType == "" {
		key.ReportType = domainReport.ReportTypeStandard
	}
	if builder, ok := r.mechanismItems[key]; ok {
		return builder, nil
	}
	familyKey := MechanismReportBuilderKey{
		AlgorithmFamily: key.AlgorithmFamily,
		ReportType:      key.ReportType,
	}
	if builder, ok := r.mechanismItems[familyKey]; ok {
		return builder, nil
	}
	return nil, fmt.Errorf("unsupported interpretation report builder mechanism: %s", key)
}
