package interpretation

import (
	"testing"
)

func TestRequiredReportCatalogIndexNamesContract(t *testing.T) {
	t.Parallel()

	want := []string{
		"uk_report_catalog_assessment",
		"idx_report_catalog_org_sort",
		"idx_report_catalog_testee_sort",
		"idx_report_catalog_org_model_sort",
		"idx_report_catalog_org_risk_sort",
		"idx_report_catalog_testee_model_sort",
		"idx_report_catalog_testee_risk_sort",
	}
	got := RequiredReportCatalogIndexNames()
	if len(got) != len(want) {
		t.Fatalf("index count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	models := reportCatalogIndexModels()
	if len(models) != len(want) {
		t.Fatalf("reportCatalogIndexModels count = %d, want %d", len(models), len(want))
	}
}

func TestHasAssociationMismatchUsesSharedComparator(t *testing.T) {
	t.Parallel()

	catalog := ReportCatalogPO{AssessmentID: 1, OrgID: 10, TesteeID: 100}
	if HasAssociationMismatch(catalog, CatalogSourceAssociation{AssessmentID: 1, OrgID: 10, HasOrgID: true, TesteeID: 100}) {
		t.Fatal("expected aligned association")
	}
	if !HasAssociationMismatch(catalog, CatalogSourceAssociation{AssessmentID: 2, OrgID: 10, HasOrgID: true, TesteeID: 100}) {
		t.Fatal("expected assessment mismatch")
	}
	if !HasAssociationMismatch(catalog, CatalogSourceAssociation{AssessmentID: 1, OrgID: 11, HasOrgID: true, TesteeID: 100}) {
		t.Fatal("expected org mismatch")
	}
	if !HasAssociationMismatch(catalog, CatalogSourceAssociation{AssessmentID: 1, HasOrgID: false, TesteeID: 101}) {
		t.Fatal("expected testee mismatch")
	}
}

func TestCatalogDriftKindConstants(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{
		CatalogDriftMissing,
		CatalogDriftDangling,
		CatalogDriftAssociationMismatch,
		CatalogDriftWrongWinner,
	} {
		if kind == "" {
			t.Fatal("drift kind must not be empty")
		}
	}
}
