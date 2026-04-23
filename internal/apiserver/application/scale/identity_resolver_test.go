package scale

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type identityResolverStub struct {
	enabled bool
	names   map[string]string
	lastIDs []meta.ID
}

func (s *identityResolverStub) IsEnabled() bool {
	return s.enabled
}

func (s *identityResolverStub) ResolveUserNames(_ context.Context, ids []meta.ID) map[string]string {
	s.lastIDs = append([]meta.ID(nil), ids...)
	return s.names
}

func TestResolveIdentityNamesUsesResolver(t *testing.T) {
	resolver := &identityResolverStub{
		enabled: true,
		names:   map[string]string{"101": "Alice"},
	}

	got := resolveIdentityNames(context.Background(), resolver, []meta.ID{meta.ID(101)})
	if len(resolver.lastIDs) != 1 || resolver.lastIDs[0] != meta.ID(101) {
		t.Fatalf("resolver ids = %v, want [101]", resolver.lastIDs)
	}
	if got["101"] != "Alice" {
		t.Fatalf("resolved names = %v, want Alice", got)
	}
	if displayIdentityName(meta.ID(101), got) != "Alice" {
		t.Fatalf("displayIdentityName() did not use resolved value")
	}
}
