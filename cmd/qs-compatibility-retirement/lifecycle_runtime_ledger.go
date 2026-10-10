package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	sqlaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scheduler"
	appaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	mongoscan "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/mongoconsistency"
	currentmq "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	evalmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Private counters describe a complete bounded read of the standard audit's
// supported business matrix and the current AI command ledger. They are not
// full non-target-data coverage, broker/consumer or business-flow acceptance.
// Original references/IDs/bodies remain only in the borrowed read scopes.
type lifecycleRuntimeLedgerCounts struct {
	SQLForward, SQLReverse, MongoForward, MongoReverse               uint64
	SQLHistoricalGaps, SQLBlockingFindings, MongoFindings            uint64
	MongoStandardReferences, MongoRetiredVerified, MongoUnverifiable uint64
	Organizations, Requests, CommandsPending, CommandAttempts        uint64
}

type lifecycleRuntimeLedgerObservation struct {
	self                       *lifecycleRuntimeLedgerObservation
	host                       *lifecycleFixedHost
	owner                      *lifecyclePreparationOwner
	windowStart, requestSHA256 string
	counts                     lifecycleRuntimeLedgerCounts
}

func (*lifecycleRuntimeLedgerObservation) MarshalJSON() ([]byte, error) {
	return nil, lifecycleError("lifecycle_private_runtime_serialization_forbidden")
}

// The standard Mongo scanner consumes the existing immutable Outcome contract.
// Conversion reuses the current application codec, not a second decoder.
type lifecycleAuditOutcomeFacts struct{ source domainoutcome.Repository }

func (r lifecycleAuditOutcomeFacts) FindByID(ctx context.Context, id meta.ID) (*evaluationfact.Record, error) {
	v, e := r.source.FindByID(ctx, id)
	if e != nil {
		return nil, e
	}
	if v == nil {
		return nil, evaluationfact.ErrNotFound
	}
	return evaloutcome.FactRecord(v), nil
}
func (r lifecycleAuditOutcomeFacts) FindByAssessmentID(ctx context.Context, id meta.ID) (*evaluationfact.Record, error) {
	v, e := r.source.FindByAssessmentID(ctx, id)
	if e != nil {
		return nil, e
	}
	if v == nil {
		return nil, evaluationfact.ErrNotFound
	}
	return evaloutcome.FactRecord(v), nil
}

func lifecycleAddLedgerCount(dst *uint64, value uint64) error {
	if dst == nil || math.MaxUint64-*dst < value {
		return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
	}
	*dst += value
	return nil
}

// Always request the actual final empty page, including after a full page or
// the reader's upper-bound exhausted marker. No saved checkpoint is used.
func lifecycleScanStandardSQL(ctx context.Context, reader evaluationconsistency.CycleReader, out *lifecycleRuntimeLedgerCounts) error {
	if ctx == nil || ctx.Err() != nil || reader == nil || out == nil {
		return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
	}
	upper, e := reader.BusinessUpperBound(ctx)
	if e != nil {
		return e
	}
	reverseUpper, e := reader.OutboxUpperBound(ctx)
	if e != nil {
		return e
	}
	for reverse := 0; reverse < 2; reverse++ {
		bound, after := upper, uint64(0)
		if reverse == 1 {
			bound = reverseUpper
		}
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			q, cancel := context.WithTimeout(ctx, 10*time.Second)
			var result sqlaudit.AuditBatchResult
			if reverse == 0 {
				var batch evaluationconsistency.Batch
				batch, e = reader.ReadBatchTo(q, after, bound, 100)
				if e == nil {
					result = sqlaudit.SummarizeReadOnlyBatch(batch, time.Now())
				}
			} else {
				var batch evaluationconsistency.ReverseBatch
				batch, e = reader.ReadOutboxBatch(q, after, bound, 100)
				result = sqlaudit.AuditBatchResult{Scanned: batch.Scanned, Detected: len(batch.Conflicts), NextCursor: batch.NextCursor, CycleComplete: batch.CycleComplete}
			}
			if e == nil {
				e = q.Err()
			}
			cancel()
			if e != nil {
				return e
			}
			if result.Scanned < 0 || result.Scanned > 100 || result.Detected < result.HistoricalGaps || result.HistoricalGaps < 0 {
				return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
			}
			if result.Scanned == 0 {
				if !result.CycleComplete || result.NextCursor != 0 || result.Detected != 0 {
					return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
				}
				break
			}
			if result.NextCursor <= after || result.NextCursor > bound {
				return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
			}
			dst := &out.SQLForward
			if reverse == 1 {
				dst = &out.SQLReverse
			}
			if e = lifecycleAddLedgerCount(dst, uint64(result.Scanned)); e != nil {
				return e
			}
			if e = lifecycleAddLedgerCount(&out.SQLHistoricalGaps, uint64(result.HistoricalGaps)); e != nil {
				return e
			}
			if e = lifecycleAddLedgerCount(&out.SQLBlockingFindings, uint64(result.Detected-result.HistoricalGaps)); e != nil {
				return e
			}
			after = result.NextCursor
		}
	}
	return ctx.Err()
}

func lifecycleScanStandardMongo(ctx context.Context, scanner appaudit.Scanner, out *lifecycleRuntimeLedgerCounts) error {
	if ctx == nil || ctx.Err() != nil || scanner == nil || out == nil {
		return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
	}
	// Capture all bounds before scanning; the same native snapshot is retained.
	phases := []appaudit.Phase{appaudit.PhaseAnswerSheetOutbox, appaudit.PhaseOutboxAnswerSheet, appaudit.PhaseGenerationRun, appaudit.PhaseGeneratedTerminal, appaudit.PhaseRetryOutbox, appaudit.PhaseModelRelease, appaudit.PhasePublishedModelRuntime}
	bounds := make(map[appaudit.Phase]uint64, len(phases))
	upper, e := scanner.OutboxUpperBound(ctx, 3*time.Second)
	if e != nil {
		return e
	}
	upper = append([]byte(nil), upper...)
	for _, phase := range phases {
		if phase != appaudit.PhaseOutboxAnswerSheet {
			bounds[phase], e = scanner.UpperBound(ctx, phase, 3*time.Second)
			if e != nil {
				return e
			}
		}
	}
	for _, phase := range phases {
		after, cursor := uint64(0), []byte(nil)
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			request := appaudit.BatchRequest{Phase: phase, AfterID: after, UpperBound: bounds[phase], OutboxCursor: append([]byte(nil), cursor...), OutboxUpperBound: append([]byte(nil), upper...), Limit: 200, MaxTime: 3 * time.Second}
			batch, e := scanner.ScanBatch(ctx, request)
			if e != nil {
				return e
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if batch.Scanned < 0 || batch.Scanned > request.Limit {
				return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
			}
			if batch.Scanned == 0 {
				if !batch.Exhausted || batch.NextID != 0 || len(batch.NextOutboxCursor) != 0 || len(batch.Findings) != 0 {
					return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
				}
				break
			}
			dst := &out.MongoForward
			if phase == appaudit.PhaseOutboxAnswerSheet {
				if len(batch.NextOutboxCursor) == 0 || len(batch.NextOutboxCursor) > 1024 || bson.Raw(batch.NextOutboxCursor).Validate() != nil || bytes.Equal(cursor, batch.NextOutboxCursor) {
					return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
				}
				cursor = append([]byte(nil), batch.NextOutboxCursor...)
				dst = &out.MongoReverse
			} else {
				if batch.NextID <= after || batch.NextID > bounds[phase] {
					return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
				}
				after = batch.NextID
			}
			if e = lifecycleAddLedgerCount(dst, uint64(batch.Scanned)); e != nil {
				return e
			}
			if e = lifecycleAddLedgerCount(&out.MongoFindings, uint64(len(batch.Findings))); e != nil {
				return e
			}
			for class, count := range batch.EvidenceClasses {
				if count < 0 {
					return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
				}
				var dst *uint64
				switch class {
				case "standard_reference":
					dst = &out.MongoStandardReferences
				case "retired_verified":
					dst = &out.MongoRetiredVerified
				case "unverifiable":
					dst = &out.MongoUnverifiable
				default:
					return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
				}
				if e = lifecycleAddLedgerCount(dst, uint64(count)); e != nil {
					return e
				}
			}
		}
	}
	return ctx.Err()
}

// The union includes both ledger directions and the request owner. Invalid
// organizations and orphan commands cannot disappear behind the statistics JOIN.
const lifecycleMQOrganizations = `SELECT organization_id FROM ai_bridge_requests
 UNION SELECT organization_id FROM ai_messaging_operations WHERE retired=FALSE AND kind IN (1,2,3)
 UNION SELECT organization_id FROM ai_messaging_outbox WHERE producer='qs-server' AND destination='qs-ai' AND kind IN (1,2,3)`

func lifecycleReadCurrentMQ(ctx context.Context, tx *sql.Tx, out *lifecycleRuntimeLedgerCounts) error {
	if ctx == nil || ctx.Err() != nil || tx == nil || out == nil {
		return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
	}
	var upper int64
	var invalid uint64
	if e := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(organization_id),0),COALESCE(SUM(organization_id IS NULL OR organization_id<=0),0) FROM (`+lifecycleMQOrganizations+`) scoped`).Scan(&upper, &invalid); e != nil {
		return e
	}
	if invalid != 0 || upper < 0 {
		return currentmq.ErrMessagingLedgerIntegrity
	}
	for after := int64(0); ; {
		rows, e := tx.QueryContext(ctx, `SELECT organization_id FROM (`+lifecycleMQOrganizations+`) scoped WHERE organization_id>? AND organization_id<=? ORDER BY organization_id ASC LIMIT 100`, after, upper)
		if e != nil {
			return e
		}
		var organizations []int64
		for rows.Next() {
			var org int64
			if e = rows.Scan(&org); e != nil {
				break
			}
			if org <= after || org > upper || len(organizations) >= 100 {
				e = currentmq.ErrMessagingLedgerIntegrity
				break
			}
			organizations = append(organizations, org)
			after = org
		}
		e = errors.Join(e, rows.Err(), rows.Close())
		if e != nil {
			return e
		}
		if len(organizations) == 0 {
			break
		}
		for _, org := range organizations {
			v, e := currentmq.ReadCurrentMessagingOrganization(ctx, tx, org)
			if e != nil {
				return e
			}
			for _, item := range []struct {
				dst   *uint64
				count uint64
			}{{&out.Organizations, 1}, {&out.Requests, v.Requests}, {&out.CommandsPending, v.CommandsPending}, {&out.CommandAttempts, v.CommandAttempts}} {
				if e = lifecycleAddLedgerCount(item.dst, item.count); e != nil {
					return e
				}
			}
		}
	}
	return ctx.Err()
}

func (h *lifecycleFixedHost) observeRuntimeLedgers(ctx context.Context, r lifecycleRequest) (result error) {
	if ctx == nil || ctx.Err() != nil || h == nil || h.owner == nil || h.owner.originalConn == nil || h.owner.originalMongo == nil || h.owner.originalDB == nil || h.services == nil || h.services.window == nil || !h.services.identity.matches(r) || h.api == nil || h.api.acceptance == nil || h.api.acceptance.self != h.api.acceptance || h.api.acceptance.owner != h.api || h.runtimeLedgers != nil {
		return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
	}
	d, e := h.services.window.Diagnostic(ctx)
	if e != nil || d.RecoverySHA256 != "" || d.Binding != lifecycleWindowBinding(r) {
		return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
	}
	var counts lifecycleRuntimeLedgerCounts
	// Scoped function returns only after actual same-session abort response and
	// original SQL rollback; unknown cleanup cannot produce this observation.
	e = func() (result error) {
		tx, e := h.owner.originalConn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if e != nil {
			return e
		}
		defer func() {
			if e := tx.Rollback(); e != nil && result == nil {
				result = lifecycleError("lifecycle_acceptance_snapshot_cleanup_failed")
			}
		}()
		g, e := gorm.Open(gormmysql.New(gormmysql.Config{Conn: tx, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
		if e != nil {
			return e
		}
		session, e := h.owner.originalMongo.StartSession()
		if e != nil {
			return e
		}
		defer session.EndSession(context.Background())
		if e = session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())); e != nil {
			return e
		}
		defer func() {
			q, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			native := lifecycleAbortComparisonMongo(q, h.owner.originalMongo, session)
			local := session.AbortTransaction(q)
			if (native != nil || local != nil) && result == nil {
				result = lifecycleError("lifecycle_acceptance_snapshot_cleanup_failed")
			}
		}()
		paired := mongo.NewSessionContext(hostmysql.WithTx(ctx, g), session)
		reader, ok := evalmysql.NewConsistencyReadModel(g).(evaluationconsistency.CycleReader)
		if !ok {
			return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
		}
		if e = lifecycleScanStandardSQL(paired, reader, &counts); e != nil {
			return e
		}
		scanner := mongoscan.NewScanner(h.owner.originalDB, nil).WithOutcomeFacts(lifecycleAuditOutcomeFacts{source: evalmysql.NewOutcomeRepository(g)})
		if e = lifecycleScanStandardMongo(paired, scanner, &counts); e != nil {
			return e
		}
		return lifecycleReadCurrentMQ(paired, tx, &counts)
	}()
	if e != nil {
		return e
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	after, e := h.services.window.Diagnostic(ctx)
	if e != nil || after.StartSHA256 != d.StartSHA256 || after.RecoverySHA256 != "" || after.Binding != d.Binding {
		return lifecycleError("lifecycle_runtime_ledger_scan_rejected")
	}
	o := &lifecycleRuntimeLedgerObservation{host: h, owner: h.owner, windowStart: d.StartSHA256, requestSHA256: r.requestSHA256, counts: counts}
	o.self, h.runtimeLedgers = o, o
	if counts.SQLBlockingFindings != 0 || counts.MongoFindings != 0 {
		return lifecycleError("lifecycle_standard_audit_findings_present")
	}
	return nil
}
