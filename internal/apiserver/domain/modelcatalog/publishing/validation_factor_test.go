package publishing_test

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestValidateForPublishRejectsInvalidFactorHierarchy(t *testing.T) {
	t.Parallel()

	model, err := modelcatalog.NewAssessmentModel(modelcatalog.NewAssessmentModelInput{
		Code:  "BR-HIER",
		Kind:  modelcatalog.KindBehavioralRating,
		Title: "层级校验",
		Now:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	if err := model.UpdateDefinition(modelcatalog.DefinitionPayload{
		Data: []byte(`{
			"dimensions": [{
				"code": "bri",
				"title": "BRI",
				"role": "index",
				"parent_code": "gec"
			}]
		}`),
	}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	_ = model.BindQuestionnaire(modelcatalog.QuestionnaireBinding{
		QuestionnaireCode:    "Q-001",
		QuestionnaireVersion: "1.0.0",
	}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	result := model.ValidateForPublish()
	if result.Passed() {
		t.Fatal("ValidateForPublish() should reject invalid hierarchy")
	}
}
