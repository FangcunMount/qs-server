package statistics

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
)

func TestGenerationKeyIsOrganizationScoped(t *testing.T) {
	publisher := NewGenerationPublisher(nil, keyspace.NewBuilderWithNamespace("cache:query"))
	if got := publisher.keyBuilder.BuildStatisticsGenerationKey(42); got != "cache:query:query:version:statistics:v2:org:42" {
		t.Fatalf("key=%q", got)
	}
}
