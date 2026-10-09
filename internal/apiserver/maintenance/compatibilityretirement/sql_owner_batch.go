package retirement

import (
	"context"
	"slices"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
)

// This page composes one authenticated-current SQL responsibility cycle with
// bounded original business reads. It still does not authenticate an editable
// historical source or prove cross-database business closure or DROP safety.
type SQLBusinessOwnerBatch struct {
	facts          *sqlevaluation.SQLHistoricalOwnerBatch
	responsibility *SQLResponsibilitySnapshot
}

type SQLBatchOwnerResolution struct{ local SQLLocalResolution }

func (*SQLBusinessOwnerBatch) MarshalJSON() ([]byte, error)   { return nil, ErrSourceSerialization }
func (*SQLBusinessOwnerBatch) MarshalBSON() ([]byte, error)   { return nil, ErrSourceSerialization }
func (*SQLBusinessOwnerBatch) String() string                 { return "private bounded SQL business page" }
func (b *SQLBusinessOwnerBatch) GoString() string             { return b.String() }
func (*SQLBatchOwnerResolution) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*SQLBatchOwnerResolution) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*SQLBatchOwnerResolution) String() string               { return "private SQL batch-local resolution" }
func (r *SQLBatchOwnerResolution) GoString() string           { return r.String() }

func PrepareSQLBusinessOwnerBatch(ctx context.Context, responsibility *SQLResponsibilitySnapshot, request sqlevaluation.SQLHistoricalOwnerBatchRequest, limits sqlevaluation.SQLHistoricalOwnerBatchLimits) (*SQLBusinessOwnerBatch, error) {
	if ctx == nil || responsibility == nil || responsibility.cycle == nil {
		return nil, ErrSQLOwnerResolution
	}
	facts, err := sqlevaluation.PrepareSQLHistoricalOwnerBatch(ctx, responsibility.cycle, request, limits)
	if err != nil {
		return nil, err
	}
	return &SQLBusinessOwnerBatch{facts: facts, responsibility: responsibility}, nil
}

func (b *SQLBusinessOwnerBatch) OwnerByAssessment(id uint64) (*sqlevaluation.SQLHistoricalBatchOwnerFacts, error) {
	if b == nil || b.facts == nil {
		return nil, ErrSQLOwnerResolution
	}
	return b.facts.OwnerByAssessment(id)
}
func (b *SQLBusinessOwnerBatch) OwnerByAnswerSheet(id uint64) (*sqlevaluation.SQLHistoricalBatchOwnerFacts, error) {
	if b == nil || b.facts == nil {
		return nil, ErrSQLOwnerResolution
	}
	return b.facts.OwnerByAnswerSheet(id)
}

// Indexed reads do no SQL per source row. The complete-source opaque seal and
// external Mongo closure remain separate inputs of the final coordinator.
func (b *SQLBusinessOwnerBatch) ResolveUntrustedSQLSource(ctx context.Context, source *DecodedSourceEvent) (*SQLBatchOwnerResolution, error) {
	if ctx == nil || b == nil || b.facts == nil || b.responsibility == nil {
		return nil, ErrSQLOwnerResolution
	}
	id, err := sqlSourceAssessment(source)
	if err != nil {
		return nil, err
	}
	facts, err := b.facts.OwnerByAssessment(id)
	if err != nil {
		return nil, err
	}
	local, err := resolveSQLLocalFacts(source, facts.Snapshot())
	if err != nil {
		return nil, err
	}
	current, err := b.responsibility.ForUntrustedSQLSource(ctx, source)
	if err != nil {
		return nil, err
	}
	local.CurrentResponsibilityCount = len(current.Observations)
	for _, reason := range current.BlockingReasons {
		if !slices.Contains(local.BlockingReasons, reason) {
			local.BlockingReasons = append(local.BlockingReasons, reason)
		}
	}
	return &SQLBatchOwnerResolution{local: local}, nil
}

func (r *SQLBatchOwnerResolution) Local() SQLLocalResolution {
	if r == nil {
		return (*SQLOwnerResolution)(nil).Local()
	}
	return (&SQLOwnerResolution{local: r.local}).Local()
}

func (b *SQLBusinessOwnerBatch) Report() sqlevaluation.SQLHistoricalOwnerBatchReport {
	if b == nil || b.facts == nil {
		return (*sqlevaluation.SQLHistoricalOwnerBatch)(nil).Report()
	}
	return b.facts.Report()
}
func (b *SQLBusinessOwnerBatch) ValidateBorrowedSnapshot(ctx context.Context) error {
	if b == nil || b.facts == nil {
		return ErrSQLOwnerResolution
	}
	return b.facts.ValidateBorrowedSnapshot(ctx)
}

// Call once per page with the coordinator's fresh complete SQL cycle. The
// coordinator also compares that cycle to the old full ledger snapshot.
func (b *SQLBusinessOwnerBatch) RecheckBusiness(ctx context.Context, fresh *SQLResponsibilitySnapshot) error {
	if b == nil || b.facts == nil || fresh == nil || fresh.cycle == nil {
		return ErrSQLOwnerResolution
	}
	return b.facts.RecheckBusiness(ctx, fresh.cycle)
}
