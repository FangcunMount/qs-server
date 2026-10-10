package main

import (
	"context"
	"errors"
	"path/filepath"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
)

type lifecycleServiceControl struct {
	LocalDescriptorSHA256 string `json:"local_descriptor_sha256"`
	SSHChannelSHA256      string `json:"ssh_channel_sha256"`
}

// This adapter borrows the caller's one original Window. It never opens a
// copied/new window, begins recovery, reconstructs an opaque Lease from JSON,
// or treats stopping QS containers as isolation of all writers and old refs.
// Captured after actual native approval. Expected identity alone never produces
// a lease or budget; it prevents a reconnect from changing the original binding.
type lifecycleServiceControllerIdentity struct {
	windowBinding          fence.WindowBinding
	toolSourceSHA          string
	actualRunID            string
	localDescriptorSHA256  string
	sshChannelSHA256       string
	remoteDescriptorSHA256 string
}

func (i lifecycleServiceControllerIdentity) matches(r lifecycleRequest) bool {
	return r.ServiceControl != nil && i.windowBinding == lifecycleWindowBinding(r) &&
		i.toolSourceSHA == r.ToolSourceSHA && i.toolSourceSHA == sourceSHA && i.actualRunID == r.ActualRunID &&
		i.localDescriptorSHA256 == r.ServiceControl.LocalDescriptorSHA256 &&
		i.sshChannelSHA256 == r.ServiceControl.SSHChannelSHA256
}

type lifecycleServiceController struct {
	window                   *fence.MaintenanceWindow
	approval                 *stop.Approval
	local                    *stop.Lease
	remote                   *stop.RemoteController
	issuer                   *stop.BudgetIssuer
	child                    *lifecycleOwnedServiceSSH
	journal                  string
	stopAttempted            bool
	remoteStopAttempted      bool
	partialRecoveryAttempted bool
	identity                 lifecycleServiceControllerIdentity
	recoveryAttempted        bool
	managementReady          bool
	controlledAttempted      bool
}

func openLifecycleServiceController(ctx context.Context, r lifecycleRequest, w *fence.MaintenanceWindow, recovery bool) (_ *lifecycleServiceController, result error) {
	if ctx == nil || ctx.Err() != nil || w == nil || r.ServiceControl == nil ||
		!hashRE.MatchString(r.ServiceControl.LocalDescriptorSHA256) || !hashRE.MatchString(r.ServiceControl.SSHChannelSHA256) {
		return nil, lifecycleError("lifecycle_service_controller_binding_rejected")
	}
	root := lifecycleServicesRoot(r.OperationID, "server-a")
	path := filepath.Join(root, "approved-services.json")
	a, err := stop.ReadApprovedDescriptor(path, r.ServiceControl.LocalDescriptorSHA256)
	if err != nil {
		return nil, lifecycleError("lifecycle_service_approval_rejected")
	}
	b, err := a.WindowBinding(ctx)
	if err != nil || b != lifecycleWindowBinding(r) {
		return nil, lifecycleError("lifecycle_service_approval_rejected")
	}
	key, err := stop.OpenRootBudgetKey(ctx, a)
	if err != nil {
		return nil, lifecycleError("lifecycle_actual_budget_issuer_missing")
	}
	issuer, err := stop.OpenBudgetIssuer(ctx, a, key, w)
	if err != nil {
		return nil, lifecycleError("lifecycle_actual_budget_issuer_missing")
	}
	v := &lifecycleServiceController{window: w, approval: a, issuer: issuer, journal: filepath.Join(root, "service-journal"),
		identity: lifecycleServiceControllerIdentity{windowBinding: b, toolSourceSHA: r.ToolSourceSHA, actualRunID: r.ActualRunID,
			localDescriptorSHA256: r.ServiceControl.LocalDescriptorSHA256, sshChannelSHA256: r.ServiceControl.SSHChannelSHA256}}
	defer func() {
		if result != nil {
			_ = v.Close()
		}
	}()
	var session context.Context
	var cancel context.CancelFunc
	if recovery {
		session, cancel, err = w.RecoveryContext(ctx)
	} else {
		session, cancel, err = w.ForwardContext(ctx)
	}
	if err != nil {
		return nil, err
	}
	defer cancel()
	child, channel, err := startLifecycleServiceSSH(session, r, filepath.Join(root, "ssh-channel.json"), r.ServiceControl.SSHChannelSHA256, recovery)
	if err != nil {
		return nil, err
	}
	v.child = child
	// Re-read the exact native approval only for this expected channel binding;
	// WindowBinding above already performed root/machine/tool/socket validation.
	var descriptor stop.Descriptor
	if readLifecyclePrivate(path, r.ServiceControl.LocalDescriptorSHA256, &descriptor) != nil || descriptor.HostRole != "server-a" || descriptor.RemoteDescriptorSHA256 != channel.RemoteDescriptorSHA256 {
		return nil, lifecycleError("lifecycle_service_approval_rejected")
	}
	v.identity.remoteDescriptorSHA256 = channel.RemoteDescriptorSHA256
	v.remote, err = stop.OpenLiveRemoteController(session, issuer, child.in, child.out)
	if err != nil {
		return nil, lifecycleError("lifecycle_live_remote_controller_rejected")
	}
	if recovery {
		v.local, err = stop.OpenRecovery(session, a, v.journal, w)
		if err != nil {
			return nil, err
		}
	} else {
		if _, err = v.remote.Do(session, "bind"); err != nil {
			return nil, lifecycleError("lifecycle_live_management_binding_failed")
		}
		v.managementReady = true // Actual native binding only, never writer isolation.
	}
	return v, nil
}

func (v *lifecycleServiceController) StopAndDrain(ctx context.Context) error {
	if v == nil || v.window == nil || v.remote == nil || !v.managementReady || v.stopAttempted || v.local != nil {
		return lifecycleError("lifecycle_service_controller_binding_rejected")
	}
	// An uncertain first action cannot be retried, even in the same process.
	v.stopAttempted = true
	return lifecycleRunServiceStops(func() error {
		l, err := stop.StopAndDrain(ctx, v.approval, v.journal, v.window)
		// Preserve the native partial lease; this does not itself restore services.
		v.local = l
		return err
	}, func() error {
		// This is unknown-action responsibility, never an issued remote Lease or
		// permission. Recovery must read D's actual original journal/baseline.
		v.remoteStopAttempted = true
		_, err := v.remote.Do(ctx, "stop")
		return err
	})
}

// Local failure never dispatches D. These callbacks are bound to the real
// opaque producers at the only caller above; no request supplies a callback.
func lifecycleRunServiceStops(local, remote func() error) error {
	if local == nil || remote == nil {
		return lifecycleError("lifecycle_service_controller_binding_rejected")
	}
	if err := local(); err != nil {
		return err
	}
	return remote()
}

// Each already-issued side must be attempted even when the other refuses an
// unknown stop. Joining errors preserves both obligations without calling any
// failed or pending action successful. Nil means that side was never issued.
func lifecycleRecoverIssuedServiceSides(local, remote func() error) error {
	var result error
	if local != nil {
		result = local()
	}
	if remote != nil {
		result = errors.Join(result, remote())
	}
	return result
}

func (v *lifecycleServiceController) RestorePartialStop(ctx context.Context, r lifecycleRequest, w *fence.MaintenanceWindow) error {
	if v == nil || ctx == nil || ctx.Err() != nil || w == nil || v.window != w || !v.stopAttempted ||
		v.partialRecoveryAttempted || v.issuer == nil || v.approval == nil || !v.identity.matches(r) {
		return lifecycleError("lifecycle_service_controller_binding_rejected")
	}
	// A real original-budget observation is required for this one attempt.
	// It never constructs a service Lease or grants Stop/DROP permission.
	d, err := w.Diagnostic(ctx)
	if err != nil || d.Binding != v.identity.windowBinding || d.RecoverySHA256 == "" {
		return lifecycleError("lifecycle_service_controller_binding_rejected")
	}
	v.partialRecoveryAttempted = true
	var local, remote func() error
	if v.local != nil {
		local = func() error { return v.local.Restore(ctx) }
	}
	if v.remoteStopAttempted {
		remote = func() error {
			if err := v.UseOriginalRecoverySession(ctx, r, w); err != nil {
				return err
			}
			_, err := v.remote.Do(ctx, "restore")
			return err
		}
	}
	result := lifecycleRecoverIssuedServiceSides(local, remote)
	// Reap the actual old or recovery transport before calling the attempt done.
	// If D received no Stop, do not reconnect or manufacture a D baseline.
	result = errors.Join(result, v.closeForwardTransport())
	if d, err := w.Diagnostic(ctx); err != nil || d.Binding != v.identity.windowBinding || d.RecoverySHA256 == "" {
		result = errors.Join(result, lifecycleError("lifecycle_partial_stop_original_budget_exhausted"))
	}
	if result != nil {
		if errors.Is(result, stop.ErrRollbackAPI) {
			return errors.Join(lifecycleError("lifecycle_partial_stop_bound_rollback_api_required"), result)
		}
		return errors.Join(lifecycleError("lifecycle_partial_stop_recovery_failed"), result)
	}
	return nil
}

func (v *lifecycleServiceController) Check(ctx context.Context) error {
	if v == nil || v.local == nil || v.remote == nil {
		return lifecycleError("lifecycle_actual_service_lease_missing")
	}
	if err := v.local.Check(ctx); err != nil {
		return err
	}
	_, err := v.remote.Do(ctx, "check")
	return err
}

// This post-deployment readback keeps the original A/D leases and the full
// relevant-container catalog. The API ID is supplied only by the original
// inline native API owner, never a request or imported receipt. It does not
// relax Check or any recovery path, resume services, or prove external fencing.
func (v *lifecycleServiceController) CheckStoppedDependents(ctx context.Context, nativeInlineAPIID string) error {
	if v == nil || v.local == nil || v.remote == nil {
		return lifecycleError("lifecycle_actual_service_lease_missing")
	}
	if err := v.local.CheckStoppedDependents(ctx, nativeInlineAPIID); err != nil {
		return err
	}
	_, err := v.remote.Do(ctx, "check")
	return err
}

func (v *lifecycleServiceController) RestoreDependents(ctx context.Context) error {
	if v == nil || v.local == nil || v.remote == nil {
		return lifecycleError("lifecycle_actual_service_lease_missing")
	}
	// This does not restart original API (its migrations are enabled). The
	// parent must first reconcile actual DDL/schema and deploy the approved A
	// rollback with the existing --migration.enabled=false flag.
	if _, err := v.remote.Do(ctx, "restore_dependents"); err != nil {
		return err
	}
	return v.local.RestoreDependents(ctx)
}

func (v *lifecycleServiceController) Close() error {
	if v == nil {
		return nil
	}
	var result error
	if v.remote != nil {
		result = v.remote.Close()
	}
	if v.child != nil {
		if err := v.child.Close(); result == nil {
			result = err
		}
	}
	if v.local != nil {
		if err := v.local.Close(); result == nil {
			result = err
		}
	}
	if v.issuer != nil {
		if err := v.issuer.Close(); result == nil {
			result = err
		}
	}
	return result
}

// closeForwardTransport reaps only the old physical SSH transport. It keeps the
// original native local Lease and BudgetIssuer alive for same-process recovery.
// Its nil result proves local transport cleanup only, never a remote action.
func (v *lifecycleServiceController) closeForwardTransport() error {
	if v == nil {
		return lifecycleError("lifecycle_service_controller_binding_rejected")
	}
	var result error
	if v.remote != nil {
		result = v.remote.Close()
	}
	if v.child != nil {
		if err := v.child.closeForRecovery(); result == nil {
			result = err
		}
	}
	if result == nil {
		v.remote, v.child = nil, nil
	}
	return result
}

// UseOriginalRecoverySession keeps the same actual SSH process, native pipes,
// issuer, Lease and original Window. It never opens a new SSH entrypoint after
// the fence closed that key. Unknown/lost transport remains blocked; a saved
// reply or pending action cannot reconstruct the original continuation.
func (v *lifecycleServiceController) UseOriginalRecoverySession(ctx context.Context, r lifecycleRequest, w *fence.MaintenanceWindow) error {
	if v == nil || ctx == nil || ctx.Err() != nil || w == nil || v.window != w || v.local == nil || v.issuer == nil ||
		v.approval == nil || v.recoveryAttempted || !v.managementReady || !v.identity.matches(r) || v.remote == nil || v.child == nil {
		return lifecycleError("lifecycle_service_controller_binding_rejected")
	}
	b, err := v.approval.WindowBinding(ctx)
	if err != nil || b != v.identity.windowBinding {
		return lifecycleError("lifecycle_service_approval_rejected")
	}
	session, cancel, err := w.RecoveryContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	d, err := w.Diagnostic(session)
	if err != nil || d.Binding != v.identity.windowBinding || d.RecoverySHA256 == "" {
		return lifecycleError("lifecycle_service_controller_binding_rejected")
	}
	if err = v.child.requireLive(); err != nil {
		return err
	}
	if err = v.remote.ValidateRecoveryContinuation(session); err != nil {
		return lifecycleError("lifecycle_original_live_recovery_session_unknown")
	}
	// One recovery continuation per actual controller. Its next Do action still
	// exchanges a fresh D challenge and original Window's real recovery signature.
	v.recoveryAttempted = true
	return nil
}
