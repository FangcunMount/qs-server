package main

import (
	"context"
	"errors"
	"os"
	"syscall"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
)

// This original-process fact replaces only D control-channel liveness after
// its native owned purge. It proves neither acceptance nor any other writer
// scope. Only closePurgedOriginalD produces one; requests/JSON cannot choose it.
type lifecycleDTerminal struct {
	self       *lifecycleDTerminal
	host       *lifecycleFixedHost
	services   *lifecycleServiceController
	child      *lifecycleOwnedServiceSSH
	controller *stop.RemoteController
	issuer     *stop.BudgetIssuer
	window     *fence.MaintenanceWindow
	accepted   *lifecycleAcceptedMaterials
	native     *stop.RemoteMaterialZero
	binding    lifecycleMaterialBinding
	start      string
	scope      string
	pid        int
}

func (*lifecycleDTerminal) MarshalJSON() ([]byte, error) {
	return nil, lifecycleError("lifecycle_material_zero_unproven")
}
func (*lifecycleDTerminal) UnmarshalJSON([]byte) error {
	return lifecycleError("lifecycle_material_zero_unproven")
}

// Close's actual Wait and original-group absence are necessary. Saved exit=0,
// a closed done channel or reaped=true alone cannot satisfy this observation.
func lifecycleOriginalDChildTerminal(v *lifecycleOwnedServiceSSH, pid int) error {
	if v == nil || v.cmd == nil || v.cmd.Process == nil || v.done == nil || v.in == nil || v.out == nil || pid <= 0 || v.cmd.Process.Pid != pid {
		return lifecycleError("lifecycle_original_live_recovery_session_unknown")
	}
	select {
	case <-v.done:
	default:
		return lifecycleError("lifecycle_original_live_recovery_session_unknown")
	}
	// done synchronizes the original cmd.Wait's ProcessState and waitErr writes.
	if v.cmd.ProcessState == nil || v.cmd.ProcessState.Pid() != pid || !v.cmd.ProcessState.Exited() || !v.cmd.ProcessState.Success() {
		return lifecycleError("lifecycle_original_live_recovery_session_unknown")
	}
	if !v.reaped || v.waitErr != nil || v.closeErr != nil || !errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH) {
		return lifecycleError("lifecycle_original_live_recovery_session_unknown")
	}
	if _, e := v.in.Stat(); !errors.Is(e, os.ErrClosed) {
		return lifecycleError("lifecycle_original_live_recovery_session_unknown")
	}
	if _, e := v.out.Stat(); !errors.Is(e, os.ErrClosed) {
		return lifecycleError("lifecycle_original_live_recovery_session_unknown")
	}
	return nil
}

func (h *lifecycleFixedHost) closePurgedOriginalD(ctx context.Context, r lifecycleRequest, native *stop.RemoteMaterialZero) (*lifecycleDTerminal, error) {
	if h == nil || ctx == nil || ctx.Err() != nil || h.services == nil || h.services.child == nil || h.services.remote == nil || h.services.issuer == nil || h.services.window == nil ||
		!h.services.identity.matches(r) || h.acceptedMaterials == nil || h.acceptedMaterials.self != h.acceptedMaterials || h.acceptedMaterials.host != h || h.acceptedMaterials.binding != lifecycleMaterialsBinding(r) || native == nil {
		return nil, lifecycleError("lifecycle_material_zero_unproven")
	}
	if e := native.ValidateOriginalController(ctx, h.services.remote); e != nil {
		return nil, e
	}
	snapshot, e := native.Snapshot()
	if e != nil || snapshot.RemainingTemporaryFiles != 0 {
		return nil, lifecycleError("lifecycle_material_zero_unproven")
	}
	d, e := h.services.window.Diagnostic(ctx)
	if e != nil || d.Binding != lifecycleWindowBinding(r) || !hashRE.MatchString(d.StartSHA256) || !d.DirectoryLeaseHeld || d.RecoverySHA256 != "" {
		return nil, lifecycleError("lifecycle_material_zero_unproven")
	}
	child := h.services.child
	if child.cmd == nil || child.cmd.Process == nil {
		return nil, lifecycleError("lifecycle_material_zero_unproven")
	}
	pid := child.cmd.Process.Pid
	if e = child.Close(); e != nil {
		return nil, e
	}
	t := &lifecycleDTerminal{host: h, services: h.services, child: child, controller: h.services.remote, issuer: h.services.issuer, window: h.services.window,
		accepted: h.acceptedMaterials, native: native, binding: lifecycleMaterialsBinding(r), start: d.StartSHA256, scope: digest(snapshot), pid: pid}
	t.self = t
	if e = t.validate(ctx, h, r); e != nil {
		return nil, e // Unknown/expired terminal work is retained, never replayed.
	}
	return t, nil
}

func (t *lifecycleDTerminal) validate(ctx context.Context, h *lifecycleFixedHost, r lifecycleRequest) error {
	if t == nil || t.self != t || h == nil || t.host != h || ctx == nil || ctx.Err() != nil || t.services == nil || t.services != h.services ||
		t.child == nil || t.child != h.services.child || t.controller == nil || t.controller != h.services.remote || t.issuer == nil || t.issuer != h.services.issuer ||
		t.window == nil || t.window != h.services.window || t.accepted == nil || t.accepted != h.acceptedMaterials || t.accepted.self != t.accepted || t.accepted.host != h ||
		t.binding != lifecycleMaterialsBinding(r) || t.accepted.binding != t.binding || !t.binding.valid() || !h.services.identity.matches(r) ||
		!hashRE.MatchString(t.start) || !hashRE.MatchString(t.scope) || t.native == nil {
		return lifecycleError("lifecycle_material_zero_unproven")
	}
	q, cancel, e := t.window.ForwardContext(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	d, e := t.window.Diagnostic(q)
	if e != nil || d.Binding != lifecycleWindowBinding(r) || d.StartSHA256 != t.start || !d.DirectoryLeaseHeld || d.RemainingMilliseconds <= 0 || d.RecoverySHA256 != "" {
		return lifecycleError("lifecycle_material_zero_unproven")
	}
	if e = h.verifyPreBDataComparison(q, r); e != nil {
		return e
	}
	if e = t.native.ValidateOriginalController(q, t.controller); e != nil {
		return e
	}
	snapshot, e := t.native.Snapshot()
	if e != nil || snapshot.RemainingTemporaryFiles != 0 || digest(snapshot) != t.scope {
		return lifecycleError("lifecycle_material_zero_unproven")
	}
	if e = lifecycleOriginalDChildTerminal(t.child, t.pid); e != nil {
		return e
	}
	return q.Err()
}
