// Package rendering owns Interpretation report rendering mechanisms and their
// template-version-aware resolution.
package rendering

import (
	"context"
	"fmt"

	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type Builder interface {
	ReportType() policy.ReportType
	TemplateVersion() policy.TemplateVersion
	BuilderIdentity() string
	ContentSchemaVersion() string
	Build(ctx context.Context, input interpinput.InterpretationInput) (*report.Draft, error)
}

type Key struct {
	DecisionKind    modelcatalog.DecisionKind
	ReportType      policy.ReportType
	TemplateVersion policy.TemplateVersion
	Algorithm       modelcatalog.Algorithm
	ReportProfile   policy.ReportProfile
}

func (k Key) String() string {
	base := string(k.DecisionKind) + "/" + string(k.ReportType) + "/" + k.TemplateVersion.String()
	if k.Algorithm != "" {
		base += "/" + string(k.Algorithm)
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
	items map[builderIndexKey]Builder
}

// NewDefaultRegistry is the single production/preview composition for
// Interpretation builders. Callers may replace the output adapter around a
// resolved Builder, but must not bypass mechanism resolution.
func NewDefaultRegistry(composer report.DraftBuilder) (Registry, error) {
	return NewRegistry(DefaultBuilders(composer)...)
}

// builderIndexKey is an in-process index detail. AlgorithmFamily is deliberately
// derived from DecisionKind here rather than accepted from report input.
type builderIndexKey struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	Key
}

func NewRegistry(builders ...Builder) (Registry, error) {
	r := &registry{items: make(map[builderIndexKey]Builder, len(builders))}
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
		registryKey, err := toBuilderIndexKey(key)
		if err != nil {
			return err
		}
		if _, exists := r.items[registryKey]; exists {
			return fmt.Errorf("interpretation report builder already registered for mechanism %s", key)
		}
		r.items[registryKey] = builder
	}
	return nil
}

func (r *registry) ResolveByMechanism(key Key) (Builder, error) {
	if r == nil {
		return nil, fmt.Errorf("interpretation report builder registry is not configured")
	}
	if key.ReportType == "" {
		key.ReportType = policy.ReportTypeStandard
	}
	if key.TemplateVersion.IsEmpty() {
		key.TemplateVersion = policy.TemplateVersionV1
	}
	for _, candidate := range fallbackCandidates(key) {
		registryKey, err := toBuilderIndexKey(candidate)
		if err != nil {
			return nil, err
		}
		if builder, ok := r.items[registryKey]; ok {
			return builder, nil
		}
	}
	return nil, fmt.Errorf("unsupported interpretation report builder mechanism: %s", key)
}

type RoutingContext struct {
	DecisionKind    modelcatalog.DecisionKind
	ReportType      policy.ReportType
	TemplateVersion policy.TemplateVersion
	Algorithm       modelcatalog.Algorithm
	ReportProfile   policy.ReportProfile
}

func RoutingContextFromInput(input interpinput.InterpretationInput) (RoutingContext, bool) {
	value := RoutingContext{
		DecisionKind:    input.Runtime.DecisionKind,
		ReportType:      input.Report.ReportType,
		TemplateVersion: input.Report.TemplateVersion,
		Algorithm:       input.Report.Algorithm,
		ReportProfile:   input.Report.ReportProfile,
	}
	if value.ReportType == "" {
		value.ReportType = policy.ReportTypeStandard
	}
	if value.TemplateVersion.IsEmpty() {
		value.TemplateVersion = policy.TemplateVersionV1
	}
	_, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(value.DecisionKind)
	if !ok {
		return RoutingContext{}, false
	}
	if value.ReportProfile == "" {
		value.ReportProfile = policy.ReportProfileForDecisionKind(value.DecisionKind)
	}
	if value.DecisionKind == "" {
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

func fallbackCandidates(key Key) []Key {
	base := []Key{
		key,
		{DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm, ReportProfile: key.ReportProfile},
		{DecisionKind: key.DecisionKind, ReportType: key.ReportType, Algorithm: key.Algorithm},
		{DecisionKind: key.DecisionKind, ReportType: key.ReportType, ReportProfile: key.ReportProfile},
		{DecisionKind: key.DecisionKind, ReportType: key.ReportType},
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

func toBuilderIndexKey(key Key) (builderIndexKey, error) {
	family, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(key.DecisionKind)
	if !ok {
		return builderIndexKey{}, fmt.Errorf("unknown interpretation decision_kind: %s", key.DecisionKind)
	}
	return builderIndexKey{AlgorithmFamily: family, Key: key}, nil
}
