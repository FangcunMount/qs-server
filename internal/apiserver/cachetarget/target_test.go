package cachetarget

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachemodel"
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
	if got := NewStaticScaleListWarmupTarget(); got.Scope != "published" {
		t.Fatalf("scale list scope = %q, want published", got.Scope)
	}
	if got := NewQueryStatsSystemWarmupTarget(9); got.Scope != "org:9" {
		t.Fatalf("system scope = %q, want org:9", got.Scope)
	}
	if got := NewQueryStatsQuestionnaireWarmupTarget(9, " Q-001 "); got.Scope != "org:9:questionnaire:q-001" {
		t.Fatalf("questionnaire stats scope = %q", got.Scope)
	}
	if got := NewQueryStatsPlanWarmupTarget(9, 88); got.Scope != "org:9:plan:88" {
		t.Fatalf("plan stats scope = %q", got.Scope)
	}
}

func TestWarmupTargetParsers(t *testing.T) {
	t.Parallel()

	if kind, ok := ParseWarmupKind("query.stats_plan"); !ok || kind != WarmupKindQueryStatsPlan {
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
	if orgID, ok := ParseQueryStatsSystemScope("org:7"); !ok || orgID != 7 {
		t.Fatalf("ParseQueryStatsSystemScope() = %d, %v", orgID, ok)
	}
	if orgID, code, ok := ParseQueryStatsQuestionnaireScope("org:7:questionnaire:q-001"); !ok || orgID != 7 || code != "q-001" {
		t.Fatalf("ParseQueryStatsQuestionnaireScope() = %d, %q, %v", orgID, code, ok)
	}
	if orgID, planID, ok := ParseQueryStatsPlanScope("org:7:plan:99"); !ok || orgID != 7 || planID != 99 {
		t.Fatalf("ParseQueryStatsPlanScope() = %d, %d, %v", orgID, planID, ok)
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
		{name: "static scale list", kind: WarmupKindStaticScaleList, want: cachemodel.FamilyStatic},
		{name: "query stats system", kind: WarmupKindQueryStatsSystem, want: cachemodel.FamilyQuery},
		{name: "query stats questionnaire", kind: WarmupKindQueryStatsQuestionnaire, want: cachemodel.FamilyQuery},
		{name: "query stats plan", kind: WarmupKindQueryStatsPlan, want: cachemodel.FamilyQuery},
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
		{name: "static scale list", kind: WarmupKindStaticScaleList, scope: " published ", want: NewStaticScaleListWarmupTarget()},
		{name: "query stats system", kind: WarmupKindQueryStatsSystem, scope: " org:7 ", want: NewQueryStatsSystemWarmupTarget(7)},
		{name: "query stats questionnaire", kind: WarmupKindQueryStatsQuestionnaire, scope: " org:7:questionnaire:Q-001 ", want: NewQueryStatsQuestionnaireWarmupTarget(7, "q-001")},
		{name: "query stats plan", kind: WarmupKindQueryStatsPlan, scope: " org:7:plan:99 ", want: NewQueryStatsPlanWarmupTarget(7, 99)},
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
		{name: "static scale list", kind: WarmupKindStaticScaleList, scope: "draft", wantErr: "invalid static scale list warmup scope: draft"},
		{name: "query stats system", kind: WarmupKindQueryStatsSystem, scope: "org:0", wantErr: "invalid stats system warmup scope: org:0"},
		{name: "query stats questionnaire", kind: WarmupKindQueryStatsQuestionnaire, scope: "org:7", wantErr: "invalid stats questionnaire warmup scope: org:7"},
		{name: "query stats plan", kind: WarmupKindQueryStatsPlan, scope: "org:7:plan:0", wantErr: "invalid stats plan warmup scope: org:7:plan:0"},
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
		{name: "stats system", target: NewQueryStatsSystemWarmupTarget(7), want: 7, wantOK: true},
		{name: "stats questionnaire", target: NewQueryStatsQuestionnaireWarmupTarget(8, "q-001"), want: 8, wantOK: true},
		{name: "stats plan", target: NewQueryStatsPlanWarmupTarget(9, 99), want: 9, wantOK: true},
		{name: "static", target: NewStaticScaleWarmupTarget("s-001"), wantOK: false},
		{name: "invalid query", target: WarmupTarget{Family: cachemodel.FamilyQuery, Kind: WarmupKindQueryStatsPlan, Scope: "org:7:plan:0"}, wantOK: false},
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
