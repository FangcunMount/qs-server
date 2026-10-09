package main

import (
	"context"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
)

// Remaining production ports have no actual producer yet. This source gate is
// deliberately before request/DB/window/service writes; no environment flag,
// imported Permit or callback/receipt can flip it. The five-stage kernel remains
// compiled below, but a service-only lease cannot activate it.
func lifecycleEffectsPreflight(context.Context) error {
	return lifecycleError("lifecycle_actual_host_adapters_missing")
}

type lifecycleFixedHost struct {
	owner               *lifecyclePreparationOwner
	restoreOwner        *lifecyclePreparationOwner
	acceptedMaterials   *lifecycleAcceptedMaterials
	services            *lifecycleServiceController
	api                 *lifecycleAPITransition
	dataBaseline        *backup.NonTargetDataBaseline
	acceptancePlan      *backup.TargetRecoveryPlan
	acceptancePair      *migration.CompatibilityPairMigrationProof
	preBComparison      *lifecyclePreBDataComparison
	comparisonAttempted bool
	writers             *lifecycleWriterObservation
}

func newLifecycleFixedHost(ctx context.Context, r lifecycleRequest, a *backup.Archive) (lifecycleHost, error) {
	if ctx == nil || ctx.Err() != nil || a == nil || r.ServiceControl == nil {
		return nil, lifecycleError("lifecycle_actual_host_adapters_missing")
	}
	if err := backup.VerifyHostArchiveBinding(ctx, a, r.Approval); err != nil {
		return nil, err
	}
	return &lifecycleFixedHost{}, nil
}

func (h *lifecycleFixedHost) OpenRecoveryHandles(ctx context.Context, _ lifecycleRequest, _ *backup.Archive) (backup.TargetRecoveryBorrowed, error) {
	if h == nil || h.owner != nil || ctx == nil || ctx.Err() != nil {
		return backup.TargetRecoveryBorrowed{}, lifecycleError("lifecycle_native_handles_missing")
	}
	o := &lifecyclePreparationOwner{}
	h.owner = o // Failure cleanup belongs to this host, including partial opens.
	var err error
	o.originalSQL, err = lifecycleSQLPool(ctx, "")
	if err != nil {
		return backup.TargetRecoveryBorrowed{}, err
	}
	o.originalMongo, o.originalDB, err = lifecycleMongoClient(ctx, "")
	if err != nil {
		return backup.TargetRecoveryBorrowed{}, err
	}
	o.originalConn, err = o.originalSQL.Conn(ctx)
	if err != nil {
		return backup.TargetRecoveryBorrowed{}, lifecycleError("lifecycle_native_handles_missing")
	}
	return backup.TargetRecoveryBorrowed{SQL: o.originalConn, Mongo: o.originalDB}, nil
}

func (h *lifecycleFixedHost) Prepare(ctx context.Context, r lifecycleRequest, a *backup.Archive) (*lifecyclePreparation, error) {
	if h == nil || h.restoreOwner != nil || h.owner != nil {
		return nil, lifecycleError("lifecycle_native_handles_existing_or_unknown")
	}
	p, owner, err := prepareLifecycleNative(ctx, r, a)
	if owner != nil {
		h.restoreOwner = owner // Retain the actual restore owners; never adopt their JSON registry.
	}
	if err != nil {
		return nil, err
	}
	if !lifecyclePreparationMatches(a, p) {
		_ = owner.Close()
		return nil, lifecycleError("lifecycle_actual_restore_proof_missing_or_budget_rejected")
	}
	// Complete the actual restore, close/join/reap and final inspection under
	// the original600 budget before obtaining new autocommit DDL handles. A
	// restored wire connection never survives into the maintenance window.
	elapsed, err := owner.finishPreparation()
	if err != nil {
		return nil, err
	}
	p.combinedElapsedMillis = elapsed
	p.Borrowed, err = h.OpenRecoveryHandles(ctx, r, a)
	if err != nil {
		return p, err
	}
	h.api, err = prepareLifecycleAPITransition(ctx, r)
	return p, err
}

func (h *lifecycleFixedHost) StopAndDrain(ctx context.Context, r lifecycleRequest, w *fence.MaintenanceWindow) error {
	if h == nil || h.services == nil || h.services.window != w || !h.services.identity.matches(r) || !h.services.managementReady {
		return lifecycleError("lifecycle_service_controller_binding_rejected")
	}
	v := h.services
	var err error
	if err = v.StopAndDrain(ctx); err != nil {
		return err
	}
	if err = v.Check(ctx); err != nil {
		return err
	}
	// Observe the actual two databases as a separate drain check. This neither
	// elevates the service lease to whole-writer isolation nor claims final Q.
	if h.owner == nil {
		return lifecycleError("lifecycle_native_handles_missing")
	}
	_, err = stop.ObserveDatabaseDrain(ctx, h.owner.originalConn, h.owner.originalDB)
	return err
}

func (h *lifecycleFixedHost) OpenServiceManagement(ctx context.Context, r lifecycleRequest, w *fence.MaintenanceWindow) error {
	if h == nil || h.services != nil {
		return lifecycleError("lifecycle_service_controller_binding_rejected")
	}
	v, err := openLifecycleServiceController(ctx, r, w, false)
	if err != nil {
		return err
	}
	h.services = v
	return nil
}

func (h *lifecycleFixedHost) RestoreStoppedServices(ctx context.Context, r lifecycleRequest, w *fence.MaintenanceWindow) error {
	if h == nil || ctx == nil || ctx.Err() != nil || w == nil {
		return lifecycleError("lifecycle_service_controller_binding_rejected")
	}
	d, err := w.Diagnostic(ctx)
	if err != nil || d.Binding != lifecycleWindowBinding(r) || d.RecoverySHA256 == "" {
		return lifecycleError("lifecycle_service_controller_binding_rejected")
	}
	if h.services == nil {
		// Controller creation failed before either native Stop was called.
		return nil
	}
	return h.services.RestorePartialStop(ctx, r, w)
}

func (h *lifecycleFixedHost) CheckWholeWriterFence(ctx context.Context, r lifecycleRequest) error {
	return h.observeWholeWriterScopes(ctx, r)
}
func (h *lifecycleFixedHost) FinalDifferenceAndEOF(ctx context.Context, r lifecycleRequest, a *backup.Archive) error {
	return h.finalDifferenceAndEOF(ctx, r, a)
}
func (h *lifecycleFixedHost) DeployBInline(ctx context.Context, r lifecycleRequest, p *migration.CompatibilityPairMigrationProof, w *fence.MaintenanceWindow) error {
	if h == nil || h.owner == nil || h.services == nil || h.api == nil || p == nil || w == nil || h.services.window != w || h.comparisonAttempted || h.preBComparison != nil || h.acceptancePair != nil {
		return lifecycleError("lifecycle_actual_inline_b_deployment_missing")
	}
	q, c, e := w.ForwardContext(ctx)
	if e != nil {
		return e
	}
	defer c()
	// A normal B startup immediately resumes its A-equivalent relays and
	// schedulers. Complete the frozen data comparison and end both RO scopes
	// before the first native API start, while all original services are stopped.
	h.preBComparison, e = h.compareCompleteDataBeforeB(q, r, p, w)
	if e != nil {
		return e
	}
	if e = h.api.deploy(q, r, false); e != nil {
		return e
	}
	h.acceptancePair = p // The actual same-run native producer, never its DTO.
	return nil
}
func (h *lifecycleFixedHost) VerifyAcceptance(ctx context.Context, r lifecycleRequest, a *backup.Archive) error {
	return h.verifyNativeAcceptance(ctx, r, a)
}
func (h *lifecycleFixedHost) CheckActualDDLStopped(ctx context.Context, r lifecycleRequest) error {
	if h == nil || h.owner == nil || h.services == nil || h.services.window == nil {
		return lifecycleError("lifecycle_actual_ddl_stopped_unproven")
	}
	q, c, e := h.services.window.RecoveryContext(ctx)
	if e != nil {
		return e
	}
	defer c()
	if h.api != nil {
		if e = h.api.stopBForRecovery(q, r); e != nil {
			return e
		}
	}
	_, e = stop.ObserveDatabaseDrain(q, h.owner.originalConn, h.owner.originalDB)
	return e // No observation manufactures a logged DROP result or replay authority.
}
func (h *lifecycleFixedHost) DeployRollbackInline(ctx context.Context, r lifecycleRequest, p *lifecycleRecoveryReadback, w *fence.MaintenanceWindow) error {
	if h == nil || h.owner == nil || h.services == nil || w == nil || h.services.window != w || p == nil {
		return lifecycleError("lifecycle_actual_no_migration_rollback_missing")
	}
	q, c, e := w.RecoveryContext(ctx)
	if e != nil {
		return e
	}
	defer c()
	if e = p.verify(q, backup.TargetRecoveryBorrowed{SQL: h.owner.originalConn, Mongo: h.owner.originalDB}, r, w); e != nil {
		return e
	}
	if h.api == nil {
		return lifecycleError("lifecycle_actual_no_migration_rollback_missing")
	}
	return h.api.deploy(q, r, true)
}
func (h *lifecycleFixedHost) PurgeTemporaryCopies(ctx context.Context, r lifecycleRequest) error {
	materials, err := h.acceptedBatchMaterials(ctx, r)
	if err != nil {
		return err
	}
	return materials.purge(ctx)
}
func (h *lifecycleFixedHost) VerifyTemporaryMaterialsZero(ctx context.Context, r lifecycleRequest) error {
	materials, err := h.acceptedBatchMaterials(ctx, r)
	if err != nil {
		return err
	}
	return materials.verifyZero(ctx)
}

func (h *lifecycleFixedHost) ResumeAcceptedEntrypoints(ctx context.Context, _ lifecycleRequest) error {
	if h == nil || h.services == nil {
		return lifecycleError("lifecycle_actual_service_lease_missing")
	}
	return h.services.ResumeDependents(ctx)
}
func (h *lifecycleFixedHost) RestoreRollbackEntrypoints(ctx context.Context, r lifecycleRequest, w *fence.MaintenanceWindow) error {
	if h == nil {
		return lifecycleError("lifecycle_actual_service_lease_missing")
	}
	// Recovery follows actual DDL reconciliation and the separately verified
	// no-migration rollback. Preserve any native partial stop lease, issuer and
	// original Window and the same existing remote transport.
	if h.services != nil {
		if err := h.services.UseOriginalRecoverySession(ctx, r, w); err != nil {
			return err
		}
		return h.services.RestoreDependents(ctx)
	}
	// A lost original channel cannot be replaced by a login after its key was
	// fenced. Cross-process recovery needs its actual pre-established management
	// channel and native runtime handoff; JSON or a new ordinary SSH is refused.
	return lifecycleError("lifecycle_prewindow_live_recovery_channel_missing")
}
func (h *lifecycleFixedHost) Close() error {
	if h == nil {
		return nil
	}
	var result error
	if h.writers != nil {
		h.writers.close()
	}
	if h.services != nil {
		result = h.services.Close()
	}
	if h.api != nil && h.api.engine != nil {
		h.api.engine.transport.CloseIdleConnections()
	}
	if h.api != nil && h.api.materials != nil {
		if err := h.api.materials.close(); result == nil {
			result = err
		}
	}
	if h.acceptedMaterials != nil && h.acceptedMaterials.catalog != nil {
		if err := h.acceptedMaterials.catalog.close(); result == nil {
			result = err
		}
	}
	if h.restoreOwner != nil {
		if err := h.restoreOwner.Close(); result == nil {
			result = err
		}
	}
	if h.owner != nil {
		if err := h.owner.Close(); result == nil {
			result = err
		}
	}
	return result
}

var _ lifecycleHost = (*lifecycleFixedHost)(nil)
