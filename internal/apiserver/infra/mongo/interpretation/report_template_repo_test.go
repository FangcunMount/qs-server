package interpretation

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type reportTemplateManifestCatalogStub struct {
	manifest domainreporttemplate.ReleaseManifest
}

func (c reportTemplateManifestCatalogStub) ResolveManifest(templateID string, version policy.TemplateVersion) (domainreporttemplate.ReleaseManifest, bool) {
	if c.manifest.TemplateID != templateID || c.manifest.TemplateVersion != version {
		return domainreporttemplate.ReleaseManifest{}, false
	}
	return c.manifest.Clone(), true
}

func TestReportTemplatePersistenceRoundTripUsesManifestIdentity(t *testing.T) {
	t.Parallel()

	manifest := reportTemplateManifestForTest(t, "report-content/v1")
	fingerprint, err := manifest.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	template, err := domainreporttemplate.Rehydrate(domainreporttemplate.PersistedInput{
		ID: meta.FromUint64(42), Manifest: manifest, ManifestFingerprint: fingerprint,
		Status: domainreporttemplate.StatusDraft, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	po := reportTemplateToPO(template)
	payload, err := bson.Marshal(po)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ReportTemplatePO
	if err := bson.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	restored, err := reportTemplateToDomain(&decoded)
	if err != nil {
		t.Fatal(err)
	}
	if restored.TemplateID() != "mbti" || restored.ManifestFingerprint() != fingerprint {
		t.Fatalf("restored template = %#v", restored)
	}
}

func TestValidateRegisteredReportTemplateRejectsBinaryManifestDrift(t *testing.T) {
	t.Parallel()

	persistedManifest := reportTemplateManifestForTest(t, "report-content/v2")
	persistedFingerprint, err := persistedManifest.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	template, err := domainreporttemplate.Rehydrate(domainreporttemplate.PersistedInput{
		ID: meta.FromUint64(42), Manifest: persistedManifest, ManifestFingerprint: persistedFingerprint,
		Status: domainreporttemplate.StatusDraft, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	currentManifest := reportTemplateManifestForTest(t, "report-content/v1")
	if err := validateRegisteredReportTemplate(template, reportTemplateManifestCatalogStub{manifest: currentManifest}); err == nil {
		t.Fatal("persisted manifest drift must fail closed")
	}
}

func reportTemplateManifestForTest(t *testing.T, schema string) domainreporttemplate.ReleaseManifest {
	t.Helper()
	manifest, err := domainreporttemplate.NewReleaseManifest("mbti", policy.TemplateVersionV1, policy.ReportTypeStandard, []domainreporttemplate.ManifestRoute{{
		DecisionKind: modelcatalog.DecisionKindPoleComposition, BuilderIdentity: "typology",
		ContentSchemaVersion: schema, AdapterKey: "personality_type",
	}})
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
