package retirement

import (
	"context"
	"strconv"
	"time"
)

// Event pages accept only the actual opaque host-owned SQL and Mongo batches.
// Missing adapters create explicit blocked candidates; mutable terminal/complete
// DTOs are never accepted in place of those capabilities.
func (c *HistoricalCoordinator) QualifyPage(ctx context.Context, p *HistoricalSourcePage, sql *SQLBusinessOwnerBatch, mongo *MongoHistoricalOwnerBatch) error {
	if c == nil {
		return ErrCoordinatorInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.alive(ctx); err != nil {
		return err
	}
	if err := c.pageValid(p); err != nil {
		return err
	}
	for _, r := range p.rows {
		if r.event == nil {
			return ErrCoordinatorPage
		}
	}
	if sql != nil {
		if err := sql.ValidateBorrowedSnapshot(ctx); err != nil {
			return ErrCoordinatorInvalid
		}
	}
	if mongo != nil {
		if sql == nil || mongo.sql != sql.facts {
			return ErrCoordinatorInvalid
		}
		if err := mongo.ValidateBorrowedSnapshot(ctx); err != nil {
			return ErrCoordinatorInvalid
		}
	}
	rows := make([]HistoricalCandidate, 0, len(p.rows))
	for _, row := range p.rows {
		if err := c.alive(ctx); err != nil {
			return err
		}
		facts, err := row.event.Facts()
		if err != nil {
			return err
		}
		candidate := coordinatorEventCandidate(facts)
		if sql == nil {
			candidate.BlockingReasons = append(candidate.BlockingReasons, "actual_sql_business_batch_missing")
		}
		if mongo == nil {
			candidate.BlockingReasons = append(candidate.BlockingReasons, "actual_mongo_business_batch_missing")
		}
		if facts.Source.Database == "mysql" {
			if sql != nil {
				qualified, e := sql.ResolveUntrustedSQLSource(ctx, facts)
				if e != nil {
					candidate.BlockingReasons = append(candidate.BlockingReasons, "sql_original_business_or_responsibility_rejected")
				} else {
					local := qualified.Local()
					candidate.LocalQualified = local.OwnerLocalTerminal && len(local.BlockingReasons) == 0
					candidate.HistoricalGaps = append(candidate.HistoricalGaps, local.Gaps...)
					candidate.BlockingReasons = append(candidate.BlockingReasons, local.BlockingReasons...)
					candidate.BusinessBaselineSHA256 = sql.Report().BusinessRowsSHA256
					candidate.ActualOriginalRun = local.OriginalRun
					candidate.AuthorizationRun = local.AuthorizationRun
					candidate.ExecutionRun = local.ExecutionRun
				}
			}
			if mongo != nil {
				view, e := mongo.ResponsibilitiesForSource(ctx, row.event)
				if e != nil {
					candidate.BlockingReasons = append(candidate.BlockingReasons, "mongo_original_downstream_or_responsibility_rejected")
				} else {
					candidate.BlockingReasons = append(candidate.BlockingReasons, view.BlockingReasons...)
				}
			}
			candidate.RequiredAdapters = append(candidate.RequiredAdapters, "sql_stable_original_business_binding_writer_adapter")
		} else {
			if mongo != nil {
				qualified, e := mongo.ResolveSource(ctx, row.event)
				if e != nil {
					candidate.BlockingReasons = append(candidate.BlockingReasons, "mongo_original_business_or_responsibility_rejected")
				} else {
					local := qualified.Local()
					candidate.LocalQualified = local.OwnerLocalTerminal && len(local.BlockingReasons) == 0
					candidate.HistoricalGaps = append(candidate.HistoricalGaps, local.Gaps...)
					candidate.BlockingReasons = append(candidate.BlockingReasons, local.BlockingReasons...)
					candidate.BusinessBindingSHA256 = local.BusinessBindingSHA256
					candidate.ActualOriginalRun = local.OriginalRun
					candidate.BusinessBaselineSHA256 = mongo.Report().BusinessRowsSHA256
				}
			}
			// Frozen SQL responsibilities consume only genuine SQL four-type sources.
			// Never fake evaluation DTOs to infer held/DL closure for Mongo two types.
			candidate.RequiredAdapters = append(candidate.RequiredAdapters, "mongo_original_two_types_to_sql_held_deadletter_and_inbox_adapter")
		}
		if sql != nil && (sql.responsibility.Report().Unknown > 0 || sql.responsibility.Report().Blocking > 0) {
			candidate.BlockingReasons = append(candidate.BlockingReasons, "global_sql_orphan_unknown_or_conflicting_responsibility")
		}
		if mongo != nil {
			candidate.BlockingReasons = append(candidate.BlockingReasons, mongo.global.Report().BlockingReasons...)
			candidate.RequiredAdapters = append(candidate.RequiredAdapters, mongo.global.Report().CoverageGaps...)
		}
		coordinatorClassify(&candidate)
		rows = append(rows, candidate)
	}
	if sql != nil {
		if err := sql.ValidateBorrowedSnapshot(ctx); err != nil {
			return ErrCoordinatorInvalid
		}
	}
	if mongo != nil {
		if err := mongo.ValidateBorrowedSnapshot(ctx); err != nil {
			return ErrCoordinatorInvalid
		}
	}
	if err := c.alive(ctx); err != nil {
		return err
	}
	if err := c.pageValid(p); err != nil {
		return err
	}
	receipt := HistoricalCoordinatorPageReceipt{}
	if sql != nil {
		receipt.SQLBusinessBaselineSHA256 = sql.Report().BusinessRowsSHA256
	}
	if mongo != nil {
		receipt.MongoBusinessBaselineSHA256 = mongo.Report().BusinessRowsSHA256
	}
	return c.consume(p, rows, receipt)
}

func coordinatorEventCandidate(v *DecodedSourceEvent) HistoricalCandidate {
	c := HistoricalCandidate{Source: v.Source, OriginalID: v.EventID, EventType: v.EventType, OrganizationID: strconv.FormatUint(v.OrgID, 10), ContentDigest: v.ContentDigest, OriginalRunID: v.OriginalRun.RunID, HistoricalGaps: append([]string(nil), v.ResolverGaps...), OriginalRunMissing: append([]string(nil), v.OriginalRun.Missing...), RequiredAdapters: coordinatorRequiredAdapters()}
	if v.OriginalRun.Attempt != nil {
		n := *v.OriginalRun.Attempt
		c.OriginalAttempt = &n
	}
	switch v.EventType {
	case "evaluation.requested", "evaluation.retry.requested", "evaluation.failed":
		c.OwnerDatabase = "mysql"
		c.OwnerObject = "assessment"
		c.OwnerID = v.BusinessIDs["assessment_id"]
	case "evaluation.outcome.committed":
		c.OwnerDatabase = "mysql"
		c.OwnerObject = "evaluation_outcome"
		c.OwnerID = v.BusinessIDs["outcome_id"]
	case "answersheet.submitted":
		c.OwnerDatabase = "mongodb"
		c.OwnerObject = "answersheets"
		c.OwnerID = v.BusinessIDs["answersheet_id"]
	case "interpretation.report.generated":
		c.OwnerDatabase = "mongodb"
		c.OwnerObject = "report_generations"
		c.OwnerID = v.BusinessIDs["generation_id"]
	}
	return c
}

// AI local resolution needs an actual borrowed locking transaction. It is
// independent of the RR-RO event epoch and never turns delivered into accepted.
// Both physical source rows yield candidates/counts, sharing one command owner.
func (c *HistoricalCoordinator) QualifyAIPage(ctx context.Context, p *HistoricalSourcePage, resolver *AILocalResolver) error {
	return c.qualifyAIPage(ctx, p, resolver, nil)
}

// QualifyAIReadOnlyPage observes the actual authenticated point graph within
// the original RR-RO epoch. It grants no RW/CAS or external closure capability.
func (c *HistoricalCoordinator) QualifyAIReadOnlyPage(ctx context.Context, p *HistoricalSourcePage, resolver *AIReadOnlyResolver) error {
	if resolver == nil || resolver.owner != c {
		return ErrAILocalBinding
	}
	return c.qualifyAIPage(ctx, p, nil, resolver)
}

func (c *HistoricalCoordinator) qualifyAIPage(ctx context.Context, p *HistoricalSourcePage, resolver *AILocalResolver, readonly *AIReadOnlyResolver) error {
	if c == nil {
		return ErrCoordinatorInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.alive(ctx); err != nil {
		return err
	}
	if err := c.pageValid(p); err != nil {
		return err
	}
	for _, r := range p.rows {
		if r.event != nil || r.bridge == nil {
			return ErrCoordinatorPage
		}
	}
	if readonly != nil {
		if readonly.owner != c || readonly.validate(ctx) != nil {
			return ErrAILocalTransaction
		}
		resolver = readonly.inner
	}
	if resolver != nil && (resolver.binding.SourceSHA != c.binding.SourceSHA || resolver.binding.OperationID != c.binding.OperationID || resolver.binding.BridgeBoundary != c.copies[1].Expected.Boundary || resolver.binding.LegacyBoundary != c.copies[2].Expected.Boundary) {
		return ErrCoordinatorInvalid
	}
	var rows []HistoricalCandidate
	for _, r := range p.rows {
		if err := c.alive(ctx); err != nil {
			return err
		}
		bridge, err := r.bridge.Facts()
		if err != nil {
			return err
		}
		var legacy *DecodedAICommand
		if r.legacy != nil {
			legacy, err = r.legacy.Facts()
			if err != nil {
				return err
			}
		}
		var summary AILocalSummary
		var localError bool
		if readonly != nil {
			q, e := readonly.resolve(ctx, bridge, legacy)
			// A failed point read has no complete baseline. Refuse this page
			// rather than hiding drift behind an empty blocked-candidate hash.
			if e != nil {
				return e
			}
			summary = q.Summary().AILocalSummary
		} else if resolver != nil {
			q, e := resolver.Resolve(ctx, bridge, legacy)
			localError = e != nil
			if e == nil {
				summary = q.Summary()
			}
		}
		sources := []*DecodedAICommand{bridge}
		if legacy != nil {
			sources = append(sources, legacy)
		}
		for _, source := range sources {
			candidate := HistoricalCandidate{Source: source.Source, OriginalID: source.CommandID, EventType: "ai.command." + source.SourceKind, OwnerDatabase: "mysql", OwnerObject: "ai_messaging_operations", OwnerID: source.CommandID, OrganizationID: source.OrganizationID, ContentDigest: source.PayloadBytesDigest, WriterPayloadDigest: source.WriterPayloadDigest, MappedMessagingBodySHA256: source.Transport.MessagingBodySHA256, AIRequestID: source.RequestID, AISubjectID: source.SubjectID, AIResourceID: source.ResourceID, AISourceAttempts: source.Transport.SourceAttempts, AIHandoffBudgetFloor: source.Transport.HandoffBudgetFloor, AIHandoffBudgetExhausted: source.Transport.HandoffBudgetExhausted, HistoricalGaps: append([]string(nil), source.ResolverGaps...), RequiredAdapters: append(coordinatorRequiredAdapters(), "qs_ai_all_original_runs_model_calls_jobs_leases_and_messages_closure", "qs_server_ai_full_reverse_authenticated_message_coverage", "post_retirement_expected_pk_and_metadata_recheck")}
			if resolver == nil {
				candidate.BlockingReasons = append(candidate.BlockingReasons, "actual_ai_local_resolver_missing")
			} else if localError {
				candidate.BlockingReasons = append(candidate.BlockingReasons, "ai_original_business_or_responsibility_rejected")
			} else {
				candidate.LocalQualified = summary.LocalQualified
				candidate.BusinessBaselineSHA256 = summary.BaselineSHA256
				candidate.AIAdmissionRevision = resolver.binding.AdmissionRevision
				candidate.HistoricalGaps = append(candidate.HistoricalGaps, summary.Gaps...)
			}
			coordinatorClassify(&candidate)
			rows = append(rows, candidate)
		}
	}
	if err := c.alive(ctx); err != nil {
		return err
	}
	if err := c.pageValid(p); err != nil {
		return err
	}
	if readonly != nil {
		if err := readonly.validate(ctx); err != nil {
			return err
		}
	}
	return c.consume(p, rows, HistoricalCoordinatorPageReceipt{})
}
func coordinatorClassify(v *HistoricalCandidate) {
	v.HistoricalGaps = coordinatorReasons(v.HistoricalGaps)
	v.BlockingReasons = coordinatorReasons(v.BlockingReasons)
	v.RequiredAdapters = coordinatorReasons(v.RequiredAdapters)
	if len(v.BlockingReasons) > 0 {
		v.LocalQualified = false
	}
	switch {
	case !v.LocalQualified:
		v.LocalClassification = "blocked"
	case len(v.HistoricalGaps) > 0:
		v.LocalClassification = "candidate_historical_gap_requires_joint_closure"
	default:
		v.LocalClassification = "candidate_local_verified_requires_joint_closure"
	}
}
func (c *HistoricalCoordinator) consume(p *HistoricalSourcePage, rows []HistoricalCandidate, receipt HistoricalCoordinatorPageReceipt) error {
	var keys []verifiedSourceKey
	for _, r := range p.rows {
		keys = append(keys, r.keys...)
	}
	if len(rows) != len(keys) {
		return ErrCoordinatorPage
	}
	seen := map[verifiedSourceKey]bool{}
	for i, key := range keys {
		if !c.remaining[key] || seen[key] {
			return ErrCoordinatorPage
		}
		seen[key] = true
		actual, err := sourceAuthKey(rows[i].Source.Database, rows[i].Source.Object, rows[i].Source.PrimaryKeySHA256)
		if err != nil || actual != key {
			return ErrCoordinatorPage
		}
	}
	// All physical source identities are checked before sharing immutable lists.
	// Cache saturation keeps the original lists; cache faults consume no rows.
	if err := c.shareCandidateProfiles(rows); err != nil {
		c.failed = true
		return err
	}
	receipt.Sequence = p.sequence
	receipt.IssuedAt = p.issued.UTC().Truncate(time.Millisecond)
	receipt.ConsumedAt = c.now().UTC().Truncate(time.Millisecond)
	receipt.PageTTLMilliseconds = c.limits.PageTTL.Milliseconds()
	for i, key := range keys {
		if receipt.FirstPrimaryKeyBySourceSHA256[key.object] == "" {
			receipt.FirstPrimaryKeyBySourceSHA256[key.object] = rows[i].Source.PrimaryKeySHA256
		}
		receipt.LastPrimaryKeyBySourceSHA256[key.object] = rows[i].Source.PrimaryKeySHA256
		receipt.Records[key.object]++
		c.consumed[key.object]++
		delete(c.remaining, key)
	}
	receipt.FirstPrimaryKeySHA256 = rows[0].Source.PrimaryKeySHA256
	receipt.LastPrimaryKeySHA256 = rows[len(rows)-1].Source.PrimaryKeySHA256
	receipt.CandidateSHA256 = coordinatorCandidateHash(rows)
	// Retain these private, bounded page values without copying earlier pages.
	for i := range rows {
		c.candidates = append(c.candidates, &rows[i])
	}
	c.pages = append(c.pages, receipt)
	p.consumed = true
	p.rows = nil
	c.pending = nil
	return nil
}
