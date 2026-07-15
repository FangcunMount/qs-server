package main

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestConvertSnapshotMapsLegacyFieldsAndKeepsPayload(t *testing.T) {
	legacyID := primitive.NewObjectID()
	payload := []byte(`{"version":"1"}`)
	row := bson.M{
		"_id":                   legacyID,
		"model_product_channel": "medical",
		"model_kind":            "scale",
		"model_sub_kind":        "",
		"model_algorithm":       "scale_default",
		"model_code":            "PHQ9",
		"model_version":         "2.0.0",
		"status":                "published",
		"questionnaire_code":    "PHQ9_Q",
		"questionnaire_version": "2.0.0",
		"payload_format":        "assessment_scale.v1",
		"payload":               payload,
		"decision_kind":         "score_range",
		"definition_v2":         bson.M{"measure": bson.M{}},
	}

	got := convertSnapshot(row)
	if got["_id"] == legacyID || got["legacy_source_id"] != legacyID.Hex() {
		t.Fatalf("migration provenance = %#v", got)
	}
	if got["record_role"] != roleSnapshot || got["is_active_published"] != true {
		t.Fatalf("snapshot role/activity = %#v", got)
	}
	if got["code"] != "PHQ9" || got["release_version"] != "2.0.0" || got["kind"] != "scale" {
		t.Fatalf("mapped identity = %#v", got)
	}
	if payloadHash(bytesField(got, "payload")) != payloadHash(payload) {
		t.Fatal("payload bytes changed during conversion")
	}
	if _, exists := got["model_code"]; exists {
		t.Fatalf("legacy identity leaked into converted document: %#v", got)
	}
}

func TestConvertSnapshotRetainsLegacySoftDeleteAsInactiveHistory(t *testing.T) {
	deletedAt := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	got := convertSnapshot(bson.M{
		"_id":             primitive.NewObjectID(),
		"model_kind":      "typology",
		"model_algorithm": "mbti",
		"model_code":      "MBTI",
		"model_version":   "1.0.0",
		"status":          "unpublished",
		"deleted_at":      deletedAt,
		"payload":         []byte("payload"),
	})
	if got["is_active_published"] != false || got["retention_state"] != "legacy_soft_deleted" {
		t.Fatalf("soft-deleted snapshot was not retained inactive: %#v", got)
	}
	if got["deleted_at"] != nil || got["legacy_deleted_at"] != deletedAt || got["status"] != "published" {
		t.Fatalf("retained snapshot lifecycle = %#v", got)
	}
}
