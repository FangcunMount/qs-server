package cachetarget

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
)

func TestWarmupTargetFactoriesNormalizeScopes(t *testing.T) {
	t.Parallel()

	scale := NewStaticScaleWarmupTarget(" S-001 ")
	if scale.Family != cachemodel.FamilyStatic || scale.Kind != WarmupKindStaticScale || scale.Scope != "scale:s-001" {
		t.Fatalf("scale target = %#v", scale)
	}
	questionnaire := NewStaticQuestionnaireWarmupTarget(" Q-001 ")
	if questionnaire.Scope != "questionnaire:q-001" {
		t.Fatalf("questionnaire scope = %q", questionnaire.Scope)
	}
	if got := NewQueryStatsOverviewWarmupTarget(9, " 30D "); got.Scope != "org:9:preset:30d" {
		t.Fatalf("overview scope = %q, want org:9:preset:30d", got.Scope)
	}
}

func TestWarmupTargetParsers(t *testing.T) {
	t.Parallel()

	if kind, ok := ParseWarmupKind("query.stats_overview"); !ok || kind != WarmupKindQueryStatsOverview {
		t.Fatalf("ParseWarmupKind() = %q, %v", kind, ok)
	}
	if _, ok := ParseWarmupKind("unknown"); ok {
		t.Fatal("ParseWarmupKind() should reject unknown kind")
	}
	if code, ok := ParseStaticScaleScope("scale:s-001"); !ok || code != "s-001" {
		t.Fatalf("ParseStaticScaleScope() = %q, %v", code, ok)
	}
	if code, ok := ParseStaticQuestionnaireScope("questionnaire:q-001"); !ok || code != "q-001" {
		t.Fatalf("ParseStaticQuestionnaireScope() = %q, %v", code, ok)
	}
	if orgID, preset, ok := ParseQueryStatsOverviewScope("org:7:preset:30d"); !ok || orgID != 7 || preset != "30d" {
		t.Fatalf("ParseQueryStatsOverviewScope() = %d, %q, %v", orgID, preset, ok)
	}
}

func TestFamilyForKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind WarmupKind
		want cachemodel.Family
	}{
		{name: "static scale", kind: WarmupKindStaticScale, want: cachemodel.FamilyStatic},
		{name: "static questionnaire", kind: WarmupKindStaticQuestionnaire, want: cachemodel.FamilyStatic},
		{name: "query stats overview", kind: WarmupKindQueryStatsOverview, want: cachemodel.FamilyQuery},
		{name: "unknown", kind: WarmupKind("unknown"), want: cachemodel.FamilyDefault},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := FamilyForKind(tt.kind); got != tt.want {
				t.Fatalf("FamilyForKind(%q) = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

func TestParseWarmupTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		kind  WarmupKind
		scope string
		want  WarmupTarget
	}{
		{name: "static scale", kind: WarmupKindStaticScale, scope: " scale:S-001 ", want: NewStaticScaleWarmupTarget("s-001")},
		{name: "static questionnaire", kind: WarmupKindStaticQuestionnaire, scope: " questionnaire:Q-001 ", want: NewStaticQuestionnaireWarmupTarget("q-001")},
		{name: "query stats overview", kind: WarmupKindQueryStatsOverview, scope: " org:7:preset:30D ", want: NewQueryStatsOverviewWarmupTarget(7, "30d")},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseWarmupTarget(tt.kind, tt.scope)
			if err != nil {
				t.Fatalf("ParseWarmupTarget() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseWarmupTarget() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseWarmupTargetRejectsInvalidScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    WarmupKind
		scope   string
		wantErr string
	}{
		{name: "static scale", kind: WarmupKindStaticScale, scope: "questionnaire:q-001", wantErr: "invalid static scale warmup scope: questionnaire:q-001"},
		{name: "static questionnaire", kind: WarmupKindStaticQuestionnaire, scope: "scale:s-001", wantErr: "invalid static questionnaire warmup scope: scale:s-001"},
		{name: "query stats overview", kind: WarmupKindQueryStatsOverview, scope: "org:7:preset:90d", wantErr: "invalid stats overview warmup scope: org:7:preset:90d"},
		{name: "unsupported kind", kind: WarmupKind("unknown"), scope: "scope", wantErr: "unsupported warmup kind: unknown"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseWarmupTarget(tt.kind, tt.scope)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("ParseWarmupTarget() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestWarmupTargetOrgID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target WarmupTarget
		want   int64
		wantOK bool
	}{
		{name: "stats overview", target: NewQueryStatsOverviewWarmupTarget(7, "30d"), want: 7, wantOK: true},
		{name: "static", target: NewStaticScaleWarmupTarget("s-001"), wantOK: false},
		{name: "invalid query", target: WarmupTarget{Family: cachemodel.FamilyQuery, Kind: WarmupKindQueryStatsOverview, Scope: "org:7:preset:90d"}, wantOK: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := tt.target.OrgID()
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("OrgID() = %d, %v, want %d, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestHotsetRecordingSuppressionContext(t *testing.T) {
	t.Parallel()

	if HotsetRecordingSuppressed(context.Background()) {
		t.Fatal("plain context should not suppress hotset recording")
	}
	if !HotsetRecordingSuppressed(SuppressHotsetRecording(context.Background())) {
		t.Fatal("suppressed context should suppress hotset recording")
	}
}
