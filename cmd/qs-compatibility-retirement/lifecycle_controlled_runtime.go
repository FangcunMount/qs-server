package main

import (
	"context"
	"errors"

	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
)

// Same-process actual original owners only. This partial runtime observation
// cannot mint whole acceptance, a writer fence or a material purge capability.
type lifecycleControlledRuntime struct {
	self     *lifecycleControlledRuntime
	services *lifecycleServiceController
	api      *lifecycleAPITransition
	local    *stop.DependentRuntimeObservation
	remote   *stop.RemoteRuntimeObservation
}

func (v *lifecycleServiceController) controlledResume(ctx context.Context) error {
	if v == nil || ctx == nil || ctx.Err() != nil || v.window == nil || v.local == nil || v.remote == nil || v.child == nil || !v.managementReady || !v.remoteStopAttempted || v.controlledAttempted {
		return lifecycleError("lifecycle_controlled_original_lease_missing")
	}
	if e := v.child.requireLive(); e != nil {
		return e
	}
	v.controlledAttempted = true // A partial result is never replayed or renamed success.
	if e := v.local.ControlledResumeDependents(ctx); e != nil {
		return e
	}
	_, e := v.remote.Do(ctx, "controlled_resume")
	return e
}
func (v *lifecycleServiceController) observeControlled(ctx context.Context, api *lifecycleAPITransition) (*lifecycleControlledRuntime, error) {
	if v == nil || api == nil || v.local == nil || v.remote == nil || v.child == nil || !v.controlledAttempted || !hashRE.MatchString(api.bCID) {
		return nil, lifecycleError("lifecycle_controlled_original_runtime_missing")
	}
	if e := v.child.requireLive(); e != nil {
		return nil, e
	}
	local, e := v.local.ObserveRunningDependentsWithInlineAPI(ctx, api.bCID)
	if e != nil {
		return nil, e
	}
	remote, e := v.remote.ObserveRunningDependents(ctx)
	if e != nil {
		return nil, e
	}
	o := &lifecycleControlledRuntime{services: v, api: api, local: local, remote: remote}
	o.self = o
	return o, nil
}

// This leaf wires genuine controlled resume and live readback after the complete
// approved frozen baseline comparison, preserving all original handles. The
// host's still-required whole fence is rechecked on both sides. It is not called
// from a JSON field, service health callback or ordinary deployment workflow.
func (h *lifecycleFixedHost) observeControlledAfterDataComparison(ctx context.Context, r lifecycleRequest) (*lifecycleControlledRuntime, error) {
	if h == nil || h.services == nil || h.api == nil || h.owner == nil || h.dataBaseline == nil || h.acceptancePair == nil {
		return nil, lifecycleError("lifecycle_actual_runtime_and_data_acceptance_missing")
	}
	// B already started only after the complete stopped comparison and actual
	// RO cleanup. Consume that original host/Window/plan fact again here; never
	// compare live rows after its normal background writers have resumed.
	if e := h.verifyPreBDataComparison(ctx, r); e != nil {
		return nil, e
	}
	if e := h.CheckWholeWriterFence(ctx, r); e != nil {
		return nil, e
	}
	if e := h.services.controlledResume(ctx); e != nil {
		return nil, e
	}
	o, e := h.services.observeControlled(ctx, h.api)
	if e != nil {
		return nil, e
	}
	if e = h.api.observeAcceptance(ctx, r); e != nil {
		return nil, e
	}
	if e = h.CheckWholeWriterFence(ctx, r); e != nil {
		return nil, e
	}
	return o, nil
}

// This concrete cleanup producer combines the same native D zero receipt with
// the actual terminal result of its original pinned SSH child. Neither alone
// proves zero. It cannot be called before this host's genuine whole acceptance
// and complete eight-scope material registration has been issued.
func (h *lifecycleFixedHost) purgeAcceptedRemoteMaterials(ctx context.Context, r lifecycleRequest, o *lifecycleControlledRuntime) (*lifecycleRemoteMaterialZero, error) {
	if h == nil || h.acceptedMaterials == nil || h.acceptedMaterials.self != h.acceptedMaterials || h.acceptedMaterials.host != h || h.acceptedMaterials.binding != lifecycleMaterialsBinding(r) || o == nil || o.self != o || o.services != h.services || o.api != h.api || o.remote == nil || h.services.remote == nil || h.services.child == nil {
		return nil, lifecycleError("lifecycle_actual_batch_acceptance_missing")
	}
	if e := h.CheckWholeWriterFence(ctx, r); e != nil {
		return nil, e
	}
	z, e := h.services.remote.PurgeOwnedMaterials(ctx, o.remote)
	if e != nil {
		return nil, e
	}
	// Only an actual successful terminal D reply may enter this branch. Closing
	// stdio/SSH without that proof never constructs a remote zero observation.
	terminal, e := h.closePurgedOriginalD(ctx, r, z)
	if e != nil {
		return nil, e
	}
	snapshot, e := z.Snapshot()
	if e != nil || snapshot.RemainingTemporaryFiles != 0 {
		return nil, lifecycleError("lifecycle_material_zero_unproven")
	}
	if e = h.observeWholeWriterScopesAfterDTerminal(ctx, r, terminal); e != nil {
		return nil, e
	}
	remote := &lifecycleRemoteMaterialZero{binding: lifecycleMaterialsBinding(r), native: z, services: h.services, terminal: terminal}
	remote.self = remote
	return remote, nil
}

func (o *lifecycleControlledRuntime) validate(h *lifecycleFixedHost) error {
	if o == nil || o.self != o || h == nil || o.services != h.services || o.api != h.api || o.local == nil || o.remote == nil {
		return lifecycleError("lifecycle_controlled_original_runtime_missing")
	}
	_, a := o.local.Snapshot()
	_, b := o.remote.Snapshot()
	return errors.Join(a, b)
}
