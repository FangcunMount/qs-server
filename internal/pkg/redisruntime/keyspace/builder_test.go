package keyspace

import "testing"

func TestBuilderWithoutNamespace(t *testing.T) {
	ApplyNamespace("")

	builder := NewBuilder()
	if got := builder.BuildStatsQueryKey("system:1"); got != "stats:query:system:1" {
		t.Fatalf("unexpected stats query key: %s", got)
	}
	if got := builder.BuildAnswerSheetProcessingLockKey(42); got != "answersheet:processing:42" {
		t.Fatalf("unexpected answersheet lock key: %s", got)
	}
	if got := builder.BuildLockKey("qs:plan-scheduler:leader"); got != "qs:plan-scheduler:leader" {
		t.Fatalf("unexpected generic lock key: %s", got)
	}
	if got := builder.BuildWeChatCacheKey("access_token"); got != "wechat:cache:access_token" {
		t.Fatalf("unexpected wechat key: %s", got)
	}
	if got := builder.BuildQueryVersionKey("assessment:list", "42"); got != "query:version:assessment:list:42" {
		t.Fatalf("unexpected query version key: %s", got)
	}
	if got := builder.BuildVersionedQueryKey("assessment:list", "42", 3, "deadbeef"); got != "query:assessment:list:42:v3:deadbeef" {
		t.Fatalf("unexpected versioned query key: %s", got)
	}
	if got := builder.BuildScaleHotDailyKey("20260501"); got != "scale:hot:{rank}:daily:20260501" {
		t.Fatalf("unexpected scale hot daily key: %s", got)
	}
	if got := builder.BuildScaleHotProjectedKey("evt-1"); got != "scale:hot:{rank}:projected:evt-1" {
		t.Fatalf("unexpected scale hot projected key: %s", got)
	}
}

func TestBuilderWithNamespace(t *testing.T) {
	ApplyNamespace("dev")
	defer ApplyNamespace("")

	builder := NewBuilder()
	if got := builder.BuildStatsQueryKey("system:1"); got != "dev:stats:query:system:1" {
		t.Fatalf("unexpected namespaced stats query key: %s", got)
	}
	if got := builder.BuildAnswerSheetProcessingLockKey(42); got != "dev:answersheet:processing:42" {
		t.Fatalf("unexpected namespaced answersheet lock key: %s", got)
	}
	if got := builder.BuildLockKey("qs:plan-scheduler:leader"); got != "dev:qs:plan-scheduler:leader" {
		t.Fatalf("unexpected namespaced generic lock key: %s", got)
	}
	if got := builder.BuildWeChatCacheKey("access_token"); got != "dev:wechat:cache:access_token" {
		t.Fatalf("unexpected namespaced wechat key: %s", got)
	}
	if got := builder.BuildAssessmentListVersionKey(42); got != "dev:query:version:assessment:list:42" {
		t.Fatalf("unexpected namespaced assessment list version key: %s", got)
	}
}

func TestBuilderWithExplicitNamespace(t *testing.T) {
	ApplyNamespace("dev")
	defer ApplyNamespace("")

	builder := NewBuilderWithNamespace("prod:cache:query")
	if got := builder.BuildStatsQueryKey("system:1"); got != "prod:cache:query:stats:query:system:1" {
		t.Fatalf("unexpected explicit namespaced stats query key: %s", got)
	}
	if got := builder.BuildScaleKey("SDS"); got != "prod:cache:query:scale:SDS" {
		t.Fatalf("unexpected explicit namespaced scale key: %s", got)
	}
	if got := builder.BuildPublishedScaleKey("s-001"); got != "prod:cache:query:scale:published:s-001" {
		t.Fatalf("unexpected published scale key: %s", got)
	}
	if got := builder.BuildPublishedScaleByQuestionnaireKey("q-001"); got != "prod:cache:query:scale:published:questionnaire:q-001" {
		t.Fatalf("unexpected published scale questionnaire key: %s", got)
	}
	if got := builder.BuildPublishedAssessmentModelLatestByCodeKey("Typology", "MBTI"); got != "prod:cache:query:assessment_model:published:latest:typology:mbti" {
		t.Fatalf("unexpected published model latest-by-code key: %s", got)
	}
	if got := builder.BuildScaleHotListKey(5, 30); got != "prod:cache:query:scale:hot:list:v1:5:30" {
		t.Fatalf("unexpected hot list key: %s", got)
	}
	if got := builder.BuildScaleHotWindowKey("20260501:30"); got != "prod:cache:query:scale:hot:{rank}:window:20260501:30" {
		t.Fatalf("unexpected explicit namespaced scale hot window key: %s", got)
	}
}
