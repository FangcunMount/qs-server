package modelcatalog

import "testing"

func TestNewAssessmentModelSetsDefaultProductChannel(t *testing.T) {
	model, err := NewAssessmentModel(NewAssessmentModelInput{
		Code:      "cog_demo",
		Kind:      KindCognitive,
		Algorithm: AlgorithmSPM,
		Title:     "SPM",
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	if model.ProductChannel != ProductChannelCognitive {
		t.Fatalf("ProductChannel = %q, want cognitive", model.ProductChannel)
	}
}

func TestNewAssessmentModelRejectsInvalidProductChannel(t *testing.T) {
	_, err := NewAssessmentModel(NewAssessmentModelInput{
		Code:           "bad",
		Kind:           KindCognitive,
		Algorithm:      AlgorithmSPM,
		ProductChannel: ProductChannel("invalid"),
		Title:          "bad",
	})
	if err == nil {
		t.Fatal("expected invalid product channel error")
	}
}
