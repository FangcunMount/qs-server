package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	reader "github.com/FangcunMount/qs-server/internal/pkg/runtimefactsreader"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This is only a summary of the original Worker's registered consumers and
// visible publisher connections from the native UDS/two-pass broker GET read.
// It is not a business receipt, historical-message proof, full broker scope,
// writer fence or permission to accept/purge this batch's materials.
type lifecycleCurrentMQConnections struct {
	scope      string
	diagnostic stop.LoadedMQDiagnostic
}

func summarizeLifecycleCurrentMQConnections(d stop.LoadedMQDiagnostic, readErr error) (*lifecycleCurrentMQConnections, error) {
	if readErr != reader.ErrScopeUnproven || !hashRE.MatchString(d.ObservationSHA256) || d.Workers < 1 || d.Workers > 32 || d.Nodes < 1 || d.Nodes > 2048 || d.Clients < 1 || d.Clients > 65536 || !d.ExternalAIUnproven || d.BrokerScopeComplete {
		return nil, lifecycleError("lifecycle_loaded_mq_observation_failed")
	}
	// Publisher history, shared failure-handoff publisher attribution and
	// external AI remain explicitly unproven. Native reader failures or unknown
	// gaps do not reach this caller with a valid diagnostic/ScopeUnproven pair.
	return &lifecycleCurrentMQConnections{scope: "original_worker_registered_consumers_and_visible_publishers", diagnostic: d}, nil
}

func (h *lifecycleFixedHost) BindAcceptancePlan(ctx context.Context, r lifecycleRequest, a *backup.Archive, p *backup.TargetRecoveryPlan) error {
	if h == nil || h.owner == nil || h.services == nil || h.services.window == nil || h.dataBaseline == nil || h.acceptancePlan != nil || !h.services.identity.matches(r) {
		return lifecycleError("lifecycle_actual_complete_data_baseline_missing")
	}
	if e := backup.VerifyHostAcceptancePlan(ctx, p, a, backup.TargetRecoveryBorrowed{SQL: h.owner.originalConn, Mongo: h.owner.originalDB}, r.Recovery, h.services.window); e != nil {
		return e
	}
	h.acceptancePlan = p
	return nil
}

// Every partial proof below comes from original native owners. This adapter
// deliberately cannot finish acceptance until the complete same-batch material
// producers are present. Audit/current-ledger and original Worker connection
// observations retain their precise scope rather than proving all broker
// history or business receipts. Controlled A/D
// resume is wired only after consuming the retained pre-B comparison. The
// complete data comparison has already finished before the native B API start (its first controlled internal resume). Acceptance consumes that
// same-process fact; it does not demand byte equality after normal writers run.
// It creates no user/event/command or audit checkpoint database write.
func (h *lifecycleFixedHost) verifyNativeAcceptance(ctx context.Context, r lifecycleRequest, a *backup.Archive) error {
	if h != nil {
		h.currentMQ = nil // A failed fresh read cannot retain old success.
	}
	if h == nil || ctx == nil || ctx.Err() != nil || h.owner == nil || h.services == nil || h.services.window == nil || !h.services.identity.matches(r) || h.dataBaseline == nil || h.acceptancePlan == nil || h.acceptancePair == nil || h.api == nil {
		return lifecycleError("lifecycle_actual_runtime_and_data_acceptance_missing")
	}
	q, cancel, e := h.services.window.ForwardContext(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	if e = h.CheckWholeWriterFence(q, r); e != nil {
		return e
	}
	if e = h.services.CheckStoppedDependents(q, h.api.bCID); e != nil {
		return e
	}
	if e = backup.VerifyHostAcceptancePlan(q, h.acceptancePlan, a, backup.TargetRecoveryBorrowed{SQL: h.owner.originalConn, Mongo: h.owner.originalDB}, r.Recovery, h.services.window); e != nil {
		return e
	}
	if _, e = backup.VerifyDroppedTargets(q, h.acceptancePlan); e != nil {
		return e
	}
	if e = h.acceptancePair.VerifyAfter(q, h.owner.originalConn, h.owner.originalDB); e != nil || !lifecycleInlineMigrationBindingMatches(r, h.acceptancePair.Observation(), h.services.window, q) {
		return lifecycleError("lifecycle_actual_pair_acceptance_missing")
	}
	if e = h.verifyPreBDataComparison(q, r); e != nil {
		return e
	}
	if e = h.api.observeAcceptance(q, r); e != nil {
		return e
	}
	if e = h.services.CheckStoppedDependents(q, h.api.bCID); e != nil {
		return e
	}
	if e = h.CheckWholeWriterFence(q, r); e != nil {
		return e
	}
	if e = h.observeRuntimeLedgers(q, r); e != nil {
		return e
	}
	if e = h.services.CheckStoppedDependents(q, h.api.bCID); e != nil {
		return e
	}
	if e = h.CheckWholeWriterFence(q, r); e != nil {
		return e
	}
	observed, e := h.observeControlledAfterDataComparison(q, r)
	if e != nil {
		return e
	}
	if e = observed.validate(h); e != nil {
		return e
	}
	// The original D channel performs a genuine loaded UDS + two-pass broker GET
	// observation. Consume only its registered current-connection scope while
	// retaining every broader gap, the ScopeUnproven contract and full=false.
	if e = h.services.child.requireLive(); e != nil {
		return e
	}
	connections, e := summarizeLifecycleCurrentMQConnections(h.services.remote.ObserveLoadedMQ(q))
	if e != nil {
		return e
	}
	if e = h.CheckWholeWriterFence(q, r); e != nil {
		return e
	}
	h.currentMQ = connections
	if e = h.composeNativeMaterialOwners(q, r); e != nil {
		return e
	}
	// The actual accepted-material catalog producer is still missing. Neither
	// a local connection observation nor its known broader gaps can mint it.
	return lifecycleError("lifecycle_actual_complete_material_scope_missing")
}

func (h *lifecycleFixedHost) verifyCompleteDataBeforeInternalResume(ctx context.Context, proof *migration.CompatibilityPairMigrationProof) (result error) {
	if h == nil || h.owner == nil || h.owner.originalConn == nil || h.owner.originalMongo == nil || h.owner.originalDB == nil {
		return lifecycleError("lifecycle_actual_complete_data_baseline_missing")
	}
	tx, e := h.owner.originalConn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return lifecycleError("lifecycle_acceptance_snapshot_unproven")
	}
	defer func() {
		if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) && result == nil {
			result = lifecycleError("lifecycle_acceptance_snapshot_cleanup_failed")
		}
	}()
	g, e := gorm.Open(gormmysql.New(gormmysql.Config{Conn: tx, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		return lifecycleError("lifecycle_acceptance_snapshot_unproven")
	}
	session, e := h.owner.originalMongo.StartSession()
	if e != nil {
		return lifecycleError("lifecycle_acceptance_snapshot_unproven")
	}
	defer session.EndSession(context.Background())
	if e = session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())); e != nil {
		return lifecycleError("lifecycle_acceptance_snapshot_unproven")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Pinned driver's AbortTransaction discards its actual command error.
		// First require the server's response from the original native session;
		// ordinary Abort then maintains driver state regardless of that result.
		nativeError := lifecycleAbortComparisonMongo(cleanup, h.owner.originalMongo, session)
		localError := session.AbortTransaction(cleanup)
		if (nativeError != nil || localError != nil) && result == nil {
			result = lifecycleError("lifecycle_acceptance_snapshot_cleanup_failed")
		}
	}()
	paired := mongo.NewSessionContext(hostmysql.WithTx(ctx, g), session)
	return backup.VerifyCompleteNonTargetData(paired, h.dataBaseline, h.acceptancePlan, proof, backup.BorrowedSources{SQL: tx, Mongo: h.owner.originalDB})
}
