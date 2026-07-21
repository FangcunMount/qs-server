package main

import (
	"testing"
)

func TestBuildReportOneToManyUsage(t *testing.T) {
	t.Parallel()
	r := buildReport(
		[]string{"norm-a", "norm-b"},
		[]snapshotRef{
			{Code: "M1", Version: "1.0.0", Kind: "behavioral_rating", Algorithm: "brief2", NormRefs: []normRef{
				{FactorCode: "f1", NormTableVersion: "norm-a"},
				{FactorCode: "f2", NormTableVersion: "norm-a"},
			}},
			{Code: "M2", Version: "2.0.0", Kind: "cognitive", Algorithm: "spm", NormRefs: []normRef{
				{FactorCode: "total", NormTableVersion: "norm-a"},
			}},
		},
		"",
	)
	if r.PublishedScanned != 2 || r.PublishedWithRefs != 2 {
		t.Fatalf("scanned=%d with_refs=%d", r.PublishedScanned, r.PublishedWithRefs)
	}
	if r.UsageCount != 1 || r.Usages[0].NormTableVersion != "norm-a" || len(r.Usages[0].Models) != 2 {
		t.Fatalf("usages=%#v", r.Usages)
	}
	if r.UnreferencedCount != 1 || r.UnreferencedNorms[0] != "norm-b" {
		t.Fatalf("unreferenced=%#v", r.UnreferencedNorms)
	}
	if r.DanglingCount != 0 {
		t.Fatalf("dangling=%#v", r.DanglingRefs)
	}
}

func TestBuildReportDangling(t *testing.T) {
	t.Parallel()
	r := buildReport(
		[]string{"norm-a"},
		[]snapshotRef{
			{Code: "M1", Version: "1.0.0", Kind: "behavioral_rating", Algorithm: "brief2", NormRefs: []normRef{
				{FactorCode: "f1", NormTableVersion: "missing"},
			}},
		},
		"",
	)
	if r.DanglingCount != 1 || r.DanglingRefs[0].NormTableVersion != "missing" {
		t.Fatalf("dangling=%#v", r.DanglingRefs)
	}
	if r.UsageCount != 1 { // usage still listed for missing version (impact analysis)
		t.Fatalf("usage_count=%d want 1", r.UsageCount)
	}
}

func TestBuildReportUnreferenced(t *testing.T) {
	t.Parallel()
	r := buildReport([]string{"orphan"}, nil, "")
	if r.UnreferencedCount != 1 || r.UnreferencedNorms[0] != "orphan" {
		t.Fatalf("unreferenced=%#v", r.UnreferencedNorms)
	}
}

func TestBuildReportMultiVersionSnapshot(t *testing.T) {
	t.Parallel()
	r := buildReport(
		[]string{"v1", "v2"},
		[]snapshotRef{
			{Code: "M1", Version: "1.0.0", Kind: "behavioral_rating", Algorithm: "brief2", NormRefs: []normRef{
				{FactorCode: "a", NormTableVersion: "v1"},
				{FactorCode: "b", NormTableVersion: "v2"},
			}},
		},
		"",
	)
	if r.MultiVersionCount != 1 || len(r.MultiVersionSnapshots[0].NormTableVersions) != 2 {
		t.Fatalf("multi=%#v", r.MultiVersionSnapshots)
	}
	if r.UsageCount != 2 {
		t.Fatalf("usage_count=%d want 2", r.UsageCount)
	}
}

func TestBuildReportFilterVersion(t *testing.T) {
	t.Parallel()
	r := buildReport(
		[]string{"v1", "v2", "orphan"},
		[]snapshotRef{
			{Code: "M1", Version: "1.0.0", Kind: "behavioral_rating", Algorithm: "brief2", NormRefs: []normRef{
				{FactorCode: "a", NormTableVersion: "v1"},
				{FactorCode: "b", NormTableVersion: "v2"},
			}},
			{Code: "M2", Version: "1.0.0", Kind: "cognitive", Algorithm: "spm", NormRefs: []normRef{
				{FactorCode: "total", NormTableVersion: "v2"},
			}},
		},
		"v1",
	)
	if r.UsageCount != 1 || r.Usages[0].NormTableVersion != "v1" || len(r.Usages[0].Models) != 1 {
		t.Fatalf("usages=%#v", r.Usages)
	}
	if r.UnreferencedCount != 0 {
		t.Fatalf("unreferenced=%#v want empty for filtered used version", r.UnreferencedNorms)
	}
	if r.MultiVersionCount != 1 {
		t.Fatalf("multi=%#v want M1 which includes v1", r.MultiVersionSnapshots)
	}
	if r.DanglingCount != 0 {
		t.Fatalf("dangling=%#v", r.DanglingRefs)
	}
}

func TestBuildReportFilterUnreferencedVersion(t *testing.T) {
	t.Parallel()
	r := buildReport(
		[]string{"orphan"},
		[]snapshotRef{
			{Code: "M1", Version: "1.0.0", Kind: "behavioral_rating", Algorithm: "brief2", NormRefs: []normRef{
				{FactorCode: "a", NormTableVersion: "other"},
			}},
		},
		"orphan",
	)
	if r.UsageCount != 0 || r.UnreferencedCount != 1 || r.UnreferencedNorms[0] != "orphan" {
		t.Fatalf("report=%#v", r)
	}
}

func TestBuildReportFilterDangling(t *testing.T) {
	t.Parallel()
	r := buildReport(
		[]string{"v1"},
		[]snapshotRef{
			{Code: "M1", Version: "1.0.0", Kind: "behavioral_rating", Algorithm: "brief2", NormRefs: []normRef{
				{FactorCode: "a", NormTableVersion: "ghost"},
				{FactorCode: "b", NormTableVersion: "v1"},
			}},
		},
		"ghost",
	)
	if r.DanglingCount != 1 || r.DanglingRefs[0].NormTableVersion != "ghost" {
		t.Fatalf("dangling=%#v", r.DanglingRefs)
	}
	if r.UsageCount != 1 {
		t.Fatalf("usage_count=%d", r.UsageCount)
	}
}

func TestBuildReportListsDemographicLookupNormsWithPublishedReferences(t *testing.T) {
	t.Parallel()
	r := buildReportWithAssets(
		[]normAsset{
			{TableVersion: "scoped", DemographicLookupFactors: []string{"gec", "bri"}},
			{TableVersion: "generic"},
		},
		[]snapshotRef{{Code: "BRIEF2", Version: "1", Kind: "behavioral_rating", Algorithm: "brief2", NormRefs: []normRef{{FactorCode: "gec", NormTableVersion: "scoped"}}}},
		"",
	)
	if r.DemographicNormCount != 1 || len(r.DemographicNorms) != 1 {
		t.Fatalf("demographic norms = %#v", r.DemographicNorms)
	}
	got := r.DemographicNorms[0]
	if got.NormTableVersion != "scoped" || len(got.FactorCodes) != 2 || len(got.Models) != 1 || got.Models[0].Code != "BRIEF2" {
		t.Fatalf("demographic norm usage = %#v", got)
	}
}
