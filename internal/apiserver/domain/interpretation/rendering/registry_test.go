package rendering

import (
	"context"
	"testing"

	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type registryBuilder struct {
	key  Key
	keys []Key
	name string
}

func (b registryBuilder) ReportType() policy.ReportType { return policy.ReportTypeStandard }
func (b registryBuilder) TemplateVersion() policy.TemplateVersion {
	return policy.TemplateVersionV1
}
func (b registryBuilder) BuilderIdentity() string    { return b.name }
func (registryBuilder) ContentSchemaVersion() string { return "report-content/v1" }
func (b registryBuilder) MechanismKey() Key          { return b.key }
func (b registryBuilder) MechanismKeys() []Key       { return b.keys }
func (registryBuilder) Build(context.Context, interpinput.InterpretationInput) (*report.Draft, error) {
	return report.NewDraft(report.Content{}), nil
}

func registryKey(decision modelcatalog.DecisionKind) Key {
	return Key{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
		DecisionKind:    decision,
		ReportType:      policy.ReportTypeStandard,
		TemplateVersion: policy.TemplateVersionV1,
		Algorithm:       modelcatalog.AlgorithmMBTI,
		ProductChannel:  modelcatalog.ProductChannelTypology,
		ReportProfile:   policy.ReportProfilePersonalityType,
	}
}

func TestRegistryResolvesCompleteKeyAndFallbackWithinTemplateVersion(t *testing.T) {
	key := registryKey(modelcatalog.DecisionKindPoleComposition)
	builder := registryBuilder{key: key, keys: []Key{key}, name: "complete"}
	registry, err := NewRegistry(builder)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := registry.ResolveByMechanism(key); err != nil || got.BuilderIdentity() != "complete" {
		t.Fatalf("complete key resolve = %v, %v", got, err)
	}

	fallbackKey := key
	fallbackKey.ReportProfile = ""
	fallback := registryBuilder{key: fallbackKey, keys: []Key{fallbackKey}, name: "fallback"}
	registry, err = NewRegistry(fallback)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := registry.ResolveByMechanism(key); err != nil || got.BuilderIdentity() != "fallback" {
		t.Fatalf("fallback resolve = %v, %v", got, err)
	}
	key.TemplateVersion = "v2"
	if _, err := registry.ResolveByMechanism(key); err == nil {
		t.Fatal("fallback crossed template version")
	}
}

func TestRegistryRejectsDuplicateKeyAndSupportsMultiKey(t *testing.T) {
	first := registryKey(modelcatalog.DecisionKindPoleComposition)
	second := registryKey(modelcatalog.DecisionKindTraitProfile)
	multi := registryBuilder{key: first, keys: []Key{first, second}, name: "multi"}
	registry, err := NewRegistry(multi)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := registry.ResolveByMechanism(second); err != nil || got.BuilderIdentity() != "multi" {
		t.Fatalf("multi key resolve = %v, %v", got, err)
	}
	if _, err := NewRegistry(multi, registryBuilder{key: first, keys: []Key{first}, name: "duplicate"}); err == nil {
		t.Fatal("duplicate key registration succeeded")
	}
}
