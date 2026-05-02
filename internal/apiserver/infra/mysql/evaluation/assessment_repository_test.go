package evaluation

import (
	"testing"
)

func TestNewAssessmentRepositoryWithTopicResolverCreatesCommandRepository(t *testing.T) {
	repo, ok := NewAssessmentRepositoryWithTopicResolver(nil, nil).(*assessmentRepository)
	if !ok {
		t.Fatalf("repository type = %T, want *assessmentRepository", repo)
	}
	if repo.mapper == nil {
		t.Fatalf("mapper = nil")
	}
}
