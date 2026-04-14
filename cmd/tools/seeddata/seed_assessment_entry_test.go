package main

import (
	"testing"
	"time"
)

func TestValidateAndNormalizeAssessmentEntryTargetConfig(t *testing.T) {
	t.Run("normalizes target fields", func(t *testing.T) {
		cfg, err := validateAndNormalizeAssessmentEntryTargetConfig(AssessmentEntryTargetConfig{
			TargetType:    " Questionnaire ",
			TargetCode:    " SDQ ",
			TargetVersion: " V1 ",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.TargetType != "questionnaire" || cfg.TargetCode != "SDQ" || cfg.TargetVersion != "V1" {
			t.Fatalf("unexpected normalized config: %+v", cfg)
		}
	})

	t.Run("rejects invalid target type", func(t *testing.T) {
		_, err := validateAndNormalizeAssessmentEntryTargetConfig(AssessmentEntryTargetConfig{
			TargetType: "entry",
			TargetCode: "sdq",
		})
		if err == nil {
			t.Fatal("expected invalid target type error")
		}
	})

	t.Run("rejects duplicate expiry modes", func(t *testing.T) {
		_, err := validateAndNormalizeAssessmentEntryTargetConfig(AssessmentEntryTargetConfig{
			TargetType:   "scale",
			TargetCode:   "mchat",
			ExpiresAt:    "2029-12-21 23:59:59",
			ExpiresAfter: "30d",
		})
		if err == nil {
			t.Fatal("expected duplicate expiry mode error")
		}
	})
}

func TestResolveAssessmentEntryExpiresAt(t *testing.T) {
	createdAt := time.Date(2026, 4, 1, 9, 30, 0, 0, time.UTC)

	t.Run("supports relative expiry from created_at", func(t *testing.T) {
		expiresAt, err := resolveAssessmentEntryExpiresAt(AssessmentEntryTargetConfig{
			ExpiresAfter: "30d",
		}, createdAt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := createdAt.Add(30 * 24 * time.Hour)
		if expiresAt == nil || !expiresAt.Equal(want) {
			t.Fatalf("unexpected expires_at: got=%v want=%v", expiresAt, want)
		}
	})

	t.Run("rejects absolute expiry before created_at", func(t *testing.T) {
		_, err := resolveAssessmentEntryExpiresAt(AssessmentEntryTargetConfig{
			ExpiresAt: "2026-03-31 23:59:59",
		}, createdAt)
		if err == nil {
			t.Fatal("expected expires_at before created_at error")
		}
	})
}

func TestDeriveAssessmentEntryCreatedAt(t *testing.T) {
	anchor := time.Date(2026, 4, 1, 8, 0, 0, 123, time.UTC)
	first := deriveAssessmentEntryCreatedAt(anchor, 0)
	second := deriveAssessmentEntryCreatedAt(anchor, 1)

	if !first.Equal(anchor.Round(0)) {
		t.Fatalf("unexpected first created_at: got=%v want=%v", first, anchor.Round(0))
	}
	if !second.Equal(anchor.Round(0).Add(assessmentEntrySeedTargetInterval)) {
		t.Fatalf("unexpected second created_at: got=%v want=%v", second, anchor.Round(0).Add(assessmentEntrySeedTargetInterval))
	}
}
