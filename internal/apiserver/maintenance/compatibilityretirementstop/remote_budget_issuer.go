package compatibilityretirementstop

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

const budgetProtocol = "qs-live-original-window-budget/v1"

var ErrRemoteBudget = errors.New("retirement_live_remote_budget_rejected")

// RootBudgetKey is one batch's temporary issuer seed. It grants no service or
// database permission. Its directory is fixed, root protected and never reset.
// The host must remove this temporary seed after acceptance/purge; no permanent
// account, shared key or GitHub secret is introduced by this package.
type RootBudgetKey struct {
	self     *RootBudgetKey
	approval *Approval
	dir      string
	seedHash string
	public   ed25519.PublicKey
}

func (*RootBudgetKey) MarshalJSON() ([]byte, error) { return nil, ErrRemoteBudget }
func budgetIssuerDirectory(a *Approval) string {
	return filepath.Join("/opt/qs-server/qs-apiserver/compatibility-retirement", a.descriptor.OperationID, "budget-issuer")
}
func budgetRemoteDirectory(a *Approval) string {
	return filepath.Join("/opt/qs-server/qs-worker/compatibility-retirement", a.descriptor.OperationID, "remote-budget")
}
func writeRootExclusive(fd int, name string, b []byte) error {
	n, e := unix.Openat(fd, name, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return ErrRemoteBudget
	}
	f := os.NewFile(uintptr(n), "batch-budget-record")
	wrote, we := f.Write(b)
	se := f.Sync()
	ce := f.Close()
	if we != nil || wrote != len(b) || se != nil || ce != nil || syscall.Fsync(fd) != nil {
		return ErrRemoteBudget
	}
	return nil
}
func readRootBudgetFile(path string) ([]byte, error) {
	info, e := os.Lstat(path)
	if e != nil || info.Mode().Perm() != 0600 {
		return nil, ErrRemoteBudget
	}
	b, e := readProtected(path)
	if e != nil {
		return nil, ErrRemoteBudget
	}
	return b, nil
}

// CreateRootBudgetKey is called by the already trusted fixed Server A management
// entrypoint before the window. The terminal root 0700 directory must already
// exist; this never provisions root access or accepts a supplied private seed.
func CreateRootBudgetKey(ctx context.Context, a *Approval) (out *RootBudgetKey, result error) {
	if ctx == nil || ctx.Err() != nil || a.validate() != nil || a.descriptor.HostRole != "server-a" {
		return nil, ErrRemoteBudget
	}
	dir := budgetIssuerDirectory(a)
	fd, e := openJournal(dir)
	if e != nil {
		return nil, e
	}
	defer func() {
		if err := syscall.Close(fd); result == nil && err != nil {
			out = nil
			result = ErrRemoteBudget
		}
	}()
	seed := make([]byte, ed25519.SeedSize)
	if _, e = rand.Read(seed); e != nil {
		return nil, ErrRemoteBudget
	}
	if e = writeRootExclusive(fd, "issuer-seed", seed); e != nil {
		return nil, e
	}
	return OpenRootBudgetKey(ctx, a)
}
func OpenRootBudgetKey(ctx context.Context, a *Approval) (*RootBudgetKey, error) {
	if ctx == nil || ctx.Err() != nil || a.validate() != nil || a.descriptor.HostRole != "server-a" {
		return nil, ErrRemoteBudget
	}
	dir := budgetIssuerDirectory(a)
	seed, e := readRootBudgetFile(filepath.Join(dir, "issuer-seed"))
	if e != nil || len(seed) != ed25519.SeedSize {
		return nil, ErrRemoteBudget
	}
	private := ed25519.NewKeyFromSeed(seed)
	k := &RootBudgetKey{approval: a, dir: dir, seedHash: digest(seed), public: append(ed25519.PublicKey(nil), private.Public().(ed25519.PublicKey)...)}
	k.self = k
	for i := range private {
		private[i] = 0
	}
	for i := range seed {
		seed[i] = 0
	}
	return k, nil
}
func (k *RootBudgetKey) PublicKey() (string, error) {
	if k == nil || k.self != k || k.approval.validate() != nil {
		return "", ErrRemoteBudget
	}
	return hex.EncodeToString(k.public), nil
}
func (k *RootBudgetKey) sign(raw []byte) ([]byte, error) {
	if k == nil || k.self != k || k.approval.validate() != nil {
		return nil, ErrRemoteBudget
	}
	seed, e := readRootBudgetFile(filepath.Join(k.dir, "issuer-seed"))
	if e != nil || len(seed) != ed25519.SeedSize || digest(seed) != k.seedHash {
		return nil, ErrRemoteBudget
	}
	private := ed25519.NewKeyFromSeed(seed)
	sig := ed25519.Sign(private, raw)
	for i := range private {
		private[i] = 0
	}
	for i := range seed {
		seed[i] = 0
	}
	return sig, nil
}

type issuerWindowRecord struct {
	Protocol               string              `json:"protocol"`
	Binding                fence.WindowBinding `json:"binding"`
	StartSHA256            string              `json:"start_sha256"`
	ToolSourceSHA          string              `json:"tool_source_sha"`
	ApprovalSHA256         string              `json:"approval_sha256"`
	RemoteDescriptorSHA256 string              `json:"remote_descriptor_sha256"`
	PublicKey              string              `json:"public_key"`
}
type BudgetIssuer struct {
	self       *BudgetIssuer
	mu         sync.Mutex
	approval   *Approval
	key        *RootBudgetKey
	window     *fence.MaintenanceWindow
	record     issuerWindowRecord
	recordHash string
	dirFD      int
	closed     bool
}

func (*BudgetIssuer) MarshalJSON() ([]byte, error) { return nil, ErrRemoteBudget }

// OpenBudgetIssuer borrows the ACTUAL same-process original opaque Window.
// A first native start is O_EXCL committed once. Reopening a new/reset Window,
// a different B tool/approval/remote descriptor/key, or changing the record fails.
func OpenBudgetIssuer(ctx context.Context, a *Approval, k *RootBudgetKey, w *fence.MaintenanceWindow) (*BudgetIssuer, error) {
	if ctx == nil || ctx.Err() != nil || a.validate() != nil || a.descriptor.HostRole != "server-a" || !hash64.MatchString(a.descriptor.RemoteDescriptorSHA256) || k == nil || k.self != k || k.approval != a || checkWindow(ctx, a, w) != nil {
		return nil, ErrRemoteBudget
	}
	fd, e := openJournal(k.dir)
	if e != nil {
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			_ = syscall.Close(fd)
		}
	}()
	d, e := w.Diagnostic(ctx)
	if e != nil {
		return nil, e
	}
	public, e := k.PublicKey()
	if e != nil {
		return nil, e
	}
	record := issuerWindowRecord{budgetProtocol, d.Binding, d.StartSHA256, a.descriptor.ToolSourceSHA, a.rawHash, a.descriptor.RemoteDescriptorSHA256, public}
	raw, e := json.Marshal(record)
	if e != nil {
		return nil, ErrRemoteBudget
	}
	path := filepath.Join(k.dir, "issuer-window.json")
	existing, e := readRootBudgetFile(path)
	if _, se := os.Lstat(path); os.IsNotExist(se) {
		if e = writeRootExclusive(fd, "issuer-window.json", raw); e != nil {
			return nil, e
		}
		existing, e = readRootBudgetFile(path)
	}
	if e != nil || !bytes.Equal(existing, raw) {
		return nil, ErrRemoteBudget
	}
	out := &BudgetIssuer{approval: a, key: k, window: w, record: record, recordHash: digest(raw), dirFD: fd}
	out.self = out
	ok = true
	return out, nil
}

type RemoteBudgetChallenge struct {
	Protocol               string `json:"protocol"`
	Counter                uint64 `json:"counter"`
	Nonce                  string `json:"nonce"`
	Action                 string `json:"action"`
	RemoteDescriptorSHA256 string `json:"remote_descriptor_sha256"`
}
type remoteBudgetPayload struct {
	ToolSourceSHA          string              `json:"tool_source_sha"`
	Protocol               string              `json:"protocol"`
	Counter                uint64              `json:"counter"`
	Nonce                  string              `json:"nonce"`
	Action                 string              `json:"action"`
	RemoteDescriptorSHA256 string              `json:"remote_descriptor_sha256"`
	Binding                fence.WindowBinding `json:"binding"`
	StartSHA256            string              `json:"start_sha256"`
	RecoverySHA256         string              `json:"recovery_sha256"`
	RemainingMilliseconds  int64               `json:"remaining_milliseconds"`
	ForwardMilliseconds    int64               `json:"forward_milliseconds"`
}
type signedRemoteBudget struct {
	Payload   remoteBudgetPayload `json:"payload"`
	Signature string              `json:"signature"`
}

func recoveryAction(s string) bool { return s == "restore" || s == "restore_dependents" }
func serviceAction(s string) bool {
	switch s {
	case "bind", "stop", "check", "resume_dependents", "restore", "restore_dependents", "controlled_resume", "check_running", "observe_loaded_mq", "purge_materials":
		return true
	}
	return false
}

func controlledAction(s string) bool {
	return s == "controlled_resume" || s == "check_running" || s == "observe_loaded_mq" || s == "purge_materials"
}

// IssueFreshBudget handles a fresh live challenge from the pinned D session.
// The request contributes no budget; every budget field is read from the actual
// original Window here. The host must enter actual RecoveryContext itself before
// requesting recovery; an untrusted action never switches the native A phase.
func (i *BudgetIssuer) IssueFreshBudget(ctx context.Context, rawChallenge []byte) ([]byte, error) {
	if i == nil || i.self != i {
		return nil, ErrRemoteBudget
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed || ctx == nil || ctx.Err() != nil || i.approval.validate() != nil || checkWindow(ctx, i.approval, i.window) != nil {
		return nil, ErrRemoteBudget
	}
	saved, e := readRootBudgetFile(filepath.Join(i.key.dir, "issuer-window.json"))
	if e != nil || digest(saved) != i.recordHash {
		return nil, ErrRemoteBudget
	}
	var c RemoteBudgetChallenge
	if len(rawChallenge) > 1024 || exactJSON(rawChallenge, &c) != nil || c.Protocol != budgetProtocol || c.Counter == 0 || c.Counter > 4096 || !hash64.MatchString(c.Nonce) || !serviceAction(c.Action) || c.RemoteDescriptorSHA256 != i.record.RemoteDescriptorSHA256 {
		return nil, ErrRemoteBudget
	}
	d, e := i.window.Diagnostic(ctx)
	if e != nil || d.Binding != i.record.Binding || d.StartSHA256 != i.record.StartSHA256 || d.RemainingMilliseconds <= 0 || recoveryAction(c.Action) != (d.RecoverySHA256 != "") {
		return nil, ErrRemoteBudget
	}
	forward := max(int64(0), d.RemainingMilliseconds-int64((10*time.Minute)/time.Millisecond))
	if d.RecoverySHA256 != "" {
		forward = 0
	}
	p := remoteBudgetPayload{i.approval.descriptor.ToolSourceSHA, budgetProtocol, c.Counter, c.Nonce, c.Action, c.RemoteDescriptorSHA256, d.Binding, d.StartSHA256, d.RecoverySHA256, d.RemainingMilliseconds, forward}
	payload, e := json.Marshal(p)
	if e != nil {
		return nil, ErrRemoteBudget
	}
	// A also consumes this nonce durably. A lost response requires a NEW D
	// challenge; neither process may replay a signed or imported old success.
	if e = writeRootExclusive(i.dirFD, "nonce-"+c.Nonce, []byte(digest(rawChallenge))); e != nil {
		return nil, e
	}
	sig, e := i.key.sign(payload)
	if e != nil {
		return nil, e
	}
	after, ae := i.window.Diagnostic(ctx)
	if ae != nil || after.Binding != d.Binding || after.StartSHA256 != d.StartSHA256 || after.RecoverySHA256 != d.RecoverySHA256 || after.RemainingMilliseconds <= 0 || ctx.Err() != nil {
		return nil, ErrRemoteBudget
	}
	return json.Marshal(signedRemoteBudget{p, hex.EncodeToString(sig)})
}
func (i *BudgetIssuer) Close() error {
	if i == nil || i.self != i {
		return ErrRemoteBudget
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil
	}
	i.closed = true
	if syscall.Close(i.dirFD) != nil {
		return ErrRemoteBudget
	}
	i.dirFD = -1
	return nil
}
