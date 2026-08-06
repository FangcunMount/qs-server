package reporttemplate

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestReportTemplatePublishAndDisableAudit(t *testing.T) {
	now := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	manifest := testReleaseManifest(t, "mbti", policy.TemplateVersionV1)
	tmpl, err := NewDraft(CreateInput{
		ID: meta.FromUint64(1), Manifest: manifest, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmpl.Publish("operator-1", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if !tmpl.IsPublished() || tmpl.PublishedBy() != "operator-1" || tmpl.PublishedAt() == nil {
		t.Fatalf("publish audit = %#v", tmpl)
	}
	if err := tmpl.Disable("operator-2", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if tmpl.Status() != StatusDisabled || tmpl.DisabledBy() != "operator-2" || tmpl.DisabledAt() == nil {
		t.Fatalf("disable audit = %#v", tmpl)
	}
	if tmpl.ManifestFingerprint() == "" || tmpl.Manifest().TemplateID != "mbti" {
		t.Fatalf("manifest identity = %#v fingerprint=%q", tmpl.Manifest(), tmpl.ManifestFingerprint())
	}
}

func TestReportTemplateRehydrateRejectsManifestFingerprintMismatch(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	_, err := Rehydrate(PersistedInput{
		ID: meta.FromUint64(1), Manifest: testReleaseManifest(t, "mbti", policy.TemplateVersionV1),
		ManifestFingerprint: "tampered", Status: StatusDraft, CreatedAt: now, UpdatedAt: now,
	})
	if err == nil {
		t.Fatal("manifest fingerprint mismatch must fail closed")
	}
}

func testReleaseManifest(t *testing.T, templateID string, version policy.TemplateVersion) ReleaseManifest {
	t.Helper()
	manifest, err := NewReleaseManifest(templateID, version, policy.ReportTypeStandard, []ManifestRoute{{
		DecisionKind: modelcatalog.DecisionKindPoleComposition, BuilderIdentity: "typology",
		ContentSchemaVersion: "report-content/v1", AdapterKey: "personality_type",
	}})
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
