package main

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestBytesFieldReadsBSONBinaryAfterRoundTrip(t *testing.T) {
	want := []byte(`{"version":"1"}`)
	encoded, err := bson.Marshal(bson.M{"payload": want})
	if err != nil {
		t.Fatalf("marshal BSON: %v", err)
	}
	var row bson.M
	if err := bson.Unmarshal(encoded, &row); err != nil {
		t.Fatalf("unmarshal BSON: %v", err)
	}
	if _, ok := row["payload"].(primitive.Binary); !ok {
		t.Fatalf("payload type = %T, want primitive.Binary", row["payload"])
	}
	if got := bytesField(row, "payload"); string(got) != string(want) {
		t.Fatalf("payload = %q, want %q", got, want)
	}
}

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
	if got["release_status"] != "active" {
		t.Fatalf("release status = %#v, want active", got["release_status"])
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
	if got["release_status"] != "archived" {
		t.Fatalf("release status = %#v, want archived", got["release_status"])
	}
	if got["deleted_at"] != nil || got["legacy_deleted_at"] != deletedAt || got["status"] != "published" {
		t.Fatalf("retained snapshot lifecycle = %#v", got)
	}
}

func TestConvertQuestionnaireRecordNormalizesArchivedSnapshot(t *testing.T) {
	deletedAt := time.Date(2026, time.July, 2, 0, 0, 0, 0, time.UTC)
	got := convertQuestionnaireRecord(bson.M{
		"_id": primitive.NewObjectID(), "code": "Q-1", "version": "1.0.0",
		"record_role": roleSnapshot, "status": "published", "deleted_at": deletedAt,
	})
	if got["release_status"] != "archived" || got["is_active_published"] != false {
		t.Fatalf("questionnaire release = %#v", got)
	}
	if got["deleted_at"] != nil || got["legacy_deleted_at"] != deletedAt {
		t.Fatalf("questionnaire provenance = %#v", got)
	}
}

func TestDeduplicateSnapshotsRejectsPayloadConflict(t *testing.T) {
	base := bson.M{"model_kind": "scale", "model_code": "S-1", "model_version": "v1", "payload": []byte("one"), "questionnaire_code": "Q", "questionnaire_version": "1"}
	conflict := bson.M{"kind": "scale", "code": "S-1", "release_version": "v1", "payload": []byte("two"), "questionnaire_code": "Q", "questionnaire_version": "1"}
	_, issues := deduplicateSnapshots([]bson.M{base, conflict})
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one payload conflict", issues)
	}
}

func TestDeduplicateSnapshotsRejectsBSONBinaryPayloadConflict(t *testing.T) {
	base := bson.M{"model_kind": "scale", "model_code": "S-1", "model_version": "v1", "payload": primitive.Binary{Data: []byte("one")}, "questionnaire_code": "Q", "questionnaire_version": "1"}
	conflict := bson.M{"kind": "scale", "code": "S-1", "release_version": "v1", "payload": primitive.Binary{Data: []byte("two")}, "questionnaire_code": "Q", "questionnaire_version": "1"}
	_, issues := deduplicateSnapshots([]bson.M{base, conflict})
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one payload conflict", issues)
	}
}

func TestInspectModelSnapshotsCountsLifecycleWhenPayloadIsInvalid(t *testing.T) {
	active := bson.M{
		"model_kind": "scale", "model_code": "S-1", "model_version": "v1",
		"status": "published", "deleted_at": nil,
		"questionnaire_code": "Q", "questionnaire_version": "1",
	}
	archived := bson.M{
		"model_kind": "scale", "model_code": "S-2", "model_version": "v1",
		"status": "published", "deleted_at": time.Now().UTC(),
	}
	got := inspectModelSnapshots([]bson.M{active, archived}, map[string]struct{}{"S-1": {}, "S-2": {}})
	if len(got.activeCodes) != 1 || got.retired != 1 {
		t.Fatalf("lifecycle active=%d retired=%d, want 1/1", len(got.activeCodes), got.retired)
	}
	if len(got.issues) != 2 || !strings.Contains(strings.Join(got.issues, "\n"), "invalid published snapshot") {
		t.Fatalf("issues = %#v, want two invalid snapshot issues", got.issues)
	}
}

func TestInspectModelSnapshotsRejectsActiveOrphanButRetainsArchivedOrphan(t *testing.T) {
	active := bson.M{
		"model_kind": "scale", "model_code": "ACTIVE", "model_version": "v1",
		"status": "published", "deleted_at": nil,
		"questionnaire_code": "Q", "questionnaire_version": "1",
		"payload": []byte("payload"), "payload_format": "assessmentmodel.scale.v1",
		"decision_kind": "score_range", "definition_v2": bson.M{"measure": bson.M{}},
	}
	archived := bson.M{
		"model_kind": "scale", "model_code": "ARCHIVED", "model_version": "v1",
		"status": "published", "deleted_at": time.Now().UTC(),
		"payload": []byte("payload"), "payload_format": "assessmentmodel.scale.v1",
		"decision_kind": "score_range", "definition_v2": bson.M{"measure": bson.M{}},
	}
	got := inspectModelSnapshots([]bson.M{active, archived}, map[string]struct{}{})
	if got.orphaned != 2 {
		t.Fatalf("orphaned = %d, want 2", got.orphaned)
	}
	if len(got.issues) != 1 || !strings.Contains(got.issues[0], "active orphan snapshot ACTIVE@v1") {
		t.Fatalf("issues = %#v, want active orphan only", got.issues)
	}
}

func TestInspectModelSnapshotsReportsIncompleteFields(t *testing.T) {
	row := bson.M{
		"model_kind": "typology", "model_code": "T-1", "model_version": "v1",
		"status": "published", "deleted_at": time.Now().UTC(), "payload": []byte("payload"),
	}
	got := inspectModelSnapshots([]bson.M{row}, map[string]struct{}{"T-1": {}})
	if len(got.issues) != 1 || !strings.Contains(got.issues[0], "missing=payload_format,decision_kind,definition_v2") {
		t.Fatalf("issues = %#v, want named incomplete fields", got.issues)
	}
}

func TestQuestionnaireSnapshotSourcesDuplicatesLegacyPublishedHeadAsRelease(t *testing.T) {
	legacy := bson.M{"_id": primitive.NewObjectID(), "code": "Q-1", "version": "1", "status": "published", "questions": bson.A{bson.M{"code": "Q1"}}}
	draft := bson.M{"_id": primitive.NewObjectID(), "code": "Q-2", "version": "1", "status": "draft"}
	sources := questionnaireSnapshotSources([]bson.M{legacy, draft})
	if len(sources) != 1 || stringField(sources[0], "code") != "Q-1" {
		t.Fatalf("snapshot sources = %#v", sources)
	}
}

func TestDeduplicateQuestionnaireSnapshotsRejectsContentConflict(t *testing.T) {
	legacy := bson.M{"code": "Q-1", "version": "1", "status": "published", "title": "Before"}
	unified := bson.M{"code": "Q-1", "version": "1", "status": "published", "record_role": roleSnapshot, "title": "After"}
	_, issues := deduplicateQuestionnaireSnapshots([]bson.M{legacy, unified})
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one conflict", issues)
	}
}

func TestInspectQuestionnaireSnapshotsRejectsArchivedMissingVersion(t *testing.T) {
	got := inspectQuestionnaireSnapshots([]bson.M{{
		"record_role": roleSnapshot,
		"code":        "Q-1",
		"version":     "",
		"status":      "published",
		"deleted_at":  time.Now().UTC(),
	}})
	if len(got.issues) != 1 || !strings.Contains(got.issues[0], `code="Q-1" version=""`) {
		t.Fatalf("issues = %#v, want archived snapshot identity issue", got.issues)
	}
}

func TestInspectQuestionnaireHeadsRejectsMissingWorkingVersion(t *testing.T) {
	issues := inspectQuestionnaireHeads([]bson.M{{
		"record_role": roleHead,
		"code":        "Q-1",
		"version":     "",
		"status":      "archived",
	}}, map[string]struct{}{})
	if len(issues) != 1 || !strings.Contains(issues[0], `code="Q-1" version=""`) {
		t.Fatalf("issues = %#v, want head identity issue", issues)
	}
}

func TestInspectQuestionnaireHeadsRejectsActiveOrphan(t *testing.T) {
	issues := inspectQuestionnaireHeads(nil, map[string]struct{}{"Q-ORPHAN": {}})
	if len(issues) != 1 || issues[0] != "active questionnaire snapshot without head Q-ORPHAN" {
		t.Fatalf("issues = %#v, want active questionnaire orphan", issues)
	}
}
