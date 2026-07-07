package modelcatalog

import "testing"

func TestBehaviorAbilityRejectsNewAssessmentModel(t *testing.T) {
	t.Parallel()

	_, err := NewAssessmentModel(NewAssessmentModelInput{
		Code:  "BA-001",
		Kind:  KindBehaviorAbility,
		Title: "legacy channel",
	})
	if err == nil {
		t.Fatal("NewAssessmentModel(behavior_ability) error = nil, want rejection")
	}
}

func TestBehavioralRatingAllowsNewAssessmentModel(t *testing.T) {
	t.Parallel()

	model, err := NewAssessmentModel(NewAssessmentModelInput{
		Code:      "BR-001",
		Kind:      KindBehavioralRating,
		Algorithm: AlgorithmBrief2,
		Title:     "BRIEF-2",
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel(behavioral_rating) error = %v", err)
	}
	if model.Kind != KindBehavioralRating || model.Algorithm != AlgorithmBrief2 {
		t.Fatalf("model = %#v", model)
	}
}
