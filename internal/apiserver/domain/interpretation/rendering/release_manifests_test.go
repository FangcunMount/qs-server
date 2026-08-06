package rendering

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestBuiltinReleaseManifestsMatchRegisteredBuilders(t *testing.T) {
	t.Parallel()

	manifests, err := BuiltinReleaseManifests()
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{"standard", "mbti", "sbti", "bigfive", "enneagram"}
	if len(manifests) != len(wantIDs) {
		t.Fatalf("manifest count = %d, want %d", len(manifests), len(wantIDs))
	}
	registry, err := NewRegistry(DefaultBuilders(nil)...)
	if err != nil {
		t.Fatal(err)
	}
	fingerprints := make(map[string]struct{}, len(manifests))
	for index, manifest := range manifests {
		if manifest.TemplateID != wantIDs[index] {
			t.Fatalf("manifest[%d].TemplateID = %q, want %q", index, manifest.TemplateID, wantIDs[index])
		}
		fingerprint, err := manifest.Fingerprint()
		if err != nil {
			t.Fatalf("manifest %s fingerprint: %v", manifest.TemplateID, err)
		}
		if _, duplicate := fingerprints[fingerprint]; duplicate {
			t.Fatalf("manifest %s has a duplicate fingerprint", manifest.TemplateID)
		}
		fingerprints[fingerprint] = struct{}{}
		for _, route := range manifest.Routes {
			builder, err := registry.ResolveByMechanism(Key{
				DecisionKind: route.DecisionKind, ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1,
			})
			if err != nil {
				t.Fatalf("manifest %s route %s: %v", manifest.TemplateID, route.DecisionKind, err)
			}
			if route.BuilderIdentity != builder.BuilderIdentity() || route.ContentSchemaVersion != builder.ContentSchemaVersion() {
				t.Fatalf("manifest %s route %s does not match builder", manifest.TemplateID, route.DecisionKind)
			}
		}
	}
}

func TestBuiltinReleaseManifestsCoverAllRegisteredDecisionKinds(t *testing.T) {
	t.Parallel()

	manifests, err := BuiltinReleaseManifests()
	if err != nil {
		t.Fatal(err)
	}
	covered := make(map[modelcatalog.DecisionKind]bool)
	for _, manifest := range manifests {
		for _, route := range manifest.Routes {
			covered[route.DecisionKind] = true
		}
	}
	for _, decisionKind := range []modelcatalog.DecisionKind{
		modelcatalog.DecisionKindScoreRange,
		modelcatalog.DecisionKindNormLookup,
		modelcatalog.DecisionKindAbilityLevel,
		modelcatalog.DecisionKindPoleComposition,
		modelcatalog.DecisionKindNearestPattern,
		modelcatalog.DecisionKindDominantFactor,
		modelcatalog.DecisionKindTraitProfile,
	} {
		if !covered[decisionKind] {
			t.Fatalf("registered decision kind %s has no release manifest", decisionKind)
		}
	}
}
