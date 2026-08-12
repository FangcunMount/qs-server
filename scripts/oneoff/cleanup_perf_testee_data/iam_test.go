package main

import (
	"strings"
	"testing"
)

func TestValidateQSProfileScopeCounts(t *testing.T) {
	valid := qsProfileScopeCounts{existingTestees: 2, distinctProfiles: 2}
	if err := validateQSProfileScopeCounts(valid, false, false, 2, 2); err != nil {
		t.Fatalf("valid scope error = %v", err)
	}

	tests := []struct {
		name     string
		counts   qsProfileScopeCounts
		allow    bool
		noRows   bool
		wantPart string
	}{
		{name: "mapping", counts: qsProfileScopeCounts{invalidMappings: 1}, wantPart: "do not map"},
		{name: "non target reference", counts: qsProfileScopeCounts{nonTargetReferences: 1}, wantPart: "non-target"},
		{name: "shared profile", counts: qsProfileScopeCounts{existingTestees: 2, distinctProfiles: 1}, wantPart: "one-testee-to-one-profile"},
		{name: "iam cleanup still has qs rows", counts: valid, allow: true, noRows: true, wantPart: "requires every selected QS testee to be absent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateQSProfileScopeCounts(tt.counts, tt.allow, tt.noRows, 2, 2)
			if err == nil || !strings.Contains(err.Error(), tt.wantPart) {
				t.Fatalf("error = %v, want %q", err, tt.wantPart)
			}
		})
	}

	resumed := qsProfileScopeCounts{}
	if err := validateQSProfileScopeCounts(resumed, true, true, 2, 2); err != nil {
		t.Fatalf("fully removed resume scope error = %v", err)
	}
}

func TestValidateIAMScopeCounts(t *testing.T) {
	if err := validateIAMScopeCounts(iamScopeCounts{profiles: 2, links: 2}, 2, false); err != nil {
		t.Fatalf("valid IAM scope error = %v", err)
	}
	if err := validateIAMScopeCounts(iamScopeCounts{profiles: 2, links: 3}, 2, false); err == nil || !strings.Contains(err.Error(), "one-profile-to-one-link") {
		t.Fatalf("link count error = %v", err)
	}
	if err := validateIAMScopeCounts(iamScopeCounts{profiles: 2, links: 2, invalidLinks: 1}, 2, false); err == nil || !strings.Contains(err.Error(), "guard failed") {
		t.Fatalf("invalid link error = %v", err)
	}
	if err := validateIAMScopeCounts(iamScopeCounts{profiles: 1, links: 1}, 2, true); err != nil {
		t.Fatalf("partial IAM recovery scope error = %v", err)
	}
	if err := validateIAMScopeCounts(iamScopeCounts{profiles: 1, links: 2}, 2, true); err == nil || !strings.Contains(err.Error(), "remaining IAM scope") {
		t.Fatalf("partial IAM recovery link mismatch error = %v", err)
	}
}

func TestIAMDeleteStatementsUseBoundedBatchScope(t *testing.T) {
	for name, statement := range map[string]string{
		"profile links": deleteIAMBatchProfileLinksSQL,
		"profiles":      deleteIAMBatchProfilesSQL,
	} {
		if !strings.Contains(statement, "tmp_cleanup_profile_batch_ids") {
			t.Fatalf("%s delete must use bounded batch table: %s", name, statement)
		}
		if strings.Contains(statement, "JOIN tmp_cleanup_profile_ids ") {
			t.Fatalf("%s delete must not use the full IAM scope: %s", name, statement)
		}
	}
}
