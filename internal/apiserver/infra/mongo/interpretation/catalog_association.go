package interpretation

import artifactassociation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/artifactassociation"

// CatalogSourceAssociation carries source-side identity used for IR-R002/R015
// catalog↔source association checks.
type CatalogSourceAssociation struct {
	AssessmentID    uint64
	OrgID           int64
	HasOrgID        bool
	TesteeID        uint64
	OutcomeID       uint64
	HasOutcomeID    bool
	GenerationID    uint64
	HasGenerationID bool
}

// MismatchedAssociationFields returns catalog/source fields that disagree under
// the IR-R002 fail-closed rules shared by read paths and catalog reconcile.
func MismatchedAssociationFields(catalog ReportCatalogPO, source CatalogSourceAssociation) []string {
	result := artifactassociation.NewValidator().Validate(
		artifactassociation.Association{
			AssessmentID: catalog.AssessmentID, OrgID: catalog.OrgID, HasOrgID: true,
			TesteeID:  catalog.TesteeID,
			OutcomeID: catalog.OutcomeID, HasOutcomeID: catalog.OutcomeID != 0,
			GenerationID: catalog.GenerationID, HasGenerationID: catalog.GenerationID != 0,
		},
		artifactassociation.Association{
			AssessmentID: source.AssessmentID, OrgID: source.OrgID, HasOrgID: source.HasOrgID,
			TesteeID:  source.TesteeID,
			OutcomeID: source.OutcomeID, HasOutcomeID: source.HasOutcomeID,
			GenerationID: source.GenerationID, HasGenerationID: source.HasGenerationID,
		},
	)
	fields := make([]string, 0, len(result.Mismatch))
	for _, field := range result.Mismatch {
		fields = append(fields, string(field))
	}
	return fields
}

func mismatchedAssociationFields(catalog ReportCatalogPO, source catalogSourceEnvelope) []string {
	return MismatchedAssociationFields(catalog, CatalogSourceAssociation{
		AssessmentID:    source.AssessmentID,
		OrgID:           source.OrgID,
		HasOrgID:        source.HasOrgID,
		TesteeID:        source.TesteeID,
		OutcomeID:       source.OutcomeID,
		HasOutcomeID:    source.HasOutcomeID,
		GenerationID:    source.GenerationID,
		HasGenerationID: source.HasGenerationID,
	})
}
