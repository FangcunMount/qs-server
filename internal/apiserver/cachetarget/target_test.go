package cachetarget

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

func TestWarmupTargetFactoriesNormalizeScopes(t *testing.T) {
	t.Parallel()

	scale := NewStaticScaleWarmupTarget(" S-001 ")
	if scale.Family != redisplane.FamilyStatic || scale.Kind != WarmupKindStaticScale || scale.Scope != "scale:s-001" {
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

func TestHotsetRecordingSuppressionContext(t *testing.T) {
	t.Parallel()

	if HotsetRecordingSuppressed(context.Background()) {
		t.Fatal("plain context should not suppress hotset recording")
	}
	if !HotsetRecordingSuppressed(SuppressHotsetRecording(context.Background())) {
		t.Fatal("suppressed context should suppress hotset recording")
	}
}
