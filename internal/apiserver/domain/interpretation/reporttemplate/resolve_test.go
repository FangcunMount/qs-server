package reporttemplate

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

func TestResolveFromAssetsRequiresOneExplicitConsistentRoute(t *testing.T) {
	assets := interpretationassets.Assets{ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{
		{Code: "first", TemplateID: "standard", TemplateVersion: "2026-08-v1"},
		{Code: "second", TemplateID: "standard", TemplateVersion: "2026-08-v1"},
	}}}
	got, err := ResolveFromAssets(assets)
	if err != nil || got.TemplateID != "standard" || got.TemplateVersion != "2026-08-v1" {
		t.Fatalf("route = %#v, error = %v", got, err)
	}

	assets.ReportSpec.Sections[1].TemplateID = "mbti"
	if _, err := ResolveFromAssets(assets); err == nil {
		t.Fatal("conflicting frozen template ids were accepted")
	}
	assets.ReportSpec.Sections[1].TemplateID = "standard"
	assets.ReportSpec.Sections[1].TemplateVersion = ""
	if _, err := ResolveFromAssets(assets); err == nil {
		t.Fatal("missing frozen template version was accepted")
	}
	assets.ReportSpec.Sections[1].TemplateVersion = "legacy-v1"
	if _, err := ResolveFromAssets(assets); err == nil {
		t.Fatal("conflicting frozen template versions were accepted")
	}
	if _, err := ResolveFromAssets(interpretationassets.Assets{}); err == nil {
		t.Fatal("missing frozen report sections were accepted")
	}
	assets.ReportSpec.Sections[1].TemplateVersion = "2026-08-v1"
	assets.ReportSpec.Sections[0].TemplateID = ""
	if _, err := ResolveFromAssets(assets); err == nil {
		t.Fatal("missing frozen template id was accepted")
	}
}
