package main

import "testing"

func TestIdentityRefPatternAcceptsOnlyCanonicalV2(t *testing.T) {
	t.Parallel()

	valid := "isn:v2:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if !identityRefPattern.MatchString(valid) {
		t.Fatalf("canonical v2 ref rejected: %s", valid)
	}
	for _, invalid := range []string{
		"isn:v1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"model:brief2",
		"answersheet:42",
		"isn:v2:ABCDEF",
		"isn:v2:0123456789abcdef",
	} {
		if identityRefPattern.MatchString(invalid) {
			t.Fatalf("non-canonical ref accepted: %s", invalid)
		}
	}
}
