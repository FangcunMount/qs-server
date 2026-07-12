// Package rendering owns Interpretation report rendering mechanisms and their
// template-version-aware resolution.
package rendering

import (
	"context"
	"fmt"

	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type Builder interface {
	ReportType() domaininterpretation.ReportType
	TemplateVersion() policy.TemplateVersion
	BuilderIdentity() string
	ContentSchemaVersion() string
	Build(ctx context.Context, input interpinput.InterpretationInput) (*report.Draft, error)
}

type Key struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	ReportType      domaininterpretation.ReportType
	TemplateVersion policy.TemplateVersion
	Algorithm       modelcatalog.Algorithm
	ProductChannel  modelcatalog.ProductChannel
	ReportProfile   policy.ReportProfile
}

func (k Key) String() string {
	base := k.AlgorithmFamily.String() + "/" + string(k.DecisionKind) + "/" + string(k.ReportType) + "/" + k.TemplateVersion.String()
	if k.Algorithm != "" {
		base += "/" + string(k.Algorithm)
	}
	if k.ProductChannel != "" {
		base += "/" + string(k.ProductChannel)
	}
	if k.ReportProfile != "" {
		base += "/" + string(k.ReportProfile)
	}
	return base
}

type KeyedBuilder interface {
	Builder
	MechanismKey() Key
}

type MultiKeyedBuilder interface {
	KeyedBuilder
	MechanismKeys() []Key
}

type Registry interface {
	ResolveByMechanism(key Key) (Builder, error)
}

type registry struct {
	items map[Key]Builder
}

func NewRegistry(builders ...Builder) (Registry, error) {
	r := &registry{items: make(map[Key]Builder, len(builders))}
	for _, builder := range builders {
		if err := r.register(builder); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *registry) register(builder Builder) error {
	if builder == nil {
		return fmt.Errorf("interpretation report builder is nil")
	}
	keyed, ok := builder.(KeyedBuilder)
	if !ok {
		return fmt.Errorf("interpretation report builder must expose a rendering key")
	}
	if builder.ReportType() == "" || builder.TemplateVersion().IsEmpty() {
		return fmt.Errorf("interpretation report builder report type and template version are required")
	}
	if builder.BuilderIdentity() == "" || builder.ContentSchemaVersion() == "" {
		return fmt.Errorf("interpretation report builder identity and content schema version are required")
	}
	keys := []Key{keyed.MechanismKey()}
	if multi, ok := builder.(MultiKeyedBuilder); ok {
		keys = multi.MechanismKeys()
	}
	for _, key := range keys {
		if key.ReportType == "" {
			key.ReportType = builder.ReportType()
		}
		if key.TemplateVersion.IsEmpty() {
			key.TemplateVersion = builder.TemplateVersion()
		}
		if key.TemplateVersion != builder.TemplateVersion() {
			return fmt.Errorf("interpretation report builder template version mismatch: %s", key)
		}
		if _, exists := r.items[key]; exists {
			return fmt.Errorf("interpretation report builder already registered for mechanism %s", key)
		}
		r.items[key] = builder
	}
	return nil
}

func (r *registry) ResolveByMechanism(key Key) (Builder, error) {
	if r == nil {
		return nil, fmt.Errorf("interpretation report builder registry is not configured")
	}
	if key.ReportType == "" {
		key.ReportType = domaininterpretation.ReportTypeStandard
	}
	if key.TemplateVersion.IsEmpty() {
		key.TemplateVersion = policy.TemplateVersionV1
	}
	for _, candidate := range fallbackCandidates(key) {
		if builder, ok := r.items[candidate]; ok {
			return builder, nil
		}
	}
	return nil, fmt.Errorf("unsupported interpretation report builder mechanism: %s", key)
}

type RoutingContext struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	ReportType      domaininterpretation.ReportType
	TemplateVersion policy.TemplateVersion
	Algorithm       modelcatalog.Algorithm
	ProductChannel  modelcatalog.ProductChannel
	ReportProfile   policy.ReportProfile
}

func RoutingContextFromInput(input interpinput.InterpretationInput) (RoutingContext, bool) {
	value := RoutingContext{
		AlgorithmFamily: input.Runtime.AlgorithmFamily,
		DecisionKind:    input.Runtime.DecisionKind,
		ReportType:      input.Report.ReportType,
		TemplateVersion: input.Report.TemplateVersion,
		Algorithm:       input.Report.Algorithm,
		ProductChannel:  input.Report.ProductChannel,
		ReportProfile:   input.Report.ReportProfile,
	}
	if value.ReportType == "" {
		value.ReportType = domaininterpretation.ReportTypeStandard
	}
	if value.TemplateVersion.IsEmpty() {
		value.TemplateVersion = policy.TemplateVersionV1
	}
	if value.DecisionKind == "" {
		value.DecisionKind = defaultDecisionKind(value.AlgorithmFamily)
	}
	if value.ReportProfile == "" {
		value.ReportProfile = policy.ReportProfileForDecisionKind(value.DecisionKind)
	}
	if value.AlgorithmFamily == "" || value.DecisionKind == "" {
		return RoutingContext{}, false
	}
	return value, true
}

func KeyFromInput(input interpinput.InterpretationInput) (Key, bool) {
	value, ok := RoutingContextFromInput(input)
	if !ok {
		return Key{}, false
	}
	return Key(value), true
}

func defaultDecisionKind(family modelcatalog.AlgorithmFamily) modelcatalog.DecisionKind {
	switch family {
	case modelcatalog.AlgorithmFamilyFactorScoring:
		return modelcatalog.DecisionKindScoreRange
	case modelcatalog.AlgorithmFamilyFactorClassification:
		return modelcatalog.DecisionKindPoleComposition
	case modelcatalog.AlgorithmFamilyFactorNorm:
		return modelcatalog.DecisionKindNormLookup
	case modelcatalog.AlgorithmFamilyTaskPerformance:
		return modelcatalog.DecisionKindAbilityLevel
	default:
		return ""
	}
}

func fallbackCandidates(key Key) []Key {
	base := []Key{
		key,
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm, ProductChannel: key.ProductChannel, ReportProfile: key.ReportProfile},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm, ProductChannel: key.ProductChannel},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm, ReportProfile: key.ReportProfile},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, ProductChannel: key.ProductChannel, ReportProfile: key.ReportProfile},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, ProductChannel: key.ProductChannel},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType, ReportProfile: key.ReportProfile},
		{AlgorithmFamily: key.AlgorithmFamily, DecisionKind: key.DecisionKind, ReportType: key.ReportType},
		{AlgorithmFamily: key.AlgorithmFamily, ReportType: key.ReportType},
	}
	out := make([]Key, 0, len(base))
	seen := make(map[Key]struct{}, len(base))
	for _, candidate := range base {
		candidate.TemplateVersion = key.TemplateVersion
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}
