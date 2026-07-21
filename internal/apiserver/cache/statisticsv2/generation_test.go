package statisticsv2

import "testing"

func TestGenerationKeyIsOrganizationScoped(t *testing.T) {
	if got := GenerationKey(42); got != "query:version:statistics:v2:org:42" {
		t.Fatalf("key=%q", got)
	}
}
