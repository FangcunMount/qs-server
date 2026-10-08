package retirement

import (
	"context"
	"errors"
	"slices"
)

var ErrCoordinatorMongoCASUnqualified = errors.New("historical_coordinator_mongo_cas_joint_qualification_missing")

// This diagnostic DTO cannot authorize writes.
type HistoricalMongoCASReadiness struct {
	WholeFourCopyCoverage    bool
	MongoCandidateCount      uint64
	Required                 []string
	CASAuthorized, DropReady bool
}

func (c *HistoricalCoordinator) MongoCASReadiness() HistoricalMongoCASReadiness {
	r := HistoricalMongoCASReadiness{Required: []string{"production_source_origin_authentication", "joint_original_business_terminal_and_responsibility_qualification", "global_ai_and_inbox_reverse_coverage", "historical_rerun_and_live_writer_fence", "mongo_absent_range_fresh_recheck"}}
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
		if candidate.OwnerDatabase != "mongodb" {
			continue
		}
		r.MongoCandidateCount++
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

// Whole four-copy EOF proves source membership, not production origin or joint
// closure. Only private coordinator capabilities may eventually construct a
// proof. No mutable Candidate, terminal boolean or readiness DTO is accepted.
func (c *HistoricalCoordinator) PrepareMongoCAS(ctx context.Context, batch *MongoHistoricalOwnerBatch) (*MongoHistoricalBatchCASPlan, error) {
	if c == nil || ctx == nil || batch == nil || batch.global == nil {
		return nil, ErrCoordinatorInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.alive(ctx); e != nil {
		return nil, e
	}
	if !c.coverage || c.failed {
		return nil, ErrCoordinatorIncomplete
	}
	return nil, ErrCoordinatorMongoCASUnqualified
}
