package patterns_test

import (
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
)

func TestPersonalityTypeTemplateForSpec_EmptyUsesGenericTemplate(t *testing.T) {
	t.Parallel()

	tmpl, err := patterns.PersonalityTypeTemplateForSpec(patterns.ReportSpec{AdapterKey: patterns.ReportAdapterPersonalityType})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if tmpl.Kind != "" {
		t.Fatalf("Kind = %q, want generic", tmpl.Kind)
	}
}

func TestPersonalityTypeTemplateForSpec_KnownTemplateID(t *testing.T) {
	t.Parallel()

	tmpl, err := patterns.PersonalityTypeTemplateForSpec(patterns.ReportSpec{TemplateID: "sbti"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if tmpl.Kind != string(patterns.ReportAdapterPersonalityType) {
		t.Fatalf("Kind = %q, want %q", tmpl.Kind, patterns.ReportAdapterPersonalityType)
	}
}

func TestPersonalityTypeTemplateForSpec_UnknownTemplateID(t *testing.T) {
	t.Parallel()

	_, err := patterns.PersonalityTypeTemplateForSpec(patterns.ReportSpec{TemplateID: "not-registered", AdapterKey: patterns.ReportAdapterPersonalityType})
	if !errors.Is(err, patterns.ErrUnknownTemplateID) {
		t.Fatalf("err = %v, want ErrUnknownTemplateID", err)
	}
}

func TestTraitProfileTemplateForSpec_UnknownTemplateID(t *testing.T) {
	t.Parallel()

	_, err := patterns.TraitProfileTemplateForSpec(patterns.ReportSpec{TemplateID: "mbti", AdapterKey: patterns.ReportAdapterTraitProfile})
	if !errors.Is(err, patterns.ErrUnknownTemplateID) {
		t.Fatalf("err = %v, want ErrUnknownTemplateID", err)
	}
}

func TestIsRegisteredTemplateID(t *testing.T) {
	t.Parallel()

	if !patterns.IsRegisteredTemplateID("mbti") || !patterns.IsRegisteredTemplateID("bigfive") {
		t.Fatal("known TemplateIDs must be registered")
	}
	if patterns.IsRegisteredTemplateID("") || patterns.IsRegisteredTemplateID("nope") {
		t.Fatal("empty/unknown TemplateID must not be registered")
	}
}
