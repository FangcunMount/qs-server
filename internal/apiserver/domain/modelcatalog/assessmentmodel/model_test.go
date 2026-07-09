package assessmentmodel_test

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

func TestAssessmentModelLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	model, err := assessmentmodel.New(assessmentmodel.NewInput{
		Code:      "personality_mbti_v1",
		Kind:      binding.KindTypology,
		SubKind:   binding.SubKindTypology,
		Algorithm: binding.AlgorithmMBTI,
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
	if err := model.UpdateDefinition(assessmentmodel.DefinitionPayload{
		Format: "assessmentmodel.personality.typology.v1",
		Data:   []byte(`{"algorithm":"mbti"}`),
	}, now); err != nil {
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

func TestAssessmentModelRejectsLegacyDecodeOnlyDefinitionFormat(t *testing.T) {
	t.Parallel()

	model, err := assessmentmodel.New(assessmentmodel.NewInput{
		Code:      "personality_legacy_payload",
		Kind:      binding.KindTypology,
		SubKind:   binding.SubKindTypology,
		Algorithm: binding.AlgorithmMBTI,
		Title:     "Legacy Payload",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = model.BindQuestionnaire(binding.QuestionnaireBinding{
		QuestionnaireCode:    "q_mbti",
		QuestionnaireVersion: "v1",
	}, time.Now())
	if err := model.UpdateDefinition(assessmentmodel.DefinitionPayload{
		Format: "ruleset.mbti.v1",
		Data:   []byte(`{"algorithm":"mbti"}`),
	}, time.Now()); err != nil {
		t.Fatalf("UpdateDefinition() error = %v", err)
	}

	result := model.ValidateForPublish()
	if result.Passed() {
		t.Fatal("ValidateForPublish() should reject legacy-decode-only payload format")
	}
	if result.Issues[0].Code != "definition.format.legacy_decode_only" {
		t.Fatalf("issue = %#v", result.Issues[0])
	}
}
