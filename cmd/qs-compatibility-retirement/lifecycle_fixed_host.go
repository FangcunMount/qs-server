package main

import (
	"context"
	"errors"
	"path/filepath"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
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
	owner                                *lifecyclePreparationOwner
	restoreOwner                         *lifecyclePreparationOwner
	materials                            *lifecycleBatchMaterials
	acceptedMaterials                    *lifecycleAcceptedMaterials
	inventoryMaterials                   *lifecycleMaterialDirectory
	rootStagingMaterials                 *lifecycleMaterialDirectory
	preparationInvocationMaterials       *lifecycleMaterialDirectory
	historicalWriteMaterials             *lifecycleMaterialDirectory
	historicalWriteRegistrationMaterials *lifecycleMaterialDirectory
	historicalWritePreviousMaterials     []*lifecycleMaterialDirectory
	archiveMaterials                     *lifecycleMaterialDirectory
	services                             *lifecycleServiceController
	api                                  *lifecycleAPITransition
	dataBaseline                         *backup.NonTargetDataBaseline
	acceptancePlan                       *backup.TargetRecoveryPlan
	acceptancePair                       *migration.CompatibilityPairMigrationProof
	preBComparison                       *lifecyclePreBDataComparison
	comparisonAttempted                  bool
	writers                              *lifecycleWriterObservation
	dbWriters                            *lifecycleDBWriterLease
	runtimeLedgers                       *lifecycleRuntimeLedgerObservation
	currentMQ                            *lifecycleCurrentMQConnections
	aiStopped                            *retirement.AIStoppedRuntimeLease
	finalRuntime                         *lifecycleControlledRuntime
}

func newLifecycleFixedHost(ctx context.Context, r lifecycleRequest, a *backup.Archive) (lifecycleHost, error) {
	if ctx == nil || ctx.Err() != nil || a == nil || r.ServiceControl == nil {
		return nil, lifecycleError("lifecycle_actual_host_adapters_missing")
	}
	if err := backup.VerifyHostArchiveBinding(ctx, a, r.Approval); err != nil {
		return nil, err
	}
	h := &lifecycleFixedHost{}
	// Preparation keeps its existing behavior. The final invocation reopens the
	// exact original producer files under the authenticated source UID and holds
	// their real read-only FDs; no persisted completion field issues a capability.
	if r.prepareRoot == lifecycleInvocationBatch(r.OperationID, r.ActualRunID) {
		if err := validateLifecycleAPIInvocation(r); err != nil {
			return nil, err
		}
		raw, err := readLifecycleAPIRecord(filepath.Join(r.prepareRoot, "native-call.intent.private.json"))
		if err != nil {
			return nil, err
		}
		i, err := decodeLifecycleAPIInvocationIntent(raw)
		if err != nil {
			return nil, err
		}
		h.inventoryMaterials, err = openLifecycleOriginalInventoryMaterials(ctx, r, a, i.SourceUID)
		if err != nil {
			return nil, err
		}
		h.rootStagingMaterials, h.preparationInvocationMaterials, err = openLifecycleOriginalRootStaging(ctx, r, h.inventoryMaterials, i.SourceUID)
		if err != nil {
			return nil, errors.Join(err, h.Close())
		}
		h.historicalWriteMaterials, err = openLifecycleOriginalHistoricalWriteMaterials(ctx, r, i.SourceUID)
		if err != nil {
			return nil, errors.Join(err, h.Close())
		}
		h.historicalWriteRegistrationMaterials, h.historicalWritePreviousMaterials, err = openLifecycleOriginalHistoricalWriteInputs(ctx, r, h.historicalWriteMaterials, h.inventoryMaterials, i.SourceUID)
		if err != nil {
			return nil, errors.Join(err, h.Close())
		}
	}
	var archiveErr error
	h.archiveMaterials, archiveErr = openLifecycleOriginalArchiveMaterials(ctx, r, a)
	if archiveErr != nil {
		return nil, errors.Join(archiveErr, h.Close())
	}
	return h, nil
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
	if !r.WriterControl.valid() || r.WriterControl.DatabaseInput == nil {
		return lifecycleError("lifecycle_database_writer_input_missing")
	}
	v := h.services
	var err error
	if h.aiStopped != nil || r.FinalHistory == nil || r.FinalHistory.StopConstraints == nil {
		return lifecycleError("lifecycle_ai_actual_stop_constraints_missing")
	}
	external, err := lifecycleFinalExternalInput(r)
	if err != nil || external == nil {
		return lifecycleError("lifecycle_ai_actual_stop_constraints_missing")
	}
	lease, err := retirement.OpenAIStoppedRuntimeLease(ctx, *external, *r.FinalHistory.StopConstraints, w)
	if err != nil {
		return err
	}
	h.aiStopped = lease // Retain original restoration owner before any native Stop.
	if err = v.StopAndDrain(ctx); err != nil {
		return err
	}
	if err = v.Check(ctx); err != nil {
		return err
	}
	if err = h.aiStopped.Stop(ctx); err != nil {
		return err
	}
	// Observe the actual two databases as a separate drain check. This neither
	// elevates the service lease to whole-writer isolation nor claims final Q.
	if h.owner == nil {
		return lifecycleError("lifecycle_native_handles_missing")
	}
	if err = h.installDatabaseWriterLease(ctx, r); err != nil {
		return err
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
	var result error
	if err = h.restoreDatabaseWriterLease(ctx, r, false); err != nil {
		return err
	}
	if h.services != nil {
		result = h.services.RestorePartialStop(ctx, r, w)
	}
	return errors.Join(result, h.restoreAI(ctx))
}
func (h *lifecycleFixedHost) restoreAI(ctx context.Context) error {
	if h.aiStopped == nil {
		return nil
	}
	if e := h.aiStopped.CleanupCarrierForRecovery(ctx); e != nil {
		return e
	}
	return h.aiStopped.Restore(ctx)
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
	// The four old objects are absent and the complete frozen comparison is
	// retained. Restore only our original credential states before B can start
	// its current readers/writers; the outer host/workflow fence remains required.
	h.acceptancePair = p
	if e = h.restoreDatabaseWriterLease(q, r, true); e != nil {
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
	if e = h.restoreDatabaseWriterLease(q, r, false); e != nil {
		return e
	}
	return h.api.deploy(q, r, true)
}
func (h *lifecycleFixedHost) PurgeTemporaryCopies(ctx context.Context, r lifecycleRequest) error {
	materials, err := h.acceptedBatchMaterials(ctx, r)
	if err != nil {
		return err
	}
	if err = h.registerAIStoppedMaterials(ctx, r, materials); err != nil {
		return err
	}
	if err = h.registerLocalServiceMaterials(ctx, r, materials); err != nil {
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

func (h *lifecycleFixedHost) ResumeAcceptedEntrypoints(ctx context.Context, r lifecycleRequest) error {
	materials, err := h.acceptedBatchMaterials(ctx, r)
	if err != nil {
		return err
	}
	if !materials.purged || !materials.zeroVerified || h.finalRuntime.validate(h) != nil || h.api == nil || h.api.acceptance == nil {
		return lifecycleError("lifecycle_actual_accepted_entrypoint_completion_missing")
	}
	o := h.api.acceptance
	if o.self != o || o.owner != h.api || o.cid != h.api.bCID || o.source != r.ToolSourceSHA {
		return lifecycleError("lifecycle_actual_accepted_entrypoint_completion_missing")
	}
	if h.aiStopped == nil {
		return lifecycleError("lifecycle_ai_original_stopped_runtime_unproven")
	}
	if err := h.aiStopped.VerifyResumed(ctx); err != nil {
		return err
	}
	// Collection/Worker and B already passed a final live read before D purge.
	// This acknowledges the same native acceptance/zero/terminal/Window facts;
	// it does not read deleted journals, restart services or claim a new runtime
	// observation. The original runner restores ordinary workflow entrypoints
	// only after this known terminal completion, using its retained quarantine.
	return ctx.Err()
}
func (h *lifecycleFixedHost) RestoreRollbackEntrypoints(ctx context.Context, r lifecycleRequest, w *fence.MaintenanceWindow) error {
	if h == nil {
		return lifecycleError("lifecycle_actual_service_lease_missing")
	}
	if err := h.restoreDatabaseWriterLease(ctx, r, false); err != nil {
		return err
	}
	// Recovery follows actual DDL reconciliation and the separately verified
	// no-migration rollback. Preserve any native partial stop lease, issuer and
	// original Window and the same existing remote transport.
	if h.services != nil {
		if err := h.services.UseOriginalRecoverySession(ctx, r, w); err != nil {
			return errors.Join(err, h.restoreAI(ctx))
		}
		return errors.Join(h.services.RestoreDependents(ctx), h.restoreAI(ctx))
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
	if h.inventoryMaterials != nil {
		result = h.inventoryMaterials.close()
	}
	if h.rootStagingMaterials != nil {
		result = errors.Join(result, h.rootStagingMaterials.close())
	}
	if h.preparationInvocationMaterials != nil {
		result = errors.Join(result, h.preparationInvocationMaterials.close())
	}
	if h.historicalWriteMaterials != nil {
		result = errors.Join(result, h.historicalWriteMaterials.close())
	}
	if h.archiveMaterials != nil {
		result = errors.Join(result, h.archiveMaterials.close())
	}
	if h.dbWriters != nil && (h.dbWriters.self != h.dbWriters || h.dbWriters.host != h || !h.dbWriters.restored) {
		result = lifecycleError("lifecycle_database_original_account_restore_unproven")
	}
	if h.historicalWriteRegistrationMaterials != nil {
		result = errors.Join(result, h.historicalWriteRegistrationMaterials.close())
	}
	for _, directory := range h.historicalWritePreviousMaterials {
		result = errors.Join(result, directory.close())
	}
	if h.writers != nil {
		h.writers.close()
	}
	if h.services != nil {
		result = errors.Join(result, h.services.Close())
	}
	if h.aiStopped != nil {
		result = errors.Join(result, h.aiStopped.Close())
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
	if h.materials != nil {
		if err := h.materials.close(); result == nil {
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
