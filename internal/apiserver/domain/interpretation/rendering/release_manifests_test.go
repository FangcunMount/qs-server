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
	wantIDs := []string{"standard", "mbti", "sbti", "bigfive", "enneagram", "standard", "mbti", "sbti", "bigfive", "enneagram"}
	wantFingerprints := map[string]string{
		"legacy-v1/standard":   "c5d758a0901ed1e0c77aec5aa6606dd47b12a98e914e619fb41f1271f571fa76",
		"legacy-v1/mbti":       "38976e0b0c2a6d9b4ddb5250a9411011c294bc497e17f171ebe87db8a66349fb",
		"legacy-v1/sbti":       "0e407ca505e837054ff273d2dabd1fc7d1d31a05593a73b0d6c87510988a8706",
		"legacy-v1/bigfive":    "9b98c564a3d71b836f7099399ff0139132259696499a166de09ef395fd2c8cba",
		"legacy-v1/enneagram":  "86bf584f4ea271b4f53bf7c0c237febf714215035074ae4da58543552294dbcd",
		"2026-08-v1/standard":  "5af751626b4ac71552feb7abe1513ca8b2cb2bb78a570d87f76562efa27d5068",
		"2026-08-v1/mbti":      "3456b5d0aa2a767e0875b679bf37019a3f6a0229b65e2514d34f4d8dca744ffb",
		"2026-08-v1/sbti":      "d9d4ed92fcd6bfcd7cc9f3c11145627232bd73a3a6e7b0a4f6a1fbd9b1ee9d54",
		"2026-08-v1/bigfive":   "6b893f75f2a90c853da9493e0d7e75c8acd253c7cbb95ad829308766374e229a",
		"2026-08-v1/enneagram": "b490c2a8317c674b45468be5bb4ea109c4b58f8bce0eb1cc1d3050526fa134d4",
	}
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
		identity := manifest.TemplateVersion.String() + "/" + manifest.TemplateID
		if fingerprint != wantFingerprints[identity] {
			t.Fatalf("manifest %s fingerprint = %s, want %s", identity, fingerprint, wantFingerprints[identity])
		}
		fingerprints[fingerprint] = struct{}{}
		for _, route := range manifest.Routes {
			builder, err := registry.ResolveByMechanism(Key{
				DecisionKind: route.DecisionKind, ReportType: policy.ReportTypeStandard, TemplateVersion: manifest.TemplateVersion,
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
