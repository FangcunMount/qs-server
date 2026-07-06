package interpretation

import "testing"

func TestIsRiskLevelCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		code string
		want bool
	}{
		{"none", true},
		{"low", true},
		{"medium", true},
		{"high", true},
		{"severe", true},
		{"INTJ", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsRiskLevelCode(tc.code); got != tc.want {
			t.Fatalf("IsRiskLevelCode(%q) = %v, want %v", tc.code, got, tc.want)
		}
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
