package compatibilityretirementstop

import (
	"context"
	"errors"
	reader "github.com/FangcunMount/qs-server/internal/pkg/runtimefactsreader"
	"net/http"
	"os"
	"time"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

// ServeRemoteHostSession is the D-only actual fixed authenticated service
// producer. The same pinned SSH process carries request -> freshly generated
// native D challenge -> actual A Window issuer's signature -> actual D action.
// The root trust file is installed by the separately trusted fixed management
// entrypoint, never by this session or a DTO field. This owns only budget/journal
// handles; stdin/stdout and origin auth are caller-owned. Disconnect/deadline
// never starts services. The controller must reconcile actual DDL/data FIRST.
func ServeRemoteHostSession(ctx context.Context, a *Approval, policy fence.Policy, permit *fence.Permit, challengeDir, journalDir string, in, out *os.File, recoveryOnly bool) (err error) {
	if ctx == nil || ctx.Err() != nil || a.validate() != nil || a.descriptor.HostRole != "server-d" || !validSessionFD(in) || !validSessionFD(out) {
		return ErrRemoteBudget
	}
	if e := consumeAuthenticatedStopOrigin(ctx, a, policy, permit, challengeDir); e != nil {
		return e
	}
	return serveRemoteHostSession(ctx, a, journalDir, in, out, recoveryOnly, nil)
}

// ServeRootRemoteHostSession is the service-only port of the existing root
// management channel. It does not claim forced-command/GitHub authentication,
// whole-writer isolation or DROP permission. Its actual root approval, protected
// D trust, fresh native challenges and original A window budget are still
// mandatory. It never installs a key, changes authentication or opens a Window.
func ServeRootRemoteHostSession(ctx context.Context, a *Approval, journalDir string, in, out *os.File, recoveryOnly bool) error {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return ErrBinding
	}
	return serveRemoteHostSession(ctx, a, journalDir, in, out, recoveryOnly, nil)
}

// ServeRootRemoteOwnedSession keeps the original live D management channel and
// owns this process's actual registered temporary files. No completion token is
// accepted. Recovery and legacy sessions retain their existing cleanup rules.
func ServeRootRemoteOwnedSession(ctx context.Context, a *Approval, m *RootRemoteMaterials, journal string, in, out *os.File) error {
	return ServeRootRemoteOwnedSessionWithLoadedMQ(ctx, a, m, journal, in, out, nil)
}

// The fixed root caller owns this read-only HTTP client; the session borrows it.
// Its endpoints are exclusively read from the actual live worker UDS snapshot.
func ServeRootRemoteOwnedSessionWithLoadedMQ(ctx context.Context, a *Approval, m *RootRemoteMaterials, journal string, in, out *os.File, mqClient *http.Client) (result error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 || a == nil || a.materials != nil || m == nil || m.self != m || m.approval != a || m.closed || m.failed.Load() {
		return ErrRemoteMaterials
	}
	a.materials = m
	defer func() {
		if e := m.Close(); result == nil && e != nil {
			result = e
		}
	}()
	return serveRemoteHostSession(ctx, a, journal, in, out, false, mqClient)
}

func serveRemoteHostSession(ctx context.Context, a *Approval, journalDir string, in, out *os.File, recoveryOnly bool, mqClient *http.Client) (err error) {
	if ctx == nil || ctx.Err() != nil || a.validate() != nil || a.descriptor.HostRole != "server-d" || !validSessionFD(in) || !validSessionFD(out) {
		return ErrRemoteBudget
	}
	b, e := OpenRemoteBudget(ctx, a)
	if e != nil {
		return e
	}
	defer func() {
		if ce := b.Close(); err == nil && ce != nil {
			err = ce
		}
	}()
	var l *Lease
	var bound, stopIssued, forwardRefused, recoveryIssued bool
	var controlledIssued, controlledResumed bool
	defer func() {
		if l != nil {
			if ce := l.Close(); err == nil && ce != nil {
				err = ce
			}
		}
	}()
	for seq := uint64(1); seq <= 256; seq++ {
		var readCtx context.Context
		var cancel context.CancelFunc
		if seq == 1 && (!recoveryOnly && b.state.Counter == 0 || recoveryOnly) {
			readCtx, cancel = context.WithTimeout(ctx, 15*time.Second)
		} else {
			// A new signed recovery grant must be able to arrive on this original
			// pipe after the forward deadline. Waiting is bounded by the native
			// original total deadline; it never authorizes a service action.
			readCtx, cancel, e = b.sessionContext(ctx)
		}
		if e != nil {
			return e
		}
		raw, e := sessionRead(readCtx, in)
		cancel()
		if e != nil {
			return e
		}
		req, e := parseRemoteSessionRequest(raw, seq)
		if e != nil {
			return e
		}
		if recoveryOnly {
			if !recoveryAction(req.Action) {
				return ErrCommand
			}
		} else {
			if !remoteSessionRequestAllowed(req.Action, bound, stopIssued, forwardRefused, recoveryIssued, controlledIssued, controlledResumed) {
				return ErrCommand
			}
			if req.Action == "controlled_resume" {
				controlledIssued = true
			}
			if req.Action == "bind" {
				bound = true
			}
			if req.Action == "stop" {
				stopIssued = true
			}
			if recoveryReleaseAction(req.Action) {
				recoveryIssued = true
			}
		}
		// Root native challenge precedes ALL service actions, including every Check.
		c, e := b.BeginChallenge(ctx, req.Action)
		if e != nil {
			return e
		}
		challenge, e := c.Request()
		if e != nil {
			return e
		}
		handshakeParent := ctx
		var endHandshakeParent context.CancelFunc
		if b.state.Counter != 0 {
			handshakeParent, endHandshakeParent, e = b.sessionContext(ctx)
			if e != nil {
				return e
			}
		}
		handshake, cancel := context.WithTimeout(handshakeParent, 15*time.Second)
		if e = sessionWriteRaw(handshake, out, challenge); e != nil {
			cancel()
			if endHandshakeParent != nil {
				endHandshakeParent()
			}
			return e
		}
		grant, e := sessionReadMax(handshake, in, 4096)
		if e == nil {
			e = b.AcceptGrant(handshake, c, grant)
		}
		cancel()
		if endHandshakeParent != nil {
			endHandshakeParent()
		}
		if e != nil {
			return e
		}
		if recoveryOnly && l == nil {
			l, e = OpenRecoveryRemote(ctx, a, journalDir, b)
			if e != nil {
				return e
			}
		}
		released := false
		var actualRuntime *DependentRuntimeSnapshot
		var actualMaterials *RemoteMaterialSnapshot
		var actualMQ *LoadedMQDiagnostic
		var actualPrincipals []DatabasePrincipal
		switch req.Action {
		case "bind":
			// Actual descriptor/trust, fresh native signature and original budget
			// were verified above. Reobserve the actual approved D service catalog
			// before fencing its SSH key. No Lease or writer fence is produced.
			e = validateRemoteManagementCatalog(ctx, a, b)
		case "stop":
			l, e = StopAndDrainRemote(ctx, a, journalDir, b)
		case "check", "check_recovery":
			if l == nil {
				return ErrCommand
			}
			e = l.Check(ctx)
		case "observe_db_principals":
			if l == nil {
				return ErrCommand
			}
			actualPrincipals, e = l.ObserveDatabasePrincipals(ctx)
		case "resume_dependents":
			if l == nil {
				return ErrCommand
			}
			e = l.ResumeDependents(ctx)
			released = e == nil
		case "controlled_resume":
			if l == nil {
				return ErrCommand
			}
			e = l.ControlledResumeDependents(ctx)
			controlledResumed = e == nil
		case "check_running", "check_running_recovery":
			if l == nil {
				return ErrCommand
			}
			var observed *DependentRuntimeObservation
			observed, e = l.ObserveRunningDependents(ctx)
			if e == nil {
				var snapshot DependentRuntimeSnapshot
				snapshot, e = observed.Snapshot()
				if e == nil {
					actualRuntime = &snapshot
				}
			}
		case "observe_loaded_mq":
			if l == nil || l.runtimeObservation == nil || mqClient == nil {
				return ErrLoadedMQ
			}
			var snapshot LoadedMQDiagnostic
			snapshot, e = l.ObserveLoadedMQ(ctx, mqClient)
			if errors.Is(e, reader.ErrScopeUnproven) && loadedMQDiagnosticValid(&snapshot) {
				actualMQ = &snapshot
				e = nil
			}
		case "purge_materials":
			if l == nil || a.materials == nil || l.runtimeObservation == nil {
				return ErrRemoteMaterials
			}
			// Reobserve from this original Lease immediately before deleting its
			// journals. No reconnect, saved runtime DTO or requested CID is used.
			_, e = l.ObserveRunningDependents(ctx)
			if e == nil {
				var snapshot RemoteMaterialSnapshot
				snapshot, e = a.materials.purge(ctx, b, l)
				if e == nil {
					actualMaterials = &snapshot
					released = true
				}
			}
		case "restore":
			if l == nil {
				return ErrCommand
			}
			e = l.Restore(ctx)
			released = e == nil
		case "restore_dependents":
			if l == nil {
				return ErrCommand
			}
			e = l.RestoreDependents(ctx)
			released = e == nil
		}
		budget, be := b.Diagnostic(ctx)
		if be != nil {
			return be
		}
		remaining, be := b.RemainingForward(ctx)
		if be != nil {
			return be
		}
		diagnostic := SessionDiagnostic{Protocol: sessionProtocol, Sequence: seq, Action: req.Action, HostRole: a.descriptor.HostRole, SourceSHA: a.descriptor.SourceSHA, ToolSourceSHA: a.descriptor.ToolSourceSHA, OperationID: a.descriptor.OperationID, ManifestSHA256: a.descriptor.ManifestSHA256, OriginalRunID: a.descriptor.OriginalRunID, WindowStartSHA256: budget.StartSHA256, RemainingMilliseconds: budget.RemainingMilliseconds, ForwardRemainingMilliseconds: remaining, Outcome: "observed", ErrorCategory: sessionCategory(e)}
		diagnostic.Runtime = actualRuntime
		diagnostic.Materials = actualMaterials
		diagnostic.LoadedMQ = actualMQ
		diagnostic.DatabasePrincipals = actualPrincipals
		if e != nil {
			diagnostic.Outcome = "refused"
		}
		var reply context.Context
		if recoveryAction(req.Action) {
			reply, cancel, be = b.RecoveryContext(ctx)
		} else {
			reply, cancel, be = b.ForwardContext(ctx)
		}
		if be != nil {
			return be
		}
		writeErr := sessionWrite(reply, out, diagnostic)
		cancel()
		if writeErr != nil {
			return writeErr
		}
		if e != nil {
			if req.Action == "purge_materials" || l == nil || recoveryAction(req.Action) || recoveryOnly {
				return e
			}
			// A real refused action may already have a native partial Lease. Keep
			// it in this process and accept only a fresh signed recovery action.
			// No failed or unknown Stop is issued again.
			forwardRefused = true
			continue
		}
		if released {
			return nil
		}
	}
	return ErrCommand
}

func validateRemoteManagementCatalog(ctx context.Context, a *Approval, b *RemoteBudget) error {
	if a == nil || b == nil || b.self != b || b.approval != a {
		return ErrRemoteBudget
	}
	q, cancel, e := b.ForwardContext(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	actual, e := a.catalog(q)
	if e != nil || !remoteManagementCatalogMatches(actual, a.descriptor.Containers) {
		return ErrState
	}
	return nil
}

func remoteManagementCatalogMatches(actual []actualContainer, expected []Container) bool {
	if len(actual) != len(expected) || len(expected) == 0 {
		return false
	}
	want := map[string]Container{}
	for _, v := range expected {
		if _, exists := want[v.ID]; exists {
			return false
		}
		want[v.ID] = v
	}
	for _, v := range actual {
		original, exists := want[v.ID]
		if !exists || !sameIdentity(v, original) || v.Running != original.Running || v.StartedAt != original.StartedAt || v.Paused || v.Restarting || v.Dead || v.OOMKilled ||
			v.Running && v.PID <= 0 || !v.Running && (v.PID != 0 || v.ExitCode != 0) {
			return false
		}
		delete(want, v.ID)
	}
	return len(want) == 0
}

func remoteSessionActionAllowed(action string, bound, stopIssued, forwardRefused, recoveryIssued bool) bool {
	if recoveryIssued || !serviceAction(action) {
		return false
	}
	if recoveryAction(action) {
		return stopIssued
	}
	if forwardRefused {
		return false
	}
	switch action {
	case "bind":
		return !bound && !stopIssued
	case "stop":
		return !stopIssued // Preserves the legacy initial Stop route.
	case "check", "observe_db_principals", "resume_dependents", "controlled_resume", "check_running", "observe_loaded_mq", "purge_materials":
		return stopIssued
	}
	return false
}

// This is the actual live-session request gate, before the fresh native budget
// challenge. An allowed request still needs its original signed grant, Lease,
// current runtime observation and exact registered material owner before purge.
func remoteSessionRequestAllowed(action string, bound, stopIssued, forwardRefused, recoveryIssued, controlledIssued, controlledResumed bool) bool {
	if !remoteSessionActionAllowed(action, bound, stopIssued, forwardRefused, recoveryIssued) {
		return false
	}
	if action == "check_running_recovery" {
		return bound && stopIssued && controlledResumed
	}
	if action == "observe_db_principals" {
		return bound && stopIssued && !controlledIssued
	}
	return !controlledAction(action) || bound &&
		(action != "controlled_resume" || !controlledIssued) &&
		(action != "check_running" && action != "observe_loaded_mq" && action != "purge_materials" || controlledResumed)
}
