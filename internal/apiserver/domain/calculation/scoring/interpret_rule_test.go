package scoring

import "testing"

func TestFindInterpretRuleUsesLeftClosedRightOpenIntervals(t *testing.T) {
	factor := Factor{
		InterpretRules: []InterpretRule{
			{Min: 0, Max: 10, RiskLevel: string(RiskLevelLow)},
			{Min: 10, Max: 100, RiskLevel: string(RiskLevelSevere)},
		},
	}

	got := findInterpretRule(factor, 9.9)
	if got == nil || got.RiskLevel != string(RiskLevelLow) {
		t.Fatalf("score 9.9 = %#v, want low on [0,10)", got)
	}

	got = findInterpretRule(factor, 10)
	if got == nil || got.RiskLevel != string(RiskLevelSevere) {
		t.Fatalf("score 10 = %#v, want severe on [10,100)", got)
	}

	// Legacy snapshots without endpoint flags treat the last range as max-inclusive.
	got = findInterpretRule(factor, 100)
	if got == nil || got.RiskLevel != string(RiskLevelSevere) {
		t.Fatalf("score 100 = %#v, want legacy last-inclusive severe", got)
	}
}

func TestFindInterpretRuleExplicitMaxInclusive(t *testing.T) {
	factor := Factor{
		InterpretRules: []InterpretRule{
			{Min: 0, Max: 10, RiskLevel: string(RiskLevelLow)},
			{Min: 10, Max: 100, RiskLevel: string(RiskLevelSevere), MaxInclusive: true},
		},
	}
	got := findInterpretRule(factor, 100)
	if got == nil || got.RiskLevel != string(RiskLevelSevere) {
		t.Fatalf("score 100 = %#v", got)
	}
	got = findInterpretRule(factor, 100.1)
	if got != nil {
		t.Fatalf("score 100.1 = %#v, want no match", got)
	}
}

func TestFindInterpretRuleBoundaryGolden(t *testing.T) {
	t.Parallel()

	factor := Factor{InterpretRules: []InterpretRule{
		{Min: 0, Max: 40, RiskLevel: string(RiskLevelLow)},
		{Min: 40, Max: 100, RiskLevel: string(RiskLevelHigh), MaxInclusive: true},
	}}
	cases := []struct {
		name  string
		score float64
		want  string
	}{
		{name: "below minimum", score: -0.1},
		{name: "minimum", score: 0, want: string(RiskLevelLow)},
		{name: "below boundary", score: 39.9, want: string(RiskLevelLow)},
		{name: "boundary", score: 40, want: string(RiskLevelHigh)},
		{name: "maximum", score: 100, want: string(RiskLevelHigh)},
		{name: "above maximum", score: 100.1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findInterpretRule(factor, tc.score)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("rule = %#v, want nil", got)
				}
				return
			}
			if got == nil || got.RiskLevel != tc.want {
				t.Fatalf("rule = %#v, want %s", got, tc.want)
			}
		})
	}
}
