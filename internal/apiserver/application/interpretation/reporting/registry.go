package reporting

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type reportBuilderKey struct {
	key        evaluation.EvaluatorKey
	reportType domainReport.ReportType
}

// ReportBuilderRegistry resolves report builders by evaluator key and report type.
type ReportBuilderRegistry interface {
	Resolve(key evaluation.EvaluatorKey, reportType domainReport.ReportType) (ReportBuilder, error)
	ResolveByMechanism(key MechanismReportBuilderKey) (ReportBuilder, error)
}

type mutableReportBuilderRegistry struct {
	items          map[reportBuilderKey]ReportBuilder
	mechanismItems map[MechanismReportBuilderKey]ReportBuilder
}

// NewReportBuilderRegistry creates a registry from the given builders.
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
	key := builder.Key()
	if key.IsZero() {
		return fmt.Errorf("interpretation report builder key is empty")
	}
	reportType := builder.ReportType()
	if reportType == "" {
		return fmt.Errorf("interpretation report builder report type is empty")
	}
	registryKey := reportBuilderKey{key: key, reportType: reportType}
	if _, exists := r.items[registryKey]; exists {
		return fmt.Errorf("interpretation report builder already registered for key %s report type %s", key, reportType)
	}
	r.items[registryKey] = builder
	if keyed, ok := builder.(MechanismKeyedReportBuilder); ok {
		mechanismKey := keyed.MechanismKey()
		if mechanismKey.ReportType == "" {
			mechanismKey.ReportType = reportType
		}
		if _, exists := r.mechanismItems[mechanismKey]; exists {
			return fmt.Errorf("interpretation report builder already registered for mechanism %s", mechanismKey)
		}
		r.mechanismItems[mechanismKey] = builder
	}
	return nil
}

func (r *mutableReportBuilderRegistry) Resolve(key evaluation.EvaluatorKey, reportType domainReport.ReportType) (ReportBuilder, error) {
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

func (r *mutableReportBuilderRegistry) resolveByEvaluatorKey(key evaluation.EvaluatorKey, reportType domainReport.ReportType) (ReportBuilder, error) {
	registryKey := reportBuilderKey{key: key, reportType: reportType}
	if builder, ok := r.items[registryKey]; ok {
		return builder, nil
	}
	if routed := evaluation.ResolvePersonalityTypologyExecutorKey(key); routed != key {
		registryKey.key = routed
		if builder, ok := r.items[registryKey]; ok {
			return builder, nil
		}
	}
	if routed := evaluation.ResolveBehavioralRatingExecutorKey(key); routed != key {
		registryKey.key = routed
		if builder, ok := r.items[registryKey]; ok {
			return builder, nil
		}
	}
	if mappedKind, subKind, algorithm, ok := modelcatalog.LegacyKindMapping(key.Kind); ok {
		registryKey.key = evaluation.EvaluatorKey{Kind: mappedKind, SubKind: subKind, Algorithm: algorithm}
		if builder, ok := r.items[registryKey]; ok {
			return builder, nil
		}
	}
	return nil, fmt.Errorf("unsupported interpretation report builder key: %s report type: %s", key, reportType)
}
