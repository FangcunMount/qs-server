package modelcatalog

import "testing"

func TestLegacyScaleBindingRoundTrip(t *testing.T) {
	t.Parallel()
	model := &PublishedModel{}
	SetLegacyScaleBinding(model, LegacyScaleBinding{MedicalScaleID: 8, ScaleVersion: "1.0.0"})
	got, ok := LegacyScaleBindingFromPublished(model)
	if !ok || got.MedicalScaleID != 8 || got.ScaleVersion != "1.0.0" {
		t.Fatalf("LegacyScaleBindingFromPublished() = %#v, %v", got, ok)
	}
}

func TestLegacyScaleBindingAcceptsMongoNumericValues(t *testing.T) {
	t.Parallel()
	model := &PublishedModel{Source: map[string]any{
		legacyScaleBindingSourceKey: map[string]any{"medical_scale_id": int64(9), "scale_version": "1.0.1"},
	}}
	got, ok := LegacyScaleBindingFromPublished(model)
	if !ok || got.MedicalScaleID != 9 || got.ScaleVersion != "1.0.1" {
		t.Fatalf("LegacyScaleBindingFromPublished() = %#v, %v", got, ok)
	}
}
