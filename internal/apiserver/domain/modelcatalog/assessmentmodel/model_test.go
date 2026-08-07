package assessmentmodel_test

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

func TestAssessmentModelLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	model, err := assessmentmodel.New(assessmentmodel.NewInput{
		Code:      "personality_mbti_v1",
		Kind:      binding.KindTypology,
		Algorithm: binding.AlgorithmPersonalityTypology,
		Title:     "MBTI",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !model.IsDraft() {
		t.Fatalf("status = %s, want draft", model.Status)
	}
	if model.Revision() != 1 {
		t.Fatalf("revision = %d, want 1", model.Revision())
	}

	if err := model.BindQuestionnaire(binding.QuestionnaireBinding{
		QuestionnaireCode:    "q_mbti",
		QuestionnaireVersion: "v1",
	}, now); err != nil {
		t.Fatalf("BindQuestionnaire() error = %v", err)
	}
	if err := model.UpdateDefinition(&definition.Definition{}, now); err != nil {
		t.Fatalf("UpdateDefinition() error = %v", err)
	}
	if result := model.ValidateForPublish(); !result.Passed() {
		t.Fatalf("ValidateForPublish() issues = %#v", result.Issues)
	}
	if err := model.MarkPublished(now); err != nil {
		t.Fatalf("MarkPublished() error = %v", err)
	}
	if !model.IsPublished() {
		t.Fatalf("status = %s, want published", model.Status)
	}
	if err := model.MarkUnpublished(now); err != nil {
		t.Fatalf("MarkUnpublished() error = %v", err)
	}
	if !model.IsDraft() {
		t.Fatalf("status = %s, want draft after unpublish", model.Status)
	}
}

func TestAssessmentModelRejectsMissingDefinitionV2(t *testing.T) {
	t.Parallel()

	model, err := assessmentmodel.New(assessmentmodel.NewInput{
		Code:      "personality_legacy_payload",
		Kind:      binding.KindTypology,
		Algorithm: binding.AlgorithmPersonalityTypology,
		Title:     "Legacy Payload",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = model.BindQuestionnaire(binding.QuestionnaireBinding{
		QuestionnaireCode:    "q_mbti",
		QuestionnaireVersion: "v1",
	}, time.Now())
	result := model.ValidateForPublish()
	if result.Passed() {
		t.Fatal("ValidateForPublish() should reject missing DefinitionV2")
	}
	if result.Issues[0].Code != "definition_v2.required" {
		t.Fatalf("issue = %#v", result.Issues[0])
	}
}

func TestScalePublishRequiresCanonicalCategory(t *testing.T) {
	t.Parallel()

	model, err := assessmentmodel.New(assessmentmodel.NewInput{
		Code: "SCALE_A", Kind: binding.KindScale, Algorithm: binding.AlgorithmScaleDefault, Title: "Scale A",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = model.BindQuestionnaire(binding.QuestionnaireBinding{QuestionnaireCode: "Q_A", QuestionnaireVersion: "v1"}, time.Now())
	_ = model.UpdateDefinition(&definition.Definition{}, time.Now())
	result := model.ValidateForPublish()
	if result.Passed() {
		t.Fatal("ValidateForPublish() should reject an empty scale category")
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "category.required" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ValidateForPublish() issues = %#v, want category.required", result.Issues)
	}

	if err := model.UpdateScaleBasicInfo("Scale A", "", binding.AlgorithmScaleDefault, assessmentmodel.ScaleCategoryADHD, nil, nil, nil, nil, time.Now()); err != nil {
		t.Fatalf("UpdateScaleBasicInfo() error = %v", err)
	}
	if result := model.ValidateForPublish(); !result.Passed() {
		t.Fatalf("ValidateForPublish() issues = %#v", result.Issues)
	}
}

func TestScaleMetadataRejectsCompatibilityCategory(t *testing.T) {
	t.Parallel()

	if _, err := assessmentmodel.New(assessmentmodel.NewInput{
		Code: "SCALE_PERSONALITY", Kind: binding.KindScale, Algorithm: binding.AlgorithmScaleDefault, Title: "Scale", Category: "personality",
	}); err == nil {
		t.Fatal("New() error = nil, want legacy personality category rejected for scale")
	}
}
