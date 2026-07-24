// Package artifactassociation owns the stable catalog-to-artifact association
// contract shared by online reads and offline reconciliation.
package artifactassociation

type Status string

const (
	StatusValid    Status = "valid"
	StatusMissing  Status = "missing"
	StatusDangling Status = "dangling"
	StatusMismatch Status = "mismatch"
)

type Field string

const (
	FieldAssessmentID Field = "assessment_id"
	FieldOrgID        Field = "org_id"
	FieldTesteeID     Field = "testee_id"
	FieldOutcomeID    Field = "outcome_id"
	FieldGenerationID Field = "generation_id"
)

type Association struct {
	AssessmentID uint64
	OrgID        int64
	TesteeID     uint64
	OutcomeID    uint64
	GenerationID uint64

	HasOrgID        bool
	HasOutcomeID    bool
	HasGenerationID bool
}

type Result struct {
	Status   Status
	Mismatch []Field
}

type Validator struct{}

func NewValidator() Validator { return Validator{} }

// Validate is fail-closed for the identity fields that every source must own.
// Outcome and generation correlation are compared when the catalog has already
// been upgraded to carry those fields; this preserves reads during the
// catalog metadata backfill while making every new projection strict.
func (Validator) Validate(catalog, source Association) Result {
	fields := make([]Field, 0, 5)
	if catalog.AssessmentID != source.AssessmentID {
		fields = append(fields, FieldAssessmentID)
	}
	if !source.HasOrgID || catalog.OrgID != source.OrgID {
		fields = append(fields, FieldOrgID)
	}
	if catalog.TesteeID != source.TesteeID {
		fields = append(fields, FieldTesteeID)
	}
	if catalog.HasOutcomeID && (!source.HasOutcomeID || catalog.OutcomeID != source.OutcomeID) {
		fields = append(fields, FieldOutcomeID)
	}
	if catalog.HasGenerationID && (!source.HasGenerationID || catalog.GenerationID != source.GenerationID) {
		fields = append(fields, FieldGenerationID)
	}
	if len(fields) > 0 {
		return Result{Status: StatusMismatch, Mismatch: fields}
	}
	return Result{Status: StatusValid}
}
