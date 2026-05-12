package scale

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

func TestMedicalScaleAddUpdateRemoveFactor(t *testing.T) {
	t.Parallel()

	m := newTestMedicalScale(t)
	factor := newTestFactor(t, "F1")

	if err := m.AddFactor(factor); err != nil {
		t.Fatalf("AddFactor() error = %v", err)
	}
	if err := m.AddFactor(factor); err == nil {
		t.Fatal("expected duplicate factor error")
	}

	updated := newTestFactor(t, "F1", WithFactorType(FactorTypeMultilevel))
	if err := m.UpdateFactor(updated); err != nil {
		t.Fatalf("UpdateFactor() error = %v", err)
	}
	snapshot, ok := m.FindFactorSnapshotByCode(NewFactorCode("F1"))
	if !ok {
		t.Fatal("expected updated factor")
	}
	if snapshot.FactorType != FactorTypeMultilevel {
		t.Fatalf("factor type = %q, want %q", snapshot.FactorType, FactorTypeMultilevel)
	}

	if err := m.RemoveFactor(NewFactorCode("F1")); err != nil {
		t.Fatalf("RemoveFactor() error = %v", err)
	}
	if m.FactorCount() != 0 {
		t.Fatalf("factor count = %d, want 0", m.FactorCount())
	}
}

func TestMedicalScaleReplaceFactorsRejectsDuplicateAndMultipleTotalScore(t *testing.T) {
	t.Parallel()

	t.Run("duplicate factor code", func(t *testing.T) {
		t.Parallel()

		m := newTestMedicalScale(t)
		if err := m.ReplaceFactors([]*Factor{
			newTestFactor(t, "F1"),
			newTestFactor(t, "F1"),
		}); err == nil {
			t.Fatal("expected duplicate factor code error")
		}
	})

	t.Run("multiple total score factors", func(t *testing.T) {
		t.Parallel()

		m := newTestMedicalScale(t)
		if err := m.ReplaceFactors([]*Factor{
			newTestFactor(t, "TOTAL_1", WithIsTotalScore(true)),
			newTestFactor(t, "TOTAL_2", WithIsTotalScore(true)),
		}); err == nil {
			t.Fatal("expected multiple total score factors error")
		}
	})

	t.Run("valid replacement", func(t *testing.T) {
		t.Parallel()

		m := newTestMedicalScale(t)
		if err := m.ReplaceFactors([]*Factor{
			newTestFactor(t, "TOTAL", WithIsTotalScore(true)),
			newTestFactor(t, "F1"),
		}); err != nil {
			t.Fatalf("ReplaceFactors() error = %v", err)
		}
		if m.FactorCount() != 2 {
			t.Fatalf("factor count = %d, want 2", m.FactorCount())
		}
	})
}

func TestMedicalScaleUpdateFactorInterpretRulesValidatesRules(t *testing.T) {
	t.Parallel()

	m := newTestMedicalScale(t)
	if err := m.AddFactor(newTestFactor(t, "F1")); err != nil {
		t.Fatalf("AddFactor() error = %v", err)
	}

	validRule := NewInterpretationRule(NewScoreRange(0, 10), RiskLevelLow, "low", "watch")
	if err := m.UpdateFactorInterpretRules(NewFactorCode("F1"), []InterpretationRule{validRule}); err != nil {
		t.Fatalf("UpdateFactorInterpretRules() error = %v", err)
	}
	snapshot, ok := m.FindFactorSnapshotByCode(NewFactorCode("F1"))
	if !ok {
		t.Fatal("expected factor")
	}
	if len(snapshot.InterpretRules) != 1 {
		t.Fatalf("interpret rule count = %d, want 1", len(snapshot.InterpretRules))
	}
	if snapshot.InterpretRules[0].GetRiskLevel() != RiskLevelLow {
		t.Fatalf("risk level = %q, want %q", snapshot.InterpretRules[0].GetRiskLevel(), RiskLevelLow)
	}

	invalidRule := NewInterpretationRule(NewScoreRange(10, 0), RiskLevelLow, "invalid", "")
	if err := m.UpdateFactorInterpretRules(NewFactorCode("F1"), []InterpretationRule{invalidRule}); err == nil {
		t.Fatal("expected invalid interpretation rule error")
	}

	overlappingRules := []InterpretationRule{
		NewInterpretationRule(NewScoreRange(0, 10), RiskLevelLow, "low", "watch"),
		NewInterpretationRule(NewScoreRange(5, 12), RiskLevelMedium, "medium", "follow"),
	}
	if err := m.UpdateFactorInterpretRules(NewFactorCode("F1"), overlappingRules); err == nil {
		t.Fatal("expected overlapping interpretation rule error")
	}
}

func TestMedicalScaleEncapsulatesSlicesAndEvents(t *testing.T) {
	t.Parallel()

	stages := []Stage{StageDeepAssessment}
	tags := []Tag{NewTag("initial")}
	factor := newTestFactor(t, "F1")
	m, err := NewMedicalScale(
		meta.NewCode("SCALE_A"),
		"Scale A",
		WithStages(stages),
		WithTags(tags),
		WithFactors([]*Factor{factor}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}

	stages[0] = StageFollowUp
	tags[0] = NewTag("mutated")
	gotStages := m.GetStages()
	gotTags := m.GetTags()
	if gotStages[0] != StageDeepAssessment || gotTags[0].String() != "initial" {
		t.Fatalf("constructor reused slices: stages=%v tags=%v", gotStages, gotTags)
	}

	gotStages[0] = StageFollowUp
	gotTags[0] = NewTag("changed")
	gotFactors := m.GetFactors()
	gotFactors[0] = newTestFactor(t, "F2")
	if m.GetStages()[0] != StageDeepAssessment || m.GetTags()[0].String() != "initial" || m.GetFactors()[0].GetCode().String() != "F1" {
		t.Fatalf("getter exposed internal slices")
	}

	if err := m.AddFactor(newTestFactor(t, "F3")); err != nil {
		t.Fatalf("AddFactor() error = %v", err)
	}
	events := m.Events()
	events[0] = event.New("mutated", "MedicalScale", "1", map[string]string{})
	if got := m.Events()[0].EventType(); got != EventTypeChanged {
		t.Fatalf("stored event type = %q, want %s", got, EventTypeChanged)
	}
}

func TestFactorEncapsulatesScoringAndRules(t *testing.T) {
	t.Parallel()

	maxScore := 10.0
	questionCodes := []meta.Code{meta.NewCode("Q1")}
	cntContents := []string{"yes"}
	rules := []InterpretationRule{
		NewInterpretationRule(NewScoreRange(0, 5), RiskLevelLow, "low", "watch"),
	}
	factor := newTestFactor(t,
		"F1",
		WithQuestionCodes(questionCodes),
		WithScoringStrategy(ScoringStrategyCnt),
		WithScoringParams(NewScoringParams().WithCntOptionContents(cntContents)),
		WithMaxScore(&maxScore),
		WithInterpretRules(rules),
	)

	questionCodes[0] = meta.NewCode("Q2")
	cntContents[0] = "no"
	maxScore = 20
	rules[0] = NewInterpretationRule(NewScoreRange(5, 10), RiskLevelHigh, "high", "act")
	if factor.GetQuestionCodes()[0].String() != "Q1" ||
		factor.GetScoringParams().GetCntOptionContents()[0] != "yes" ||
		*factor.GetMaxScore() != 10 ||
		factor.GetInterpretRules()[0].GetRiskLevel() != RiskLevelLow {
		t.Fatalf("factor reused constructor inputs")
	}

	gotCodes := factor.GetQuestionCodes()
	gotParams := factor.GetScoringParams()
	gotMaxScore := factor.GetMaxScore()
	gotRules := factor.GetInterpretRules()
	gotCodes[0] = meta.NewCode("Q3")
	gotParams.WithCntOptionContents([]string{"changed"})
	*gotMaxScore = 30
	gotRules[0] = NewInterpretationRule(NewScoreRange(5, 10), RiskLevelHigh, "changed", "")
	if factor.GetQuestionCodes()[0].String() != "Q1" ||
		factor.GetScoringParams().GetCntOptionContents()[0] != "yes" ||
		*factor.GetMaxScore() != 10 ||
		factor.GetInterpretRules()[0].GetRiskLevel() != RiskLevelLow {
		t.Fatalf("factor getter exposed internal state")
	}
}

func TestNewFactorValidatesScoringSpecAndQuestionCodes(t *testing.T) {
	t.Parallel()

	if _, err := NewFactor(NewFactorCode("F1"), "Factor 1", WithQuestionCodes([]meta.Code{meta.NewCode("Q1")}), WithScoringStrategy(ScoringStrategyCnt)); err == nil {
		t.Fatal("expected cnt strategy without params error")
	}
	if _, err := NewFactor(NewFactorCode("F1"), "Factor 1", WithQuestionCodes([]meta.Code{meta.NewCode("Q1"), meta.NewCode("Q1")})); err == nil {
		t.Fatal("expected duplicate question code error")
	}
	if _, err := NewFactor(NewFactorCode("F1"), "Factor 1"); err == nil {
		t.Fatal("expected non-total factor question code error")
	}
}

func TestPublishedScaleFreezesRuleMutations(t *testing.T) {
	t.Parallel()

	m := newPublishedTestMedicalScale(t)

	if err := m.AddFactor(newTestFactor(t, "F_NEW")); err == nil || !isRuleFrozen(err) {
		t.Fatalf("AddFactor on published scale err = %v, want rule frozen", err)
	}
	if err := m.UpdateFactor(newTestFactor(t, "F1", WithFactorType(FactorTypeMultilevel))); err == nil || !isRuleFrozen(err) {
		t.Fatalf("UpdateFactor on published scale err = %v, want rule frozen", err)
	}
	if err := m.RemoveFactor(NewFactorCode("F1")); err == nil || !isRuleFrozen(err) {
		t.Fatalf("RemoveFactor on published scale err = %v, want rule frozen", err)
	}
	if err := m.RemoveAllFactors(); err == nil || !isRuleFrozen(err) {
		t.Fatalf("RemoveAllFactors on published scale err = %v, want rule frozen", err)
	}
	if err := m.ReplaceFactors([]*Factor{newTestFactor(t, "F1")}); err == nil || !isRuleFrozen(err) {
		t.Fatalf("ReplaceFactors on published scale err = %v, want rule frozen", err)
	}

	rule := NewInterpretationRule(NewScoreRange(0, 10), RiskLevelLow, "low", "watch")
	if err := m.UpdateFactorInterpretRules(NewFactorCode("F1"), []InterpretationRule{rule}); err == nil || !isRuleFrozen(err) {
		t.Fatalf("UpdateFactorInterpretRules on published scale err = %v, want rule frozen", err)
	}
	if err := m.AddFactorInterpretRule(NewFactorCode("F1"), rule); err == nil || !isRuleFrozen(err) {
		t.Fatalf("AddFactorInterpretRule on published scale err = %v, want rule frozen", err)
	}

	bi := BaseInfo{}
	if err := bi.UpdateQuestionnaire(m, meta.NewCode("Q-NEW"), "2.0"); err == nil || !isRuleFrozen(err) {
		t.Fatalf("BaseInfo.UpdateQuestionnaire on published scale err = %v, want rule frozen", err)
	}
}

func TestPublishedScaleAllowsDisplayInfoEdit(t *testing.T) {
	t.Parallel()

	m := newPublishedTestMedicalScale(t)

	bi := BaseInfo{}
	if err := bi.UpdateAll(m, "New Title", "New Description"); err != nil {
		t.Fatalf("BaseInfo.UpdateAll on published scale err = %v, want nil", err)
	}
	if got := m.GetTitle(); got != "New Title" {
		t.Fatalf("title = %q, want %q", got, "New Title")
	}
}

func TestArchivedScaleFreezesAllMutations(t *testing.T) {
	t.Parallel()

	m := newPublishedTestMedicalScale(t)
	lc := NewLifecycle()
	if err := lc.Archive(t.Context(), m); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}

	if err := m.AddFactor(newTestFactor(t, "F_NEW")); err == nil || !isRuleFrozen(err) {
		t.Fatalf("AddFactor on archived scale err = %v, want rule frozen", err)
	}
	bi := BaseInfo{}
	if err := bi.UpdateAll(m, "Should Fail", ""); err == nil || !isRuleFrozen(err) {
		t.Fatalf("BaseInfo.UpdateAll on archived scale err = %v, want rule frozen", err)
	}
}

func isRuleFrozen(err error) bool {
	kind, ok := ErrorKindOf(err)
	return ok && kind == ErrorKindRuleFrozen
}

func newPublishedTestMedicalScale(t *testing.T) *MedicalScale {
	t.Helper()

	totalFactor, err := NewFactor(
		NewFactorCode("TOTAL"),
		"Total",
		WithIsTotalScore(true),
		WithInterpretRules([]InterpretationRule{
			NewInterpretationRule(NewScoreRange(0, 10), RiskLevelLow, "low", "watch"),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor() error = %v", err)
	}
	dim := newTestFactor(t, "F1",
		WithInterpretRules([]InterpretationRule{
			NewInterpretationRule(NewScoreRange(0, 10), RiskLevelLow, "low", "watch"),
		}),
	)

	m, err := NewMedicalScale(
		meta.NewCode("SCALE_A"),
		"Scale A",
		WithQuestionnaire(meta.NewCode("Q1"), "1.0"),
		WithFactors([]*Factor{totalFactor, dim}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	lc := NewLifecycle()
	if err := lc.Publish(t.Context(), m); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	return m
}

func newTestMedicalScale(t *testing.T) *MedicalScale {
	t.Helper()

	m, err := NewMedicalScale(meta.NewCode("SCALE_A"), "Scale A")
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	return m
}

func newTestFactor(t *testing.T, code string, opts ...FactorOption) *Factor {
	t.Helper()

	opts = append([]FactorOption{WithQuestionCodes([]meta.Code{meta.NewCode("Q1")})}, opts...)
	factor, err := NewFactor(NewFactorCode(code), "Factor "+code, opts...)
	if err != nil {
		t.Fatalf("NewFactor() error = %v", err)
	}
	return factor
}
