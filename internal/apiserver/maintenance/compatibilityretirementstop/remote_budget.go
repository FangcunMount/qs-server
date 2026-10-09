package compatibilityretirementstop

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"syscall"
	"time"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

// This is TRUST CONFIGURATION, never a runtime completion flag. It is loaded
// only from this batch's fixed root-managed path, with its exact bytes already
// bound by BudgetTrustSHA256 in the approved root descriptor. The actual trusted
// fixed management entrypoint must install the native A issuer's public key.
// No function here installs a supplied DTO key or promotes it to DROP authority.
type remoteBudgetTrust struct {
	Protocol        string              `json:"protocol"`
	Binding         fence.WindowBinding `json:"binding"`
	ToolSourceSHA   string              `json:"tool_source_sha"`
	MachineIDSHA256 string              `json:"machine_id_sha256"`
	IssuerPublicKey string              `json:"issuer_public_key"`
}

func readRemoteBudgetTrust(a *Approval) (remoteBudgetTrust, error) {
	var t remoteBudgetTrust
	if a == nil || a.validate() != nil || a.descriptor.HostRole != "server-d" || !hash64.MatchString(a.descriptor.BudgetTrustSHA256) {
		return t, ErrRemoteBudget
	}
	path := filepath.Join("/opt/qs-server/qs-worker/compatibility-retirement", a.descriptor.OperationID, "budget-trust.json")
	raw, e := readRootBudgetFile(path)
	want, e2 := a.WindowBinding(context.Background())
	if e != nil || e2 != nil || digest(raw) != a.descriptor.BudgetTrustSHA256 || exactJSON(raw, &t) != nil || t.Protocol != budgetProtocol || t.Binding != want || t.ToolSourceSHA != a.descriptor.ToolSourceSHA || t.MachineIDSHA256 != a.descriptor.MachineIDSHA256 || !hash64.MatchString(t.IssuerPublicKey) {
		return t, ErrRemoteBudget
	}
	return t, nil
}
func decodeBudget(raw []byte, trust remoteBudgetTrust, expectedRemoteSHA string) (signedRemoteBudget, error) {
	var s signedRemoteBudget
	if len(raw) > 4096 || exactJSON(raw, &s) != nil {
		return s, ErrRemoteBudget
	}
	p := s.Payload
	public, e := hex.DecodeString(trust.IssuerPublicKey)
	sig, se := hex.DecodeString(s.Signature)
	canonical, ce := json.Marshal(p)
	if e != nil || se != nil || ce != nil || len(public) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize || !ed25519.Verify(public, canonical, sig) || p.ToolSourceSHA != trust.ToolSourceSHA || p.Protocol != budgetProtocol || p.Binding != trust.Binding || p.RemoteDescriptorSHA256 != expectedRemoteSHA || p.Counter == 0 || p.Counter > 4096 || !hash64.MatchString(p.Nonce) || !hash64.MatchString(p.StartSHA256) || !serviceAction(p.Action) || p.RemainingMilliseconds <= 0 || p.RemainingMilliseconds > 1800000 || p.ForwardMilliseconds < 0 || p.ForwardMilliseconds > 1200000 {
		return s, ErrRemoteBudget
	}
	if p.RecoverySHA256 == "" {
		if recoveryAction(p.Action) || p.ForwardMilliseconds != max(int64(0), p.RemainingMilliseconds-600000) {
			return s, ErrRemoteBudget
		}
	} else if !hash64.MatchString(p.RecoverySHA256) || !recoveryAction(p.Action) || p.ForwardMilliseconds != 0 || p.RemainingMilliseconds > 600000 {
		return s, ErrRemoteBudget
	}
	return s, nil
}

type remoteBudgetState struct {
	Counter         uint64 `json:"counter"`
	BootID          string `json:"boot_id"`
	StartSHA256     string `json:"start_sha256"`
	RecoverySHA256  string `json:"recovery_sha256"`
	TotalDeadline   int64  `json:"total_deadline_boot_nanos"`
	ForwardDeadline int64  `json:"forward_deadline_boot_nanos"`
}
type remoteBudgetRecord struct {
	Protocol             string             `json:"protocol"`
	ParentSHA256         string             `json:"parent_sha256"`
	BeforeChallengeNanos int64              `json:"before_challenge_boot_nanos"`
	State                remoteBudgetState  `json:"state"`
	Grant                signedRemoteBudget `json:"grant"`
}

func candidateBudgetState(old remoteBudgetState, p remoteBudgetPayload, boot string, before int64) (remoteBudgetState, error) {
	if before < 0 || before > math.MaxInt64-p.RemainingMilliseconds*int64(time.Millisecond) || before > math.MaxInt64-p.ForwardMilliseconds*int64(time.Millisecond) || p.Counter != old.Counter+1 {
		return remoteBudgetState{}, ErrRemoteBudget
	}
	n := remoteBudgetState{Counter: p.Counter, BootID: boot, StartSHA256: p.StartSHA256, RecoverySHA256: p.RecoverySHA256, TotalDeadline: before + p.RemainingMilliseconds*int64(time.Millisecond), ForwardDeadline: before + p.ForwardMilliseconds*int64(time.Millisecond)}
	if p.RecoverySHA256 != "" {
		n.ForwardDeadline = 0
	}
	if old.Counter > 0 {
		if old.BootID != boot || old.StartSHA256 != p.StartSHA256 || old.RecoverySHA256 != "" && old.RecoverySHA256 != p.RecoverySHA256 || old.RecoverySHA256 != "" && p.RecoverySHA256 == "" {
			return n, ErrRemoteBudget
		}
		n.TotalDeadline = min(n.TotalDeadline, old.TotalDeadline)
		if n.RecoverySHA256 == "" {
			n.ForwardDeadline = min(n.ForwardDeadline, old.ForwardDeadline)
		}
	}
	if n.TotalDeadline <= before || n.RecoverySHA256 == "" && n.ForwardDeadline > n.TotalDeadline {
		return n, ErrRemoteBudget
	}
	return n, nil
}

// RemoteBudget owns only this D host's protected budget-record directory lease.
// It is a budget-only opaque native receiver; it grants no writer/DDL permission.
// Its clock starts BEFORE a freshly generated challenge is sent to A. A's live
// actual Window signature is verified using the fixed root trust configuration.
// Whole-network round trip is therefore conservatively deducted without clock
// synchronization. Every subsequent bound can only shorten the original bound.
type RemoteBudget struct {
	self       *RemoteBudget
	mu         sync.Mutex
	approval   *Approval
	trust      remoteBudgetTrust
	dir        string
	dirFD      int
	state      remoteBudgetState
	recordHash string
	lastNanos  int64
	pending    *BudgetChallenge
	closed     bool
	done       context.Context
	cancel     context.CancelFunc
}

func (*RemoteBudget) MarshalJSON() ([]byte, error) { return nil, ErrRemoteBudget }

type BudgetChallenge struct {
	self    *BudgetChallenge
	owner   *RemoteBudget
	request RemoteBudgetChallenge
	boot    string
	before  int64
}

func (*BudgetChallenge) MarshalJSON() ([]byte, error) { return nil, ErrRemoteBudget }
func (c *BudgetChallenge) Request() ([]byte, error) {
	if c == nil || c.self != c || c.owner == nil {
		return nil, ErrRemoteBudget
	}
	c.owner.mu.Lock()
	defer c.owner.mu.Unlock()
	if c.owner.closed || c.owner.pending != c {
		return nil, ErrRemoteBudget
	}
	return json.Marshal(c.request)
}

var grantName = regexp.MustCompile(`^grant-([0-9]{4})\.json$`)

func OpenRemoteBudget(ctx context.Context, a *Approval) (out *RemoteBudget, err error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrRemoteBudget
	}
	trust, e := readRemoteBudgetTrust(a)
	if e != nil {
		return nil, e
	}
	dir := budgetRemoteDirectory(a)
	fd, e := openJournal(dir)
	if e != nil {
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			_ = syscall.Close(fd)
		}
	}()
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) > 4096 {
		return nil, ErrRemoteBudget
	}
	names := []string{}
	for _, entry := range entries {
		if !grantName.MatchString(entry.Name()) || entry.IsDir() {
			return nil, ErrRemoteBudget
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	done, cancel := context.WithCancel(context.Background())
	b := &RemoteBudget{approval: a, trust: trust, dir: dir, dirFD: fd, done: done, cancel: cancel}
	b.self = b
	boot, now, e := remoteBootClock()
	if e != nil {
		cancel()
		return nil, e
	}
	b.lastNanos = now
	for n, name := range names {
		if name != fmt.Sprintf("grant-%04d.json", n+1) {
			cancel()
			return nil, ErrRemoteBudget
		}
		raw, e := readRootBudgetFile(filepath.Join(dir, name))
		var rec remoteBudgetRecord
		if e != nil || len(raw) > 8192 || exactJSON(raw, &rec) != nil || rec.Protocol != budgetProtocol || rec.ParentSHA256 != b.recordHash || rec.BeforeChallengeNanos > now || rec.State.BootID != boot {
			cancel()
			return nil, ErrRemoteBudget
		}
		grantRaw, e := json.Marshal(rec.Grant)
		if e != nil {
			cancel()
			return nil, ErrRemoteBudget
		}
		grant, e := decodeBudget(grantRaw, trust, a.rawHash)
		if e != nil {
			cancel()
			return nil, e
		}
		want, e := candidateBudgetState(b.state, grant.Payload, boot, rec.BeforeChallengeNanos)
		if e != nil || want != rec.State {
			cancel()
			return nil, ErrRemoteBudget
		}
		b.state = rec.State
		b.recordHash = digest(raw)
	}
	if b.state.Counter > 0 && now >= b.state.TotalDeadline {
		cancel()
		return nil, ErrRemoteBudget
	}
	ok = true
	return b, nil
}
func (b *RemoteBudget) clockLocked(ctx context.Context) (string, int64, error) {
	if b.closed || ctx == nil || ctx.Err() != nil || b.approval.validate() != nil {
		return "", 0, ErrRemoteBudget
	}
	trust, e := readRemoteBudgetTrust(b.approval)
	if e != nil || trust != b.trust {
		return "", 0, ErrRemoteBudget
	}
	boot, now, e := remoteBootClock()
	if e != nil || now < b.lastNanos || b.state.Counter > 0 && (boot != b.state.BootID || now >= b.state.TotalDeadline) {
		return "", 0, ErrRemoteBudget
	}
	if b.state.Counter > 0 {
		raw, e := readRootBudgetFile(filepath.Join(b.dir, fmt.Sprintf("grant-%04d.json", b.state.Counter)))
		if e != nil || digest(raw) != b.recordHash {
			return "", 0, ErrRemoteBudget
		}
	}
	b.lastNanos = now
	return boot, now, nil
}

// BeginChallenge returns an opaque one-use challenge, never caller supplied time.
// The native sample precedes transmission. A failed/unknown answer is consumed;
// the host may request another fresh nonce only within the original bounds.
func (b *RemoteBudget) BeginChallenge(ctx context.Context, action string) (*BudgetChallenge, error) {
	if b == nil || b.self != b {
		return nil, ErrRemoteBudget
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending != nil || !serviceAction(action) || b.state.Counter >= 4096 {
		return nil, ErrRemoteBudget
	}
	boot, before, e := b.clockLocked(ctx)
	if e != nil {
		return nil, e
	}
	if b.state.Counter > 0 && !recoveryAction(action) && (b.state.RecoverySHA256 != "" || before >= b.state.ForwardDeadline) {
		return nil, ErrRemoteBudget
	}
	nonce := make([]byte, 32)
	if _, e = rand.Read(nonce); e != nil {
		return nil, ErrRemoteBudget
	}
	c := &BudgetChallenge{owner: b, boot: boot, before: before, request: RemoteBudgetChallenge{budgetProtocol, b.state.Counter + 1, hex.EncodeToString(nonce), action, b.approval.rawHash}}
	c.self = c
	b.pending = c
	return c, nil
}
func (b *RemoteBudget) AcceptGrant(ctx context.Context, c *BudgetChallenge, raw []byte) error {
	if b == nil || b.self != b {
		return ErrRemoteBudget
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if c == nil || c.self != c || c.owner != b || b.pending != c {
		return ErrRemoteBudget
	}
	b.pending = nil // any response consumes this challenge, including rejected input
	boot, now, e := b.clockLocked(ctx)
	if e != nil || boot != c.boot || now < c.before || now-c.before > int64(15*time.Second) {
		return ErrRemoteBudget
	}
	signed, e := decodeBudget(raw, b.trust, b.approval.rawHash)
	if e != nil {
		return e
	}
	p := signed.Payload
	if p.Counter != c.request.Counter || p.Nonce != c.request.Nonce || p.Action != c.request.Action {
		return ErrRemoteBudget
	}
	state, e := candidateBudgetState(b.state, p, boot, c.before)
	if e != nil || now >= state.TotalDeadline || !recoveryAction(p.Action) && now >= state.ForwardDeadline {
		return ErrRemoteBudget
	}
	record := remoteBudgetRecord{budgetProtocol, b.recordHash, c.before, state, signed}
	data, e := json.Marshal(record)
	if e != nil {
		return ErrRemoteBudget
	}
	name := fmt.Sprintf("grant-%04d.json", state.Counter)
	if e = writeRootExclusive(b.dirFD, name, data); e != nil {
		return e
	}
	saved, e := readRootBudgetFile(filepath.Join(b.dir, name))
	if e != nil || !bytes.Equal(saved, data) {
		return ErrRemoteBudget
	}
	b.state = state
	b.recordHash = digest(data)
	return nil
}
func (b *RemoteBudget) Diagnostic(ctx context.Context) (fence.WindowBudgetReceipt, error) {
	if b == nil || b.self != b {
		return fence.WindowBudgetReceipt{}, ErrRemoteBudget
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	_, now, e := b.clockLocked(ctx)
	if e != nil || b.state.Counter == 0 {
		return fence.WindowBudgetReceipt{}, ErrRemoteBudget
	}
	return fence.WindowBudgetReceipt{Protocol: budgetProtocol, Binding: b.trust.Binding, BudgetOnly: true, DirectoryLeaseHeld: true, ClockSource: "linux_clock_boottime_live_original_window_v1", StartSHA256: b.state.StartSHA256, RecoverySHA256: b.state.RecoverySHA256, RemainingMilliseconds: max(int64(0), b.state.TotalDeadline-now) / int64(time.Millisecond)}, nil
}
func (b *RemoteBudget) RemainingForward(ctx context.Context) (int64, error) {
	if b == nil || b.self != b {
		return 0, ErrRemoteBudget
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	_, now, e := b.clockLocked(ctx)
	if e != nil || b.state.Counter == 0 {
		return 0, ErrRemoteBudget
	}
	if b.state.RecoverySHA256 != "" {
		return 0, nil
	}
	return max(int64(0), b.state.ForwardDeadline-now) / int64(time.Millisecond), nil
}

type remoteBudgetContextKind uint8

const (
	remoteForwardContext remoteBudgetContextKind = iota
	remoteRecoveryContext
	remoteSessionContext
)

func remoteBudgetContextLimit(s remoteBudgetState, kind remoteBudgetContextKind) (int64, bool) {
	if s.Counter == 0 {
		return 0, false
	}
	switch kind {
	case remoteForwardContext:
		return s.ForwardDeadline, s.RecoverySHA256 == ""
	case remoteRecoveryContext:
		return s.TotalDeadline, s.RecoverySHA256 != ""
	case remoteSessionContext:
		// Waiting for a control packet is not a service action. Only the same
		// native signed original total bound may cover its phase transition.
		return s.TotalDeadline, true
	}
	return 0, false
}

func (b *RemoteBudget) bound(ctx context.Context, kind remoteBudgetContextKind) (context.Context, context.CancelFunc, error) {
	if b == nil || b.self != b {
		return nil, nil, ErrRemoteBudget
	}
	b.mu.Lock()
	_, now, e := b.clockLocked(ctx)
	limit, allowed := remoteBudgetContextLimit(b.state, kind)
	if e != nil || !allowed || now >= limit {
		b.mu.Unlock()
		return nil, nil, ErrRemoteBudget
	}
	q, cancel := context.WithTimeout(ctx, time.Duration(limit-now))
	b.mu.Unlock()
	go func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-q.Done():
				return
			case <-b.done.Done():
				cancel()
				return
			case <-ticker.C:
				b.mu.Lock()
				_, n, e := b.clockLocked(q)
				currentLimit, allowed := remoteBudgetContextLimit(b.state, kind)
				b.mu.Unlock()
				if e != nil || !allowed || n >= min(limit, currentLimit) {
					cancel()
					return
				}
			}
		}
	}()
	return q, cancel, nil
}
func (b *RemoteBudget) ForwardContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	return b.bound(ctx, remoteForwardContext)
}
func (b *RemoteBudget) RecoveryContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	return b.bound(ctx, remoteRecoveryContext)
}

// Only this package's live session uses this transport-only bound. It never
// implements serviceWindow or authorizes a Stop/Check/Resume/Restore action.
func (b *RemoteBudget) sessionContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	return b.bound(ctx, remoteSessionContext)
}
func (b *RemoteBudget) Close() error {
	if b == nil || b.self != b {
		return ErrRemoteBudget
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	b.cancel()
	if syscall.Close(b.dirFD) != nil {
		return ErrRemoteBudget
	}
	b.dirFD = -1
	return nil
}

// Only these concrete opaque constructors may enter the internal serviceWindow
// path. Exported callers cannot inject an interface or serialized native proof.
func StopAndDrainRemote(ctx context.Context, a *Approval, journalDir string, b *RemoteBudget) (*Lease, error) {
	if b == nil || b.self != b || b.approval != a {
		return nil, ErrRemoteBudget
	}
	return stopAndDrainWithBudget(ctx, a, journalDir, b)
}
func OpenRecoveryRemote(ctx context.Context, a *Approval, journalDir string, b *RemoteBudget) (*Lease, error) {
	if b == nil || b.self != b || b.approval != a {
		return nil, ErrRemoteBudget
	}
	return openRecoveryWithBudget(ctx, a, journalDir, b)
}
