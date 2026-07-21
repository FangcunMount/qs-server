package interpretation

// CatalogSourceAssociation carries source-side identity used for IR-R002/R015
// catalog↔source association checks. Org is compared only when HasOrgID is true.
type CatalogSourceAssociation struct {
	AssessmentID uint64
	OrgID        int64
	HasOrgID     bool
	TesteeID     uint64
}

// MismatchedAssociationFields returns catalog/source fields that disagree under
// the IR-R002 fail-closed rules shared by read paths and catalog reconcile.
func MismatchedAssociationFields(catalog ReportCatalogPO, source CatalogSourceAssociation) []string {
	var fields []string
	if catalog.AssessmentID != source.AssessmentID {
		fields = append(fields, "assessment_id")
	}
	if source.HasOrgID && catalog.OrgID != source.OrgID {
		fields = append(fields, "org_id")
	}
	if catalog.TesteeID != source.TesteeID {
		fields = append(fields, "testee_id")
	}
	return fields
}

func mismatchedAssociationFields(catalog ReportCatalogPO, source catalogSourceEnvelope) []string {
	return MismatchedAssociationFields(catalog, CatalogSourceAssociation{
		AssessmentID: source.AssessmentID,
		OrgID:        source.OrgID,
		HasOrgID:     source.HasOrgID,
		TesteeID:     source.TesteeID,
	})
}
