package statisticsv2

import (
	"strings"
	"testing"
	"time"

	domainv2 "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics/v2"
)

func TestTruncateRunTextPreservesUnicodeCharacterBoundary(t *testing.T) {
	value := strings.Repeat("审", 1001)
	got := truncateRunText(value, 1000)
	if len([]rune(got)) != 1000 || !strings.HasSuffix(got, "审") {
		t.Fatalf("runes=%d suffix=%q", len([]rune(got)), got[len(got)-3:])
	}
}

func TestFromRunPOPresentsPublishedCacheGeneration(t *testing.T) {
	publishedAt := time.Date(2026, 7, 22, 0, 31, 0, 0, domainv2.Shanghai)
	run := fromRunPO(runPO{
		ID: 9, OrgID: 7, RunMode: string(domainv2.RunModePublish),
		CacheGeneration: 5, CachePublishedAt: &publishedAt,
	})
	if run.CacheGeneration != 5 || run.CachePublishedAt == nil || !run.CachePublishedAt.Equal(publishedAt) {
		t.Fatalf("run=%+v", run)
	}
}
