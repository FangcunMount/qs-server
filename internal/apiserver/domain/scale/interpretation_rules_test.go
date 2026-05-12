package scale

import "testing"

func TestNewInterpretationRulesSortsByMinAscending(t *testing.T) {
	t.Parallel()

	unsorted := []InterpretationRule{
		NewInterpretationRule(NewScoreRange(10, 20), RiskLevelMedium, "medium", "follow"),
		NewInterpretationRule(NewScoreRange(0, 10), RiskLevelLow, "low", "watch"),
		NewInterpretationRule(NewScoreRange(20, 30), RiskLevelHigh, "high", "act"),
	}

	rules, err := NewInterpretationRules(unsorted)
	if err != nil {
		t.Fatalf("NewInterpretationRules() error = %v", err)
	}

	items := rules.Items()
	if len(items) != 3 {
		t.Fatalf("len = %d, want 3", len(items))
	}
	wantMins := []float64{0, 10, 20}
	for i, rule := range items {
		if got := rule.GetScoreRange().Min(); got != wantMins[i] {
			t.Fatalf("items[%d].min = %v, want %v", i, got, wantMins[i])
		}
	}
}

func TestNewInterpretationRulesAllowsAdjacentRanges(t *testing.T) {
	t.Parallel()

	adjacent := []InterpretationRule{
		NewInterpretationRule(NewScoreRange(0, 10), RiskLevelLow, "low", ""),
		NewInterpretationRule(NewScoreRange(10, 20), RiskLevelMedium, "medium", ""),
	}
	if _, err := NewInterpretationRules(adjacent); err != nil {
		t.Fatalf("NewInterpretationRules() with adjacent ranges err = %v, want nil", err)
	}
}

func TestNewInterpretationRulesRejectsOverlap(t *testing.T) {
	t.Parallel()

	overlapping := []InterpretationRule{
		NewInterpretationRule(NewScoreRange(0, 10), RiskLevelLow, "low", ""),
		NewInterpretationRule(NewScoreRange(9.99, 20), RiskLevelMedium, "medium", ""),
	}
	if _, err := NewInterpretationRules(overlapping); err == nil {
		t.Fatal("NewInterpretationRules() with overlapping ranges err = nil, want non-nil")
	}
}

func TestNewInterpretationRulesEmptyAllowed(t *testing.T) {
	t.Parallel()

	rules, err := NewInterpretationRules(nil)
	if err != nil {
		t.Fatalf("NewInterpretationRules(nil) err = %v, want nil", err)
	}
	if rules.Len() != 0 {
		t.Fatalf("len = %d, want 0", rules.Len())
	}
}

func TestInterpretationRulesMatchUsesSortedOrder(t *testing.T) {
	t.Parallel()

	rules, err := NewInterpretationRules([]InterpretationRule{
		NewInterpretationRule(NewScoreRange(10, 20), RiskLevelMedium, "medium", ""),
		NewInterpretationRule(NewScoreRange(0, 10), RiskLevelLow, "low", ""),
	})
	if err != nil {
		t.Fatalf("NewInterpretationRules() err = %v", err)
	}

	rule, ok := rules.Match(5)
	if !ok || rule.GetRiskLevel() != RiskLevelLow {
		t.Fatalf("Match(5) = %v, ok=%v, want low", rule.GetRiskLevel(), ok)
	}
	rule, ok = rules.Match(15)
	if !ok || rule.GetRiskLevel() != RiskLevelMedium {
		t.Fatalf("Match(15) = %v, ok=%v, want medium", rule.GetRiskLevel(), ok)
	}
}
