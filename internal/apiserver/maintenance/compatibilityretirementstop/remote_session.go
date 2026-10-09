package compatibilityretirementstop

import (
	"context"
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
	return serveRemoteHostSession(ctx, a, journalDir, in, out, recoveryOnly)
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
	return serveRemoteHostSession(ctx, a, journalDir, in, out, recoveryOnly)
}

func serveRemoteHostSession(ctx context.Context, a *Approval, journalDir string, in, out *os.File, recoveryOnly bool) (err error) {
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
			if !remoteSessionActionAllowed(req.Action, bound, stopIssued, forwardRefused, recoveryIssued) {
				return ErrCommand
			}
			if req.Action == "bind" {
				bound = true
			}
			if req.Action == "stop" {
				stopIssued = true
			}
			if recoveryAction(req.Action) {
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
		switch req.Action {
		case "bind":
			// Actual descriptor/trust, fresh native signature and original budget
			// were verified above. Reobserve the actual approved D service catalog
			// before fencing its SSH key. No Lease or writer fence is produced.
			e = validateRemoteManagementCatalog(ctx, a, b)
		case "stop":
			l, e = StopAndDrainRemote(ctx, a, journalDir, b)
		case "check":
			if l == nil {
				return ErrCommand
			}
			e = l.Check(ctx)
		case "resume_dependents":
			if l == nil {
				return ErrCommand
			}
			e = l.ResumeDependents(ctx)
			released = e == nil
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
			if l == nil || recoveryAction(req.Action) || recoveryOnly {
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
	case "check", "resume_dependents":
		return stopIssued
	}
	return false
}
