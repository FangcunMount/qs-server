package reporttemplate

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestReleaseManifestFingerprintIsCanonical(t *testing.T) {
	t.Parallel()

	left, err := NewReleaseManifest("standard", policy.TemplateVersionV1, policy.ReportTypeStandard, []ManifestRoute{
		{DecisionKind: modelcatalog.DecisionKindScoreRange, BuilderIdentity: "factor-scoring", ContentSchemaVersion: "report-content/v1"},
		{DecisionKind: modelcatalog.DecisionKindNormLookup, BuilderIdentity: "norm-profile", ContentSchemaVersion: "report-content/v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewReleaseManifest("standard", policy.TemplateVersionV1, policy.ReportTypeStandard, []ManifestRoute{
		{DecisionKind: modelcatalog.DecisionKindNormLookup, BuilderIdentity: " norm-profile ", ContentSchemaVersion: "report-content/v1"},
		{DecisionKind: modelcatalog.DecisionKindScoreRange, BuilderIdentity: "factor-scoring", ContentSchemaVersion: "report-content/v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	leftFingerprint, err := left.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	rightFingerprint, err := right.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if leftFingerprint != rightFingerprint {
		t.Fatalf("fingerprints differ: %s != %s", leftFingerprint, rightFingerprint)
	}
	if len(leftFingerprint) != 64 {
		t.Fatalf("fingerprint length = %d, want 64", len(leftFingerprint))
	}
}

func TestReleaseManifestRejectsIncompleteOrAmbiguousRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		routes []ManifestRoute
	}{
		{name: "empty"},
		{name: "unknown decision", routes: []ManifestRoute{{DecisionKind: "unknown", BuilderIdentity: "builder", ContentSchemaVersion: "schema/v1"}}},
		{name: "missing builder", routes: []ManifestRoute{{DecisionKind: modelcatalog.DecisionKindScoreRange, ContentSchemaVersion: "schema/v1"}}},
		{name: "missing schema", routes: []ManifestRoute{{DecisionKind: modelcatalog.DecisionKindScoreRange, BuilderIdentity: "builder"}}},
		{name: "duplicate decision", routes: []ManifestRoute{
			{DecisionKind: modelcatalog.DecisionKindScoreRange, BuilderIdentity: "first", ContentSchemaVersion: "schema/v1"},
			{DecisionKind: modelcatalog.DecisionKindScoreRange, BuilderIdentity: "second", ContentSchemaVersion: "schema/v1"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewReleaseManifest("standard", policy.TemplateVersionV1, policy.ReportTypeStandard, tt.routes); err == nil {
				t.Fatal("invalid manifest must be rejected")
			}
		})
	}
}

func TestReleaseManifestRouteFor(t *testing.T) {
	t.Parallel()

	manifest, err := NewReleaseManifest("mbti", policy.TemplateVersionV1, policy.ReportTypeStandard, []ManifestRoute{{
		DecisionKind: modelcatalog.DecisionKindPoleComposition, BuilderIdentity: "typology",
		ContentSchemaVersion: "report-content/v1", AdapterKey: "personality_type",
	}})
	if err != nil {
		t.Fatal(err)
	}
	route, ok := manifest.RouteFor(modelcatalog.DecisionKindPoleComposition)
	if !ok || route.AdapterKey != "personality_type" {
		t.Fatalf("route = %#v, found=%v", route, ok)
	}
	if _, ok := manifest.RouteFor(modelcatalog.DecisionKindTraitProfile); ok {
		t.Fatal("unsupported decision kind must not resolve")
	}
}
