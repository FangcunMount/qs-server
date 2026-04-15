package rediskey

import "testing"

func TestBuilderWithoutNamespace(t *testing.T) {
	ApplyNamespace("")

	builder := NewBuilder()
	if got := builder.BuildStatsQueryKey("system:1"); got != "stats:query:system:1" {
		t.Fatalf("unexpected stats query key: %s", got)
	}
	if got := builder.BuildEventProcessedKey("evt-1"); got != "event:processed:evt-1" {
		t.Fatalf("unexpected event processed key: %s", got)
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
}

func TestBuilderWithNamespace(t *testing.T) {
	ApplyNamespace("dev")
	defer ApplyNamespace("")

	builder := NewBuilder()
	if got := builder.BuildStatsDailyKey(1, "questionnaire", "PHQ9", "2026-04-15"); got != "dev:stats:daily:1:questionnaire:PHQ9:2026-04-15" {
		t.Fatalf("unexpected namespaced stats daily key: %s", got)
	}
	if got := builder.BuildStatsDailyPattern(1, "questionnaire"); got != "dev:stats:daily:1:questionnaire:*" {
		t.Fatalf("unexpected namespaced stats daily pattern: %s", got)
	}
	if got := builder.BuildEventProcessedKey("evt-1"); got != "dev:event:processed:evt-1" {
		t.Fatalf("unexpected namespaced event processed key: %s", got)
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
}
