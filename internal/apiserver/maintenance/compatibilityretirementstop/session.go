package compatibilityretirementstop

import (
	"context"
	"encoding/json"
	"errors"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	"golang.org/x/sys/unix"
	"os"
	"time"
)

const sessionProtocol = "qs-fixed-host-service-session/v1"

type SessionRequest struct {
	Protocol string `json:"protocol"`
	Sequence uint64 `json:"sequence"`
	Action   string `json:"action"`
}
type SessionDiagnostic struct {
	Protocol                     string `json:"protocol"`
	Sequence                     uint64 `json:"sequence"`
	Action                       string `json:"action"`
	HostRole                     string `json:"host_role"`
	SourceSHA                    string `json:"source_sha"`
	ToolSourceSHA                string `json:"tool_source_sha"`
	OperationID                  string `json:"operation_id"`
	ManifestSHA256               string `json:"manifest_sha256"`
	OriginalRunID                string `json:"original_run_id"`
	WindowStartSHA256            string `json:"window_start_sha256"`
	RemainingMilliseconds        int64  `json:"remaining_milliseconds"`
	ForwardRemainingMilliseconds int64  `json:"forward_remaining_milliseconds"`
	Outcome                      string `json:"outcome"`
	ErrorCategory                string `json:"error_category"`
	WholeWriterFenceProven       bool   `json:"whole_writer_fence_proven"`
}

func parseSessionRequest(raw []byte, sequence uint64) (SessionRequest, error) {
	return parseSessionAction(raw, sequence, false)
}

func parseRemoteSessionRequest(raw []byte, sequence uint64) (SessionRequest, error) {
	return parseSessionAction(raw, sequence, true)
}

func parseSessionAction(raw []byte, sequence uint64, remote bool) (SessionRequest, error) {
	var v SessionRequest
	if len(raw) > 1024 || exactJSON(raw, &v) != nil || v.Protocol != sessionProtocol || v.Sequence != sequence || sequence == 0 || sequence > 256 {
		return v, ErrCommand
	}
	if !serviceAction(v.Action) || v.Action == "bind" && !remote {
		return v, ErrCommand
	}
	return v, nil
}

// validSessionFD accepts the kernel transports used for non-PTY SSH stdio.
// Authentication still belongs to the actual root entrypoint and its SSH caller;
// an allowed descriptor by itself never grants stop or retirement authority.
func validSessionFD(f *os.File) bool {
	if f == nil {
		return false
	}
	fd := int(f.Fd())
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil {
		return false
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFIFO:
		return true
	case unix.S_IFSOCK:
		kind, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_TYPE)
		if err != nil || kind != unix.SOCK_STREAM {
			return false
		}
		local, err := unix.Getsockname(fd)
		if err != nil {
			return false
		}
		remote, err := unix.Getpeername(fd)
		if err != nil {
			return false
		}
		l, ok := local.(*unix.SockaddrUnix)
		r, peerOK := remote.(*unix.SockaddrUnix)
		return ok && peerOK && l.Name == "" && r.Name == ""
	default:
		return false
	}
}

func sessionRead(ctx context.Context, f *os.File) ([]byte, error) {
	return sessionReadMax(ctx, f, 1024)
}

func sessionReadMax(ctx context.Context, f *os.File, limit int) ([]byte, error) {
	if f == nil || limit < 1 || limit > 8192 {
		return nil, ErrCommand
	}
	fd := int(f.Fd())
	var b []byte
	for {
		if ctx.Err() != nil {
			return nil, ErrCommand
		}
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, e := unix.Poll(poll, 100)
		if e == unix.EINTR {
			continue
		}
		if e != nil {
			return nil, ErrCommand
		}
		if n == 0 {
			continue
		}
		one := []byte{0}
		n, e = unix.Read(fd, one)
		if e == unix.EINTR {
			continue
		}
		if e != nil || n == 0 {
			return nil, ErrCommand
		}
		if one[0] == '\n' {
			return b, nil
		}
		b = append(b, one[0])
		if len(b) > limit {
			return nil, ErrCommand
		}
	}
}
func sessionWrite(ctx context.Context, f *os.File, v SessionDiagnostic) error {
	if f == nil {
		return ErrCommand
	}
	b, e := json.Marshal(v)
	if e != nil {
		return ErrCommand
	}
	return sessionWriteRaw(ctx, f, b)
}

func sessionWriteRaw(ctx context.Context, f *os.File, b []byte) error {
	if f == nil || len(b) >= 4095 {
		return ErrCommand
	}
	b = append(b, '\n')
	fd := int(f.Fd())
	for {
		if ctx.Err() != nil {
			return ErrCommand
		}
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLOUT}}
		n, e := unix.Poll(poll, 100)
		if e == unix.EINTR {
			continue
		}
		if e != nil {
			return ErrCommand
		}
		if n == 0 {
			continue
		}
		n, e = unix.Write(fd, b)
		if e == unix.EINTR {
			continue
		}
		if e != nil || n <= 0 {
			return ErrCommand
		}
		b = b[n:]
		if len(b) == 0 {
			return nil
		}
	}
}
func sessionCategory(err error) string {
	if err == nil {
		return "none"
	}
	switch {
	case errors.Is(err, ErrState):
		return "service_state_changed"
	case errors.Is(err, ErrForced):
		return "graceful_exit_unproven"
	case errors.Is(err, ErrJournal):
		return "journal_unknown"
	case errors.Is(err, ErrCommand):
		return "fixed_command_failed"
	case errors.Is(err, ErrRemote):
		return "wrong_host"
	case errors.Is(err, ErrRollbackAPI):
		return "bound_rollback_api_required"
	default:
		return "binding_or_budget_rejected"
	}
}

// ServeHostSession is the actual fixed remote service producer for a protected
// root entrypoint. The caller must FIRST authenticate the real SSH key/account,
// obtain AuthorizeProbe's opaque permit and open this host's original Window.
// stdin/stdout and the Window remain host-owned. No shell, arbitrary Docker verb,
// imported JSON success, DB mutation, backup deletion or platform mutation occurs.
// Parent must launch the actual pinned SSH child, bind every reply to this live
// session and repeat Check before using it. Reading a saved reply is insufficient.
// At EOF / lost control / minute 20 forward calls stop and original state stays
// journalled and stopped. The parent must FIRST reconcile in-flight DDL/schema
// and restore this batch's data, then explicitly request service restoration.
// Blindly restarting the A API would run its enabled automatic migrations.
// Normal B success resumes dependents under the forward context; API deployment
// and acceptance belong to the actual parent producer.
func serveHostSession(ctx context.Context, a *Approval, policy fence.Policy, permit *fence.Permit, challengeDir, journalDir string, w *fence.MaintenanceWindow, in, out *os.File, recoveryOnly bool) (err error) {
	if ctx == nil || a.validate() != nil || checkWindow(ctx, a, w) != nil || in == nil || out == nil {
		return ErrBinding
	}
	if !validSessionFD(in) || !validSessionFD(out) {
		return ErrBinding
	}
	var forward context.Context
	var cancel context.CancelFunc
	var e error
	if recoveryOnly {
		forward, cancel, e = w.RecoveryContext(ctx)
	} else {
		forward, cancel, e = w.ForwardContext(ctx)
	}
	if e != nil {
		return e
	}
	defer cancel()
	var l *Lease
	if recoveryOnly {
		if e = consumeAuthenticatedStop(forward, a, policy, permit, challengeDir, w); e != nil {
			return e
		}
		l, e = OpenRecovery(forward, a, journalDir, w)
		if e != nil {
			return e
		}
	}
	released := false
	defer func() {
		if l != nil {

			if ce := l.Close(); err == nil && ce != nil {
				err = ce
			}
		}
	}()
	for seq := uint64(1); seq <= 256; seq++ {
		raw, e := sessionRead(forward, in)
		if e != nil {
			return e
		}
		req, e := parseSessionRequest(raw, seq)
		if e != nil {
			return e
		}
		if recoveryOnly {
			if req.Action != "restore" && req.Action != "restore_dependents" {
				return ErrCommand
			}
		} else if seq == 1 && req.Action != "stop" || seq != 1 && req.Action == "stop" {
			return ErrCommand
		}
		switch req.Action {
		case "stop":
			l, e = StopAndDrainAuthenticated(ctx, a, policy, permit, challengeDir, journalDir, w)
		case "check":
			if l == nil {
				return ErrCommand
			}
			e = l.Check(forward)
		case "resume_dependents":
			if l == nil {
				return ErrCommand
			}
			e = l.ResumeDependents(forward)
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
		replyCtx := forward
		if req.Action == "restore" || req.Action == "restore_dependents" {
			var rc context.CancelFunc
			replyCtx, rc, err = w.RecoveryContext(context.WithoutCancel(ctx))
			if err != nil {
				return err
			}
			defer rc()
		}
		budget, be := w.Diagnostic(replyCtx)
		if be != nil {
			return be
		}
		left := int64(0)
		if budget.RecoverySHA256 == "" {
			left = max(0, budget.RemainingMilliseconds-int64((10*time.Minute)/time.Millisecond))
		}
		diagnostic := SessionDiagnostic{Protocol: sessionProtocol, Sequence: seq, Action: req.Action, HostRole: a.descriptor.HostRole, SourceSHA: a.descriptor.SourceSHA, ToolSourceSHA: a.descriptor.ToolSourceSHA, OperationID: a.descriptor.OperationID, ManifestSHA256: a.descriptor.ManifestSHA256, OriginalRunID: a.descriptor.OriginalRunID, WindowStartSHA256: budget.StartSHA256, RemainingMilliseconds: budget.RemainingMilliseconds, ForwardRemainingMilliseconds: left, Outcome: "observed", ErrorCategory: sessionCategory(e)}
		if e != nil {
			diagnostic.Outcome = "refused"
		}
		if writeErr := sessionWrite(replyCtx, out, diagnostic); writeErr != nil {
			return writeErr
		}
		if e != nil {
			return e
		}
		if released {
			return nil
		}
	}
	return ErrCommand
}

// ServeHostSession is for the actual forward stop session. Authentication input
// is an opaque permit from this process, never a JSON/native success flag.
func ServeHostSession(ctx context.Context, a *Approval, policy fence.Policy, permit *fence.Permit, challengeDir, journalDir string, w *fence.MaintenanceWindow, in, out *os.File) error {
	return serveHostSession(ctx, a, policy, permit, challengeDir, journalDir, w, in, out, false)
}

// ServeRecoverySession is the authenticated takeover of the original protected
// journal/window. It can restore only original dependents; it cannot reconstruct
// Stop/Check success or accept a newly reset window. The parent must already have
// reconciled DDL/schema and supply the separately bound no-migration rollback API.
func ServeRecoverySession(ctx context.Context, a *Approval, policy fence.Policy, permit *fence.Permit, challengeDir, journalDir string, w *fence.MaintenanceWindow, in, out *os.File) error {
	return serveHostSession(ctx, a, policy, permit, challengeDir, journalDir, w, in, out, true)
}
