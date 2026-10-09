package compatibilityretirementstop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// RemoteController borrows the LIVE pipes of the existing actually authenticated
// pinned SSH child. Its constructor cannot prove SSH provenance: the fixed host
// caller must establish that separately and keep the child bound to these files.
// This producer exchanges fresh native challenges/signatures and actual replies;
// saved JSON is not accepted by this API, and its result is never a DROP permit.
type RemoteController struct {
	self                                   *RemoteController
	mu                                     sync.Mutex
	issuer                                 *BudgetIssuer
	in, out                                *os.File
	inDevice, inInode, outDevice, outInode uint64
	seq                                    uint64
	closed                                 bool
	managementBound, stopIssued            bool
	forwardRefused, recoveryIssued         bool
	controlledIssued, controlledResumed    bool
	runtimeObservation                     *RemoteRuntimeObservation
	materialsZero                          *RemoteMaterialZero
}

var ErrRemoteActionRefused = errors.New("actual remote service action refused")

func (*RemoteController) MarshalJSON() ([]byte, error) { return nil, ErrRemoteBudget }
func OpenLiveRemoteController(ctx context.Context, i *BudgetIssuer, in, out *os.File) (*RemoteController, error) {
	if ctx == nil || ctx.Err() != nil || i == nil || i.self != i || i.approval.validate() != nil || !validSessionFD(in) || !validSessionFD(out) {
		return nil, ErrRemoteBudget
	}
	var a, b unix.Stat_t
	if unix.Fstat(int(in.Fd()), &a) != nil || unix.Fstat(int(out.Fd()), &b) != nil {
		return nil, ErrRemoteBudget
	}
	c := &RemoteController{issuer: i, in: in, out: out, inDevice: uint64(a.Dev), inInode: uint64(a.Ino), outDevice: uint64(b.Dev), outInode: uint64(b.Ino)}
	c.self = c
	return c, nil
}
func (c *RemoteController) Do(ctx context.Context, action string) (v SessionDiagnostic, err error) {
	if c == nil || c.self != c {
		return v, ErrRemoteBudget
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || ctx == nil || ctx.Err() != nil || !serviceAction(action) || c.seq >= 256 {
		return v, ErrRemoteBudget
	}
	if action == "bind" && c.seq != 0 || action == "stop" && c.stopIssued || c.forwardRefused && !recoveryAction(action) || recoveryAction(action) && (c.recoveryIssued || c.seq != 0 && !c.stopIssued) ||
		(action == "check" || action == "resume_dependents" || controlledAction(action)) && !c.stopIssued || controlledAction(action) && (!c.managementBound || action == "controlled_resume" && c.controlledIssued || (action == "check_running" || action == "purge_materials") && !c.controlledResumed || action == "purge_materials" && c.runtimeObservation == nil) {
		return v, ErrRemoteBudget
	}
	var a, b unix.Stat_t
	if !validSessionFD(c.in) || !validSessionFD(c.out) || unix.Fstat(int(c.in.Fd()), &a) != nil || unix.Fstat(int(c.out.Fd()), &b) != nil || uint64(a.Dev) != c.inDevice || uint64(a.Ino) != c.inInode || uint64(b.Dev) != c.outDevice || uint64(b.Ino) != c.outInode {
		return v, ErrRemoteBudget
	}
	d, e := c.issuer.window.Diagnostic(ctx)
	if e != nil || recoveryAction(action) != (d.RecoverySHA256 != "") {
		return v, ErrRemoteBudget
	}
	var q context.Context
	var cancel context.CancelFunc
	if recoveryAction(action) {
		q, cancel, e = c.issuer.window.RecoveryContext(ctx)
	} else {
		q, cancel, e = c.issuer.window.ForwardContext(ctx)
	}
	if e != nil {
		return v, e
	}
	defer cancel()
	// An unknown transport/action result closes this controller. Reconnection is
	// through the original persistent recovery budget, never a replay/new window.
	knownRefused := false
	defer func() {
		if err != nil && !knownRefused {
			c.closed = true
		}
	}()
	if action == "stop" {
		c.stopIssued = true // Actual issued responsibility, not a stop proof.
	}
	if action == "controlled_resume" {
		c.controlledIssued = true
	}
	if recoveryAction(action) {
		c.recoveryIssued = true
	}
	c.seq++
	request, e := json.Marshal(SessionRequest{sessionProtocol, c.seq, action})
	if e != nil {
		return v, ErrRemoteBudget
	}
	if e = sessionWriteRaw(q, c.out, request); e != nil {
		return v, e
	}
	raw, e := sessionRead(q, c.in)
	if e != nil {
		return v, e
	}
	var challenge RemoteBudgetChallenge
	if exactJSON(raw, &challenge) != nil || challenge.Action != action || challenge.RemoteDescriptorSHA256 != c.issuer.record.RemoteDescriptorSHA256 {
		return v, ErrRemoteBudget
	}
	grant, e := c.issuer.IssueFreshBudget(q, raw)
	if e != nil {
		return v, e
	}
	if e = sessionWriteRaw(q, c.out, grant); e != nil {
		return v, e
	}
	reply, e := sessionReadMax(q, c.in, 4096)
	if e != nil {
		return v, e
	}
	if exactJSON(reply, &v) != nil || v.Protocol != sessionProtocol || v.Sequence != c.seq || v.Action != action || v.HostRole != "server-d" || v.SourceSHA != c.issuer.record.Binding.SourceSHA || v.ToolSourceSHA != c.issuer.record.ToolSourceSHA || v.OperationID != c.issuer.record.Binding.OperationID || v.ManifestSHA256 != c.issuer.record.Binding.ManifestSHA256 || v.OriginalRunID != c.issuer.record.Binding.OriginalRunID || v.WindowStartSHA256 != c.issuer.record.StartSHA256 || v.WholeWriterFenceProven || v.RemainingMilliseconds <= 0 || v.RemainingMilliseconds > 1800000 || v.ForwardRemainingMilliseconds < 0 || v.ForwardRemainingMilliseconds > 1200000 || !remoteDiagnosticOutcomeValid(v) {
		return v, ErrRemoteBudget
	}
	if !remoteRuntimeDiagnosticValid(v) || !remoteMaterialsDiagnosticValid(v) {
		return v, ErrRemoteBudget
	}
	after, e := c.issuer.window.Diagnostic(q)
	if e != nil || after.StartSHA256 != c.issuer.record.StartSHA256 || recoveryAction(action) != (after.RecoverySHA256 != "") {
		return v, ErrRemoteBudget
	}
	if !recoveryAction(action) && v.ForwardRemainingMilliseconds <= 0 || recoveryAction(action) && v.ForwardRemainingMilliseconds != 0 {
		return v, ErrRemoteBudget
	}
	if v.Outcome == "refused" {
		knownRefused = true
		c.forwardRefused = true
		return v, ErrRemoteActionRefused
	}
	if action == "bind" {
		c.managementBound = true
	}
	if action == "controlled_resume" {
		c.controlledResumed = true
	}
	if action == "check_running" {
		o := &RemoteRuntimeObservation{controller: c, sequence: c.seq, snapshot: *v.Runtime, seal: runtimeDigest(*v.Runtime)}
		o.self = o
		c.runtimeObservation = o
	}
	if action == "purge_materials" {
		z := &RemoteMaterialZero{controller: c, sequence: c.seq, snapshot: *v.Materials, seal: runtimeDigest(*v.Materials)}
		z.self = z
		c.materialsZero = z
		c.closed = true
	}
	return v, nil
}

func remoteRuntimeDiagnosticValid(v SessionDiagnostic) bool {
	if v.Action != "check_running" || v.Outcome != "observed" {
		return v.Runtime == nil
	}
	if v.Runtime == nil || len(v.Runtime.Instances) == 0 || len(v.Runtime.Instances) > 32 {
		return false
	}
	seen := map[string]bool{}
	for _, r := range v.Runtime.Instances {
		if !hash64.MatchString(r.ContainerID) || seen[r.ContainerID] || !strings.HasPrefix(r.ImageID, "sha256:") || !hash64.MatchString(strings.TrimPrefix(r.ImageID, "sha256:")) || !hash64.MatchString(r.ProgramSHA256) || !hash64.MatchString(r.StateSHA256) || !hash64.MatchString(r.ReadySHA256) {
			return false
		}
		seen[r.ContainerID] = true
	}
	return true
}
func remoteDiagnosticOutcomeValid(v SessionDiagnostic) bool {
	if v.Outcome == "observed" {
		return v.ErrorCategory == "none"
	}
	if v.Outcome != "refused" {
		return false
	}
	switch v.ErrorCategory {
	case "service_state_changed", "graceful_exit_unproven", "journal_unknown", "fixed_command_failed", "wrong_host", "bound_rollback_api_required", "binding_or_budget_rejected":
		return true
	}
	return false
}

// Continuation reuses this exact native controller and pipes. This is only a
// transport precondition; the next Do still requires a fresh D challenge, the
// original actual Window's recovery signature and the retained native D Lease.
func (c *RemoteController) ValidateRecoveryContinuation(ctx context.Context) error {
	if c == nil || c.self != c || ctx == nil || ctx.Err() != nil {
		return ErrRemoteBudget
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.managementBound || !c.stopIssued || c.recoveryIssued || c.issuer == nil || c.issuer.approval.validate() != nil || !validSessionFD(c.in) || !validSessionFD(c.out) {
		return ErrRemoteBudget
	}
	var a, b unix.Stat_t
	if unix.Fstat(int(c.in.Fd()), &a) != nil || unix.Fstat(int(c.out.Fd()), &b) != nil || uint64(a.Dev) != c.inDevice || uint64(a.Ino) != c.inInode || uint64(b.Dev) != c.outDevice || uint64(b.Ino) != c.outInode {
		return ErrRemoteBudget
	}
	d, e := c.issuer.window.Diagnostic(ctx)
	if e != nil || d.StartSHA256 != c.issuer.record.StartSHA256 || d.Binding != c.issuer.record.Binding || d.RecoverySHA256 == "" || d.RemainingMilliseconds <= 0 {
		return ErrRemoteBudget
	}
	return nil
}
func (c *RemoteController) Close() error {
	if c == nil || c.self != c {
		return ErrRemoteBudget
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}
