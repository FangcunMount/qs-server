package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
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
// deliberately cannot finish acceptance until actual audit/MQ/broker runtime
// and the complete same-batch material producers are present. Controlled A/D
// resume is wired only after consuming the retained pre-B comparison. The
// complete data comparison has already finished before the native B API start (its first controlled internal resume). Acceptance consumes that
// same-process fact; it does not demand byte equality after normal writers run.
// It creates no user/event/command or audit checkpoint database write.
func (h *lifecycleFixedHost) verifyNativeAcceptance(ctx context.Context, r lifecycleRequest, a *backup.Archive) error {
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
	// The original D channel now performs a genuine loaded UDS + broker GET
	// observation. It remains incomplete and cannot mint acceptance/materials.
	if e = h.services.child.requireLive(); e != nil {
		return e
	}
	if _, e = h.services.remote.ObserveLoadedMQ(q); errors.Is(e, reader.ErrScopeUnproven) {
		return lifecycleError("lifecycle_loaded_mq_scope_unproven")
	} else if e != nil {
		return lifecycleError("lifecycle_loaded_mq_observation_failed")
	}
	return lifecycleError("lifecycle_loaded_mq_complete_acceptance_unproven")
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
