package catalogcache

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
)

func TestQuestionnaireL1HitMissSmoke(t *testing.T) {
	t.Parallel()

	cache := questionnaire.NewLocalCache(questionnaire.LocalCacheOptions{TTL: time.Minute, MaxEntries: 8})
	cache.Set("q1", "1.0", &questionnaire.QuestionnaireResponse{Code: "q1", Version: "1.0"})
	if _, ok := cache.Get("q1", "1.0"); !ok {
		t.Fatal("expected questionnaire L1 hit")
	}
}
