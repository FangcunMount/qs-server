package evaluation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

func TestNewAssessmentRepositoryWithTopicResolverInjectsOutboxResolver(t *testing.T) {
	resolver := eventcatalog.NewCatalog(nil)
	repo, ok := NewAssessmentRepositoryWithTopicResolver(nil, resolver).(*assessmentRepository)
	if !ok {
		t.Fatalf("repository type = %T, want *assessmentRepository", repo)
	}
	if repo.outboxStore == nil {
		t.Fatalf("outbox store = nil")
	}
}
