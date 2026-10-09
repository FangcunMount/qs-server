package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

// This is expected input provenance, never a persisted isolation result. The
// native producer re-reads the complete current platform scope using fixed GETs.
type lifecycleWriterControl struct {
	WorkflowScopeSHA256 string `json:"workflow_scope_sha256"`
}

func (v *lifecycleWriterControl) valid() bool {
	return v != nil && hashRE.MatchString(v.WorkflowScopeSHA256)
}

type lifecycleWriterObservation struct {
	token    []byte
	platform *fence.PlatformObservation
}

func (v *lifecycleWriterObservation) close() {
	if v == nil {
		return
	}
	for i := range v.token {
		v.token[i] = 0
	}
	v.token = nil
	v.platform = nil
}

func lifecyclePlatformToken() ([]byte, error) {
	// The fixed root caller received this one short-lived token in its private
	// credential stdin packet. Remove the inherited environment copy immediately;
	// the host owns the remaining bytes and zeroes them on Close. No file holds it.
	raw := os.Getenv("GITHUB_READ_TOKEN")
	if e := os.Unsetenv("GITHUB_READ_TOKEN"); e != nil {
		return nil, lifecycleError("lifecycle_platform_observation_unproven")
	}
	if len(raw) < 1 || len(raw) > 8192 {
		return nil, lifecycleError("lifecycle_platform_observation_unproven")
	}
	for _, c := range []byte(raw) {
		if c < 33 || c > 126 {
			return nil, lifecycleError("lifecycle_platform_observation_unproven")
		}
	}
	return []byte(raw), nil
}

func (h *lifecycleFixedHost) observeWholeWriterScopes(ctx context.Context, r lifecycleRequest) error {
	if h == nil || ctx == nil || ctx.Err() != nil || !r.WriterControl.valid() || r.DeploymentControl == nil || h.services == nil || h.services.window == nil || !h.services.managementReady || !h.services.identity.matches(r) || h.services.child == nil {
		return lifecycleError("lifecycle_writer_scope_binding_rejected")
	}
	// This is the same pre-established D management process. Checking liveness
	// does not stop it, create another login, or claim that other sessions are gone.
	if e := h.services.child.requireLive(); e != nil {
		return e
	}
	if e := validateLifecycleAPIInvocation(r); e != nil {
		return e
	}
	intentPath := filepath.Join(r.prepareRoot, "native-call.intent.private.json")
	before, e := readLifecycleAPIRecord(intentPath)
	if e != nil {
		return e
	}
	intent, e := decodeLifecycleAPIInvocationIntent(before)
	if e != nil {
		return e
	}
	if h.writers == nil {
		token, e := lifecyclePlatformToken()
		if e != nil {
			return e
		}
		h.writers = &lifecycleWriterObservation{token: token}
	}
	scopePath := filepath.Join(lifecycleServicesRoot(r.OperationID, "server-a"), "approved-workflow-scope.json")
	observed, e := fence.ObservePlatformQuarantine(ctx, h.services.window, scopePath, r.WriterControl.WorkflowScopeSHA256, r.ActualRunID, intent.DispatcherSourceSHA, h.writers.token)
	if e != nil {
		return lifecycleError("lifecycle_platform_observation_unproven")
	}
	after, e := readLifecycleAPIRecord(intentPath)
	if e != nil || !bytes.Equal(before, after) || validateLifecycleAPIInvocation(r) != nil || h.services.child.requireLive() != nil || observed.ValidateOriginalWindow(ctx, h.services.window, lifecycleWindowBinding(r)) != nil {
		return lifecycleError("lifecycle_writer_scope_binding_rejected")
	}
	h.writers.platform = observed
	// Current workflows are only one component. Native host/config/session,
	// direct/idle DB admission and external qs-ai writers require their own live
	// installed leases and complete approved census. Those ports are not yet
	// implemented here. No source/environment/receipt may turn this into success.
	return lifecycleError("lifecycle_host_database_external_writer_isolation_unproven")
}
