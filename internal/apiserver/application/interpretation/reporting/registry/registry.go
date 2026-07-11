package registry

import (
	"fmt"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
)

// ReportBuilderRegistry resolves builders by Interpretation-owned mechanism identity.
type ReportBuilderRegistry interface {
	ResolveByMechanism(key MechanismReportBuilderKey) (ReportBuilder, error)
}

type mutableReportBuilderRegistry struct {
	mechanismItems map[MechanismReportBuilderKey]ReportBuilder
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
	templateVersion := builder.TemplateVersion()
	if templateVersion.IsEmpty() {
		return fmt.Errorf("interpretation report builder template version is empty")
	}
	if builder.BuilderIdentity() == "" || builder.ContentSchemaVersion() == "" {
		return fmt.Errorf("interpretation report builder identity and content schema version are required")
	}
	mechanismKeys := []MechanismReportBuilderKey{keyed.MechanismKey()}
	if multi, ok := builder.(MultiMechanismKeyedReportBuilder); ok {
		mechanismKeys = multi.MechanismKeys()
	}
	for _, mechanismKey := range mechanismKeys {
		if mechanismKey.ReportType == "" {
			mechanismKey.ReportType = reportType
		}
		if mechanismKey.TemplateVersion.IsEmpty() {
			mechanismKey.TemplateVersion = templateVersion
		}
		if mechanismKey.TemplateVersion != templateVersion {
			return fmt.Errorf("interpretation report builder mechanism template version does not match builder: %s", mechanismKey)
		}
		if _, exists := r.mechanismItems[mechanismKey]; exists {
			return fmt.Errorf("interpretation report builder already registered for mechanism %s", mechanismKey)
		}
		r.mechanismItems[mechanismKey] = builder
	}
	return nil
}

func (r *mutableReportBuilderRegistry) ResolveByMechanism(key MechanismReportBuilderKey) (ReportBuilder, error) {
	if r == nil {
		return nil, fmt.Errorf("interpretation report builder registry is not configured")
	}
	if key.ReportType == "" {
		key.ReportType = domainReport.ReportTypeStandard
	}
	if key.TemplateVersion.IsEmpty() {
		key.TemplateVersion = policy.TemplateVersionV1
	}
	candidates := MechanismKeyFallbackCandidates(key)
	for _, candidate := range candidates {
		if builder, ok := r.mechanismItems[candidate]; ok {
			return builder, nil
		}
	}
	return nil, fmt.Errorf("unsupported interpretation report builder mechanism: %s", key)
}
