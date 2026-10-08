package retirement

import (
	"context"
	"errors"
	"slices"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
)

var ErrCoordinatorSQLCASUnqualified = errors.New("historical_coordinator_sql_cas_joint_qualification_missing")

// Readiness contains fixed diagnostic categories only, never authorization.
// In particular, a caller cannot turn this DTO into a CAS capability.
type HistoricalSQLCASReadiness struct {
	WholeFourCopyCoverage    bool
	SQLCandidateCount        uint64
	Required                 []string
	CASAuthorized, DropReady bool
}

func (c *HistoricalCoordinator) SQLCASReadiness() HistoricalSQLCASReadiness {
	r := HistoricalSQLCASReadiness{Required: []string{"production_source_origin_authentication", "joint_original_business_terminal_and_responsibility_qualification", "global_ai_and_inbox_reverse_coverage", "historical_rerun_and_live_writer_fence"}}
	if c == nil {
		return r
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.coverage || c.failed || c.now().Sub(c.started) > c.limits.MaxDuration {
		return r
	}
	r.WholeFourCopyCoverage = true
	for _, candidate := range c.candidates {
		if candidate.OwnerDatabase != "mysql" {
			continue
		}
		r.SQLCandidateCount++
		for _, gap := range candidate.RequiredAdapters {
			if !slices.Contains(r.Required, gap) {
				r.Required = append(r.Required, gap)
			}
		}
		if len(candidate.BlockingReasons) > 0 && !slices.Contains(r.Required, "local_candidate_blocked") {
			r.Required = append(r.Required, "local_candidate_blocked")
		}
	}
	return r
}

// PrepareSQLCAS never accepts a caller-created Candidate, EventEvidence,
// terminal boolean or mutable completion receipt. The current coordinator has
// authentic whole-copy coverage but intentionally lacks production origin,
// joint closure and writer-fence capabilities, so it cannot construct write
// evidence. The infra primitive exists separately as a host-owned CAS only.
// A future joint qualifier must consume those actual opaque capabilities before
// this gate can invoke it; no allow switch or self-approved proof is provided.
func (c *HistoricalCoordinator) PrepareSQLCAS(ctx context.Context, batch *SQLBusinessOwnerBatch) (*sqlevaluation.SQLHistoricalBatchCASPlan, error) {
	if c == nil || ctx == nil || batch == nil || batch.facts == nil {
		return nil, ErrCoordinatorInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.alive(ctx); err != nil {
		return nil, err
	}
	if !c.coverage || c.failed {
		return nil, ErrCoordinatorIncomplete
	}
	return nil, ErrCoordinatorSQLCASUnqualified
}
