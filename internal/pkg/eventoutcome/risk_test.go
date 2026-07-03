package eventoutcome

import "testing"

func TestLevelIsHighRisk(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		level *ResultLevel
		want  bool
	}{
		{name: "nil", level: nil, want: false},
		{name: "high severity", level: &ResultLevel{Severity: "high"}, want: true},
		{name: "severe code", level: &ResultLevel{Code: "severe"}, want: true},
		{name: "low code", level: &ResultLevel{Code: "low"}, want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := LevelIsHighRisk(tc.level); got != tc.want {
				t.Fatalf("LevelIsHighRisk() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAttentionRiskLevel(t *testing.T) {
	t.Parallel()

	if got := AttentionRiskLevel(&ResultLevel{Code: "high"}); got != "high" {
		t.Fatalf("legacy code passthrough = %q, want high", got)
	}
	if got := AttentionRiskLevel(&ResultLevel{Severity: "medium"}); got != "medium" {
		t.Fatalf("severity mapping = %q, want medium", got)
	}
}
