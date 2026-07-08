package report

import "testing"

func TestFinalizeInterpretReportProjectsLegacySummary(t *testing.T) {
	r := NewInterpretReport(
		ID(1),
		"抑郁筛查",
		"PHQ9",
		8,
		RiskLevelLow,
		"轻度",
		nil,
		nil,
		nil,
	)
	if r.PrimaryScore() == nil || r.PrimaryScore().Value != 8 {
		t.Fatalf("PrimaryScore = %#v, want value 8", r.PrimaryScore())
	}
	if r.Level() == nil || r.Level().Code != "low" {
		t.Fatalf("Level = %#v, want low", r.Level())
	}
}

func TestAttachOutcomeSummaryPreservesV2Fields(t *testing.T) {
	r := NewInterpretReport(ID(2), "", "", 0, RiskLevelNone, "INTJ", nil, nil, nil)
	model := ModelIdentity{
		Kind:      "personality",
		SubKind:   "typology",
		Algorithm: "mbti",
		Code:      "MBTI_TEST",
		Title:     "MBTI",
	}
	primary := NewMatchPercentScore(40, "INTJ")
	level := &ResultLevel{Code: "INTJ", Label: "INTJ", Severity: "none"}

	got := AttachOutcomeSummary(r, model, primary, level)
	if got.Model().Algorithm != "mbti" {
		t.Fatalf("model = %#v", got.Model())
	}
	if got.PrimaryScore().Value != 40 {
		t.Fatalf("primary score = %#v", got.PrimaryScore())
	}
	if got.Level().Code != "INTJ" {
		t.Fatalf("level = %#v", got.Level())
	}
}

func TestIsHighRisk(t *testing.T) {
	t.Parallel()

	if !IsHighRisk(RiskLevelHigh) || !IsHighRisk(RiskLevelSevere) {
		t.Fatal("high/severe should be high risk")
	}
	if IsHighRisk(RiskLevelLow) || IsHighRisk(RiskLevelMedium) {
		t.Fatal("low/medium should not be high risk")
	}
}
