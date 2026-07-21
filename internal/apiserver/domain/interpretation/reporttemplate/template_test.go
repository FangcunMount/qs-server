package reporttemplate

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestReportTemplatePublishAndDisableAudit(t *testing.T) {
	now := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	tmpl, err := NewDraft(CreateInput{
		ID: meta.FromUint64(1), TemplateID: "mbti", TemplateVersion: policy.TemplateVersionV1,
		BuilderIdentity: "typology", AdapterKey: "personality_type", CreatedAt: now,
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
}

func TestResolveVersionDefaultsToLegacyV1(t *testing.T) {
	if got := ResolveVersion(""); got != policy.TemplateVersionV1 {
		t.Fatalf("ResolveVersion() = %q", got)
	}
	if got := ResolveVersion("custom-v2"); got != policy.TemplateVersion("custom-v2") {
		t.Fatalf("ResolveVersion() = %q", got)
	}
}
