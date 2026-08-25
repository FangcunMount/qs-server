package keyspace

import "testing"

func TestBuilderWithoutNamespace(t *testing.T) {
	builder := NewBuilder()
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
	if got := builder.BuildScaleHotDailyKey("20260501"); got != "scale:hot:{rank}:daily:20260501" {
		t.Fatalf("unexpected scale hot daily key: %s", got)
	}
	if got := builder.BuildScaleHotProjectedKey("evt-1"); got != "scale:hot:{rank}:projected:evt-1" {
		t.Fatalf("unexpected scale hot projected key: %s", got)
	}
}

func TestBuilderWithNamespace(t *testing.T) {
	builder := NewBuilderWithNamespace("prod:cache:query")
	if got := builder.BuildPublishedAssessmentModelLatestByCodeKey("Typology", "MBTI"); got != "prod:cache:query:assessment_model:published:latest:typology:mbti" {
		t.Fatalf("unexpected published model latest-by-code key: %s", got)
	}
	if got := builder.BuildScaleHotWindowKey("20260501:30"); got != "prod:cache:query:scale:hot:{rank}:window:20260501:30" {
		t.Fatalf("unexpected explicit namespaced scale hot window key: %s", got)
	}
}

func TestBuilderUsesDedicatedOperationalIndexes(t *testing.T) {
	builder := NewBuilderWithNamespace("ops:runtime")

	tests := map[string]string{
		"command":        builder.BuildResilienceCommandIndexKey("apiserver"),
		"result":         builder.BuildResilienceCommandResultIndexKey("9:request-1"),
		"instance":       builder.BuildResilienceInstanceIndexKey("apiserver"),
		"audit fallback": builder.BuildGovernanceAuditReplayIndexKey(),
	}
	wants := map[string]string{
		"command":        "ops:runtime:resilience:index:command:apiserver",
		"result":         "ops:runtime:resilience:index:result:9:request-1",
		"instance":       "ops:runtime:resilience:index:instance:apiserver",
		"audit fallback": "ops:runtime:governance:index:audit-replay",
	}
	for name, got := range tests {
		if got != wants[name] {
			t.Fatalf("%s index key = %q, want %q", name, got, wants[name])
		}
	}
}

func TestStatisticsBuilderUsesNamespacedV2Schema(t *testing.T) {
	builder := NewBuilderWithNamespace("cache:query")

	if got := builder.BuildStatisticsGenerationKey(42); got != "cache:query:query:version:statistics:v2:org:42" {
		t.Fatalf("unexpected Statistics generation key: %s", got)
	}
	if got := builder.BuildStatisticsDataKey(42, 3, "abcdef01"); got != "cache:query:query:data:statistics:v2:org:42:g:3:abcdef01" {
		t.Fatalf("unexpected Statistics data key: %s", got)
	}
}
