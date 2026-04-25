package handler

import (
	"testing"
	"time"
)

func TestBuildRelationResponse(t *testing.T) {
	sourceID := uint64(99)
	boundAt := time.Date(2026, 4, 22, 10, 30, 0, 0, time.Local)

	result := buildRelationResponse(1, 2, 3, 4, "primary", "manual", &sourceID, true, boundAt, nil)

	if result.ID != "1" || result.OrgID != "2" || result.ClinicianID != "3" || result.TesteeID != "4" {
		t.Fatalf("unexpected ids in relation response: %+v", result)
	}
	if result.SourceID == nil || *result.SourceID != "99" {
		t.Fatalf("source_id = %v, want 99", result.SourceID)
	}
	if result.RelationType != "primary" || result.SourceType != "manual" {
		t.Fatalf("unexpected relation/source: %+v", result)
	}
	if !result.IsActive || result.IsActiveLabel != "有效" {
		t.Fatalf("is_active = %v label = %q, want true/有效", result.IsActive, result.IsActiveLabel)
	}
}

func TestBuildTesteeSummaryResponse(t *testing.T) {
	profileID := uint64(88)
	birthday := time.Date(2012, 1, 2, 0, 0, 0, 0, time.Local)

	result := buildTesteeSummaryResponse(7, 8, &profileID, "Alice", 2, &birthday, []string{"vip"}, "manual", true)

	if result.ID != "7" || result.OrgID != "8" {
		t.Fatalf("unexpected ids in testee response: %+v", result)
	}
	if result.ProfileID == nil || *result.ProfileID != "88" {
		t.Fatalf("profile_id = %v, want 88", result.ProfileID)
	}
	if result.IAMChildID == nil || *result.IAMChildID != "88" {
		t.Fatalf("iam_child_id = %v, want 88", result.IAMChildID)
	}
	if result.Gender != "female" {
		t.Fatalf("gender = %q, want female", result.Gender)
	}
	if !result.IsKeyFocus || result.IsKeyFocusLabel == "" {
		t.Fatalf("unexpected key focus fields: %+v", result)
	}
	if len(result.Tags) != 1 || result.Tags[0] != "vip" {
		t.Fatalf("tags = %v, want [vip]", result.Tags)
	}
}
