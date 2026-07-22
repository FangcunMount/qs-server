package statistics

import "testing"

func TestGenerationKeyIsOrganizationScoped(t *testing.T) {
	if got := GenerationKey(42); got != "query:version:statistics:org:42" {
		t.Fatalf("key=%q", got)
	}
}
