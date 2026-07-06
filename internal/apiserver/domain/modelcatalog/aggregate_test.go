package modelcatalog_test

import (
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestPersonalityModelLifecycle(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:      "personality_mbti_v1",
		Kind:      domain.KindPersonality,
		SubKind:   domain.SubKindTypology,
		Algorithm: domain.AlgorithmMBTI,
		Title:     "MBTI",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel() error = %v", err)
	}
	if !model.IsDraft() {
		t.Fatalf("status = %s, want draft", model.Status)
	}

	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{
		QuestionnaireCode:    "q_mbti",
		QuestionnaireVersion: "v1",
	}, now); err != nil {
		t.Fatalf("BindQuestionnaire() error = %v", err)
	}
	if err := model.UpdateDefinition(domain.DefinitionPayload{
		Format: domain.PayloadFormatPersonalityTypologyV1,
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
		t.Fatalf("MarkUnpublish() error = %v", err)
	}
	if !model.IsDraft() {
		t.Fatalf("status = %s, want draft after unpublish", model.Status)
	}
}

func TestPublishedModelCanBeRepublished(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:      "personality_republish",
		Kind:      domain.KindPersonality,
		SubKind:   domain.SubKindTypology,
		Algorithm: domain.AlgorithmMBTI,
		Title:     "Republish",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel() error = %v", err)
	}
	if err := model.MarkPublished(now); err != nil {
		t.Fatalf("first MarkPublished() error = %v", err)
	}
	firstVersion := model.Version
	later := now.Add(time.Hour)
	if err := model.MarkPublished(later); err != nil {
		t.Fatalf("second MarkPublished() error = %v", err)
	}
	if model.Version != firstVersion+1 {
		t.Fatalf("version = %d, want %d", model.Version, firstVersion+1)
	}
	if model.PublishedAt == nil || !model.PublishedAt.Equal(later) {
		t.Fatalf("published_at = %v, want %v", model.PublishedAt, later)
	}
}

func TestArchiveCannotEdit(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:  "personality_sbti_v1",
		Kind:  domain.KindPersonality,
		Title: "SBTI",
		Now:   now,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel() error = %v", err)
	}
	if err := model.MarkArchived(now); err != nil {
		t.Fatalf("MarkArchived() error = %v", err)
	}
	if err := model.UpdateBasicInfo("new title", "", "", "", "", nil, now); err == nil {
		t.Fatal("UpdateBasicInfo() on archived model should fail")
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{
		QuestionnaireCode: "q", QuestionnaireVersion: "v1",
	}, now); err == nil {
		t.Fatal("BindQuestionnaire() on archived model should fail")
	}
	if err := model.UpdateDefinition(domain.DefinitionPayload{Format: domain.PayloadFormatPersonalityTypologyV1, Data: []byte(`{}`)}, now); err == nil {
		t.Fatal("UpdateDefinition() on archived model should fail")
	}
}

func TestPersonalityDefinitionValidation(t *testing.T) {
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:  "personality_missing_algo",
		Kind:  domain.KindPersonality,
		Title: "Missing algorithm",
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel() error = %v", err)
	}
	result := model.ValidateForPublish()
	if result.Passed() {
		t.Fatal("ValidateForPublish() should fail for incomplete personality model")
	}
}
