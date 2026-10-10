package compatibilityretirementstop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/unix"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

var ErrRemoteMaterials = errors.New("retirement_remote_owned_materials_rejected")

// RootRemoteMaterials owns the exact original D service bootstrap and the
// journals produced by this process. Configuration hashes are expected inputs,
// never imported cleanup/acceptance proof. It does not own any database data,
// ordinary backup, earlier operation, SSH key or other host's temporary scope.
// Unknown files and ignored-lock concurrent root writers are not adopted.
// Complete host writer isolation remains a separate mandatory capability.
type RootRemoteMaterials struct {
	self                   *RootRemoteMaterials
	mu                     sync.Mutex
	approval               *Approval
	root                   string
	dirs                   map[string]*remoteMaterialDir
	files                  map[string]*remoteMaterialFile
	failed                 atomic.Bool
	terminal, zero, closed bool
	local, sealed          bool
	receipt                RemoteMaterialSnapshot
}
type remoteMaterialDir struct {
	file *os.File
	info os.FileInfo
}
type remoteMaterialFile struct {
	file    *os.File
	info    os.FileInfo
	hash    string
	removed bool
	closed  bool
}
type RemoteMaterialSnapshot struct {
	ScopeSHA256             string `json:"scope_sha256"`
	FilesRemoved            int    `json:"files_removed"`
	DirectoriesRemoved      int    `json:"directories_removed"`
	RemainingTemporaryFiles int    `json:"remaining_temporary_files"`
}

func (*RootRemoteMaterials) MarshalJSON() ([]byte, error) { return nil, ErrRemoteMaterials }

// Native input uses the existing protected service-session schema verbatim.
// There is intentionally no boolean, catalog, filename list or success input.
type remoteMaterialRequest struct {
	FormatVersion     int    `json:"format_version"`
	Kind              string `json:"kind"`
	ToolSourceSHA     string `json:"tool_source_sha"`
	ToolBinarySHA256  string `json:"tool_binary_sha256"`
	OriginalSourceSHA string `json:"original_source_sha"`
	OperationID       string `json:"operation_id"`
	ManifestSHA256    string `json:"manifest_sha256"`
	OriginalRunID     string `json:"original_run_id"`
	ActualRunID       string `json:"actual_run_id"`
	DescriptorSHA256  string `json:"descriptor_sha256"`
}
type remoteMaterialTemplateIntent struct {
	Kind              string `json:"kind"`
	TemplateSHA256    string `json:"template_sha256"`
	DerivedSHA256     string `json:"derived_sha256"`
	ToolSourceSHA     string `json:"tool_source_sha"`
	OriginalSourceSHA string `json:"original_source_sha"`
	OperationID       string `json:"operation_id"`
	OriginalRunID     string `json:"original_run_id"`
	ActualRunID       string `json:"actual_run_id"`
}

// OpenRootRemoteMaterials is called only by the existing validated root D
// entrypoint, before opening its fresh budget/journal. It rechecks the native
// executable, original approved descriptor/trust and actual assigned request.
// Recovery-only sessions preserve their existing assets; they cannot acquire a
// new cleanup owner. Only this exact current run's initial scope is supported.
func OpenRootRemoteMaterials(ctx context.Context, a *Approval, requestPath, requestHash, actualRun, templateHash string) (out *RootRemoteMaterials, result error) {
	if runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 || ctx == nil || ctx.Err() != nil || a.validate() != nil || a.descriptor.HostRole != "server-d" || !opID.MatchString(actualRun) || !hash64.MatchString(requestHash) {
		return nil, ErrRemoteMaterials
	}
	root := filepath.Join("/opt/qs-server/qs-worker/compatibility-retirement", a.descriptor.OperationID)
	direct := filepath.Join(root, "service-session.json")
	derived := filepath.Join(root, "service-invocations", actualRun, "service-session.json")
	if a.path != filepath.Join(root, "approved-services.json") || requestPath != direct && requestPath != derived || requestPath == direct && templateHash != "" || requestPath == derived && !hash64.MatchString(templateHash) {
		return nil, ErrRemoteMaterials
	}
	raw, e := readRootBudgetFile(requestPath)
	var r remoteMaterialRequest
	if e != nil || digest(raw) != requestHash || exactJSON(raw, &r) != nil || r.FormatVersion != 1 || r.Kind != "qs_root_service_session" || r.ToolSourceSHA != a.descriptor.ToolSourceSHA || r.OriginalSourceSHA != a.descriptor.SourceSHA || r.OperationID != a.descriptor.OperationID || r.ManifestSHA256 != a.descriptor.ManifestSHA256 || r.OriginalRunID != a.descriptor.OriginalRunID || r.ActualRunID != actualRun || r.DescriptorSHA256 != a.rawHash || !hash64.MatchString(r.ToolBinarySHA256) {
		return nil, ErrRemoteMaterials
	}
	program, e := os.Executable()
	if e != nil || program != filepath.Join(root, "qs-compatibility-retirement") {
		return nil, ErrRemoteMaterials
	}
	tool, e := hashProtectedExecutable(program)
	if e != nil || tool != r.ToolBinarySHA256 {
		return nil, ErrRemoteMaterials
	}
	if _, e = readRemoteBudgetTrust(a); e != nil {
		return nil, e
	}
	m := newRemoteMaterials(root, a)
	defer func() {
		if result != nil {
			_ = m.Close()
		}
	}()
	initial := map[string]string{"qs-compatibility-retirement": tool, "approved-services.json": a.rawHash, "budget-trust.json": a.descriptor.BudgetTrustSHA256}
	if requestPath == direct {
		initial["service-session.json"] = requestHash
	} else {
		template, e := readRootBudgetFile(filepath.Join(root, "service-session-template.json"))
		var original remoteMaterialRequest
		if e != nil || digest(template) != templateHash || exactJSON(template, &original) != nil || original.ActualRunID != "" {
			return nil, ErrRemoteMaterials
		}
		original.ActualRunID = actualRun
		expected, e := json.Marshal(original)
		if e != nil || !reflect.DeepEqual(original, r) || string(append(expected, '\n')) != string(raw) {
			return nil, ErrRemoteMaterials
		}
		intentRaw, e := readRootBudgetFile(filepath.Join(filepath.Dir(requestPath), "derived-service-session.intent.private.json"))
		var intent remoteMaterialTemplateIntent
		want := remoteMaterialTemplateIntent{"qs_native_assigned_service_run", templateHash, requestHash, r.ToolSourceSHA, r.OriginalSourceSHA, r.OperationID, r.OriginalRunID, actualRun}
		if e != nil || exactJSON(intentRaw, &intent) != nil || intent != want {
			return nil, ErrRemoteMaterials
		}
		initial["service-session-template.json"] = templateHash
		initial["service-invocations/"+actualRun+"/service-session.json"] = requestHash
		initial["service-invocations/"+actualRun+"/derived-service-session.intent.private.json"] = digest(intentRaw)
	}
	for _, rel := range []string{".", "service-journal", "remote-budget"} {
		if e = m.openDir(rel, 0); e != nil {
			return nil, e
		}
	}
	if requestPath == derived {
		for _, rel := range []string{"service-invocations", "service-invocations/" + actualRun} {
			if e = m.openDir(rel, 0); e != nil {
				return nil, e
			}
		}
	}
	for rel, hash := range initial {
		if e = m.register(rel, hash, 0); e != nil {
			return nil, e
		}
	}
	if e = m.checkNamespace(); e != nil {
		return nil, e
	}
	if len(m.files) != len(initial) {
		return nil, ErrRemoteMaterials
	}
	return m, nil
}

// A's owner uses only the existing fixed bootstrap and this key's actual seed.
// Channel hashes come from the caller's independently approved, validated SSH
// channel. Directory enumeration rejects extras; it never registers them.
func OpenRootLocalMaterials(ctx context.Context, a *Approval, k *RootBudgetKey, channelHash, identityHash, knownHostsHash, workflowScopeHash string) (_ *RootRemoteMaterials, result error) {
	if ctx == nil || ctx.Err() != nil || a.validate() != nil || a.descriptor.HostRole != "server-a" || a.materials != nil || k == nil || k.self != k || k.approval != a || !hash64.MatchString(channelHash) || !hash64.MatchString(identityHash) || !hash64.MatchString(knownHostsHash) || !hash64.MatchString(workflowScopeHash) {
		return nil, ErrRemoteMaterials
	}
	root := filepath.Dir(k.dir)
	if root != filepath.Join("/opt/qs-server/qs-apiserver/compatibility-retirement", a.descriptor.OperationID) || a.path != filepath.Join(root, "approved-services.json") {
		return nil, ErrRemoteMaterials
	}
	raw, e := readRootBudgetFile(filepath.Join(root, "service-session.json"))
	var r remoteMaterialRequest
	if e != nil || exactJSON(raw, &r) != nil || r.FormatVersion != 1 || r.Kind != "qs_root_service_session" || r.ToolSourceSHA != a.descriptor.ToolSourceSHA || r.OriginalSourceSHA != a.descriptor.SourceSHA || r.OperationID != a.descriptor.OperationID || r.ManifestSHA256 != a.descriptor.ManifestSHA256 || r.OriginalRunID != a.descriptor.OriginalRunID || !opID.MatchString(r.ActualRunID) || r.DescriptorSHA256 != a.rawHash || !hash64.MatchString(r.ToolBinarySHA256) {
		return nil, ErrRemoteMaterials
	}
	tool, e := hashProtectedExecutable(filepath.Join(root, "qs-compatibility-retirement"))
	if e != nil || tool != r.ToolBinarySHA256 {
		return nil, ErrRemoteMaterials
	}
	workflow, e := readRootBudgetFile(filepath.Join(root, "approved-workflow-scope.json"))
	binding, be := a.WindowBinding(ctx)
	if e != nil || be != nil || digest(workflow) != workflowScopeHash || fence.ValidateRunnerWorkflowScopeBinding(workflow, binding, a.descriptor.ToolSourceSHA) != nil {
		return nil, ErrRemoteMaterials
	}
	m := newRemoteMaterials(root, a)
	m.local = true
	defer func() {
		if result != nil {
			_ = m.Close()
		}
	}()
	for _, rel := range []string{".", "service-journal", "budget-issuer", "ssh"} {
		if e = m.openDir(rel, 0); e != nil {
			return nil, e
		}
	}
	initial := map[string]string{"qs-compatibility-retirement": tool, "approved-services.json": a.rawHash, "service-session.json": digest(raw), "approved-workflow-scope.json": workflowScopeHash, "ssh-channel.json": channelHash, "ssh/identity": identityHash, "ssh/known_hosts": knownHostsHash, "budget-issuer/issuer-seed": k.seedHash}
	for rel, hash := range initial {
		if e = m.register(rel, hash, 0); e != nil {
			return nil, e
		}
	}
	if e = m.checkLocked(); e != nil {
		return nil, e
	}
	a.materials = m
	return m, nil
}

func (m *RootRemoteMaterials) CheckLocalMaterials(ctx context.Context, i *BudgetIssuer, l *Lease) error {
	if m == nil || m.self != m || !m.local || m.approval == nil || ctx == nil || ctx.Err() != nil || i == nil || i.self != i || i.approval != m.approval || l == nil || l.self != l || l.approval != m.approval || l.window != i.window || m.approval.materials != m || checkWindow(ctx, m.approval, i.window) != nil {
		return ErrRemoteMaterials
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if i.closed || i.dirFD < 0 || l.closed || l.dirFD < 0 || l.failed || m.sealed {
		return ErrRemoteMaterials
	}
	return m.checkLocked()
}

// This handoff closes the two original A writers, then transfers only their
// held, rechecked directory/file identities. No saved receipt can seal them.
// The host may call it only after its original D zero and terminal observation.
func (m *RootRemoteMaterials) SealLocalMaterials(ctx context.Context, i *BudgetIssuer, l *Lease, remote *RemoteController, zero *RemoteMaterialZero, directory func(string, *os.File) error, file func(string, *os.File, string) error) error {
	if directory == nil || file == nil || m.CheckLocalMaterials(ctx, i, l) != nil || remote == nil || remote.issuer != i || zero.ValidateOriginalController(ctx, remote) != nil {
		return ErrRemoteMaterials
	}
	q, cancel, e := i.window.ForwardContext(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	return m.sealLocalRegistered(q, i, l, directory, file)
}

// As with purgeRegistered, non-root tests call only this private FD kernel.
// They cannot issue the public native A owner, original Window or D proof.
func (m *RootRemoteMaterials) sealLocalRegistered(ctx context.Context, i *BudgetIssuer, l *Lease, directory func(string, *os.File) error, file func(string, *os.File, string) error) (result error) {
	if m == nil || m.self != m || !m.local || m.closed || m.sealed || m.failed.Load() || ctx == nil || ctx.Err() != nil || i == nil || i.self != i || i.approval != m.approval || l == nil || l.self != l || l.approval != m.approval || directory == nil || file == nil {
		return ErrRemoteMaterials
	}
	defer func() {
		if result != nil {
			m.markUnknown()
		}
	}()
	if result = errors.Join(i.Close(), l.Close()); result != nil {
		return result
	}
	m.mu.Lock()
	if m.checkLocked() != nil || m.sealed {
		m.mu.Unlock()
		return ErrRemoteMaterials
	}
	dirs := make([]string, 0, len(m.dirs))
	for rel := range m.dirs {
		dirs = append(dirs, rel)
	}
	sort.Slice(dirs, func(a, b int) bool {
		return len(dirs[a]) < len(dirs[b]) || len(dirs[a]) == len(dirs[b]) && dirs[a] < dirs[b]
	})
	for _, rel := range dirs {
		if ctx.Err() != nil || directory(filepath.Join(m.root, rel), m.dirs[rel].file) != nil {
			m.mu.Unlock()
			return ErrRemoteMaterials
		}
	}
	for rel, owned := range m.files {
		if ctx.Err() != nil || file(filepath.Join(m.root, rel), owned.file, owned.hash) != nil {
			m.mu.Unlock()
			return ErrRemoteMaterials
		}
	}
	m.mu.Unlock()
	if result = m.Close(); result != nil {
		return result
	}
	m.mu.Lock()
	m.sealed = true
	m.mu.Unlock()
	return ctx.Err()
}

func (m *RootRemoteMaterials) LocalMaterialsSealed(i *BudgetIssuer, l *Lease) error {
	if m == nil || m.self != m || !m.local || m.approval == nil || i == nil || i.self != i || i.approval != m.approval || l == nil || l.self != l || l.approval != m.approval || m.approval.materials != m {
		return ErrRemoteMaterials
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !i.closed || i.dirFD != -1 || !l.closed || l.dirFD != -1 || !m.sealed || !m.closed || m.failed.Load() {
		return ErrRemoteMaterials
	}
	return nil
}
func newRemoteMaterials(root string, a *Approval) *RootRemoteMaterials {
	m := &RootRemoteMaterials{approval: a, root: root, dirs: map[string]*remoteMaterialDir{}, files: map[string]*remoteMaterialFile{}}
	m.self = m
	return m
}
func materialSameInfo(a, b os.FileInfo, regular bool, uid uint32) bool {
	if a == nil || b == nil || !os.SameFile(a, b) || a.Mode() != b.Mode() || a.Mode().Perm()&0022 != 0 || regular && (!a.Mode().IsRegular() || a.Size() != b.Size() || !a.ModTime().Equal(b.ModTime())) || !regular && !a.IsDir() {
		return false
	}
	x, xok := a.Sys().(*syscall.Stat_t)
	y, yok := b.Sys().(*syscall.Stat_t)
	return xok && yok && x.Uid == uid && y.Uid == uid && x.Gid == y.Gid && (!regular || x.Nlink == y.Nlink && x.Nlink == 1)
}
func (m *RootRemoteMaterials) openDir(rel string, uid uint32) error {
	if rel != "." && (filepath.Clean(rel) != rel || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..")) {
		return ErrRemoteMaterials
	}
	var fd int
	var e error
	if rel == "." {
		fd, e = unix.Open(m.root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	} else {
		parent := m.dirs[filepath.Dir(rel)]
		if parent == nil {
			return ErrRemoteMaterials
		}
		fd, e = unix.Openat(int(parent.file.Fd()), filepath.Base(rel), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	}
	if e != nil {
		return ErrRemoteMaterials
	}
	f := os.NewFile(uintptr(fd), "owned-remote-directory")
	info, e := f.Stat()
	if e != nil || !materialSameInfo(info, info, false, uid) || info.Mode().Perm() != 0700 {
		_ = f.Close()
		return ErrRemoteMaterials
	}
	m.dirs[rel] = &remoteMaterialDir{f, info}
	return nil
}
func materialFileHash(f *os.File) (string, error) {
	if _, e := f.Seek(0, io.SeekStart); e != nil {
		return "", ErrRemoteMaterials
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, (256<<20)+1))
	if e != nil || n > 256<<20 {
		return "", ErrRemoteMaterials
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (m *RootRemoteMaterials) register(rel, expected string, uid uint32) error {
	if filepath.IsAbs(rel) || filepath.Clean(rel) != rel || strings.HasPrefix(rel, "..") || !hash64.MatchString(expected) || m.files[rel] != nil {
		return ErrRemoteMaterials
	}
	parent := m.dirs[filepath.Dir(rel)]
	if parent == nil {
		return ErrRemoteMaterials
	}
	fd, e := unix.Openat(int(parent.file.Fd()), filepath.Base(rel), unix.O_RDONLY|unix.O_NOFOLLOW, 0)
	if e != nil {
		return ErrRemoteMaterials
	}
	f := os.NewFile(uintptr(fd), "owned-remote-material")
	info, e := f.Stat()
	h, he := materialFileHash(f)
	after, ae := f.Stat()
	if e != nil || he != nil || ae != nil || h != expected || !materialSameInfo(info, after, true, uid) {
		_ = f.Close()
		return ErrRemoteMaterials
	}
	m.files[rel] = &remoteMaterialFile{file: f, info: after, hash: h}
	return nil
}

// Dynamic registration is called only after this process's actual O_EXCL write,
// file Sync, close and parent Sync. Failure is sticky; no future purge can adopt
// or clean that unknown result.
func (m *RootRemoteMaterials) registerWritten(rel string, raw []byte) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	localBudget := m.local && (rel == "budget-issuer/issuer-window.json" || strings.HasPrefix(rel, "budget-issuer/nonce-") && hash64.MatchString(strings.TrimPrefix(rel, "budget-issuer/nonce-")))
	if m.self != m || m.closed || m.terminal || m.sealed || m.failed.Load() || (!strings.HasPrefix(rel, "service-journal/") && (!strings.HasPrefix(rel, "remote-budget/") || m.local) && !localBudget) {
		m.failed.Store(true)
		return ErrRemoteMaterials
	}
	if e := m.register(rel, digest(raw), 0); e != nil {
		m.failed.Store(true)
		return e
	}
	return nil
}
func (m *RootRemoteMaterials) markUnknown() {
	if m != nil {
		m.failed.Store(true)
	}
}
func (m *RootRemoteMaterials) checkFile(rel string, v *remoteMaterialFile, uid uint32) error {
	if v.removed {
		return ErrRemoteMaterials
	}
	before, e := v.file.Stat()
	h, he := materialFileHash(v.file)
	after, ae := v.file.Stat()
	parent := m.dirs[filepath.Dir(rel)]
	if parent == nil {
		return ErrRemoteMaterials
	}
	fd, pe := unix.Openat(int(parent.file.Fd()), filepath.Base(rel), unix.O_RDONLY|unix.O_NOFOLLOW, 0)
	if pe != nil {
		return ErrRemoteMaterials
	}
	p := os.NewFile(uintptr(fd), "owned-remote-current-path")
	current, ce := p.Stat()
	closeErr := p.Close()
	if e != nil || he != nil || ae != nil || ce != nil || closeErr != nil || h != v.hash || !materialSameInfo(v.info, before, true, uid) || !materialSameInfo(before, after, true, uid) || !materialSameInfo(after, current, true, uid) {
		return ErrRemoteMaterials
	}
	return nil
}
func directoryNames(f *os.File) ([]string, error) {
	fd, e := unix.Openat(int(f.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if e != nil {
		return nil, ErrRemoteMaterials
	}
	d := os.NewFile(uintptr(fd), "owned-remote-directory-eof")
	names, e := d.Readdirnames(-1)
	ce := d.Close()
	if e != nil || ce != nil {
		return nil, ErrRemoteMaterials
	}
	sort.Strings(names)
	return names, nil
}
func (m *RootRemoteMaterials) checkNamespace() error {
	for rel, d := range m.dirs {
		held, e := d.file.Stat()
		current, ce := os.Lstat(filepath.Join(m.root, rel))
		if e != nil || ce != nil || !materialSameInfo(d.info, held, false, uint32(os.Geteuid())) || !materialSameInfo(held, current, false, uint32(os.Geteuid())) {
			return ErrRemoteMaterials
		}
		actual, e := directoryNames(d.file)
		if e != nil {
			return e
		}
		expected := []string{}
		for name, v := range m.files {
			if !v.removed && filepath.Dir(name) == rel {
				expected = append(expected, filepath.Base(name))
			}
		}
		for name := range m.dirs {
			if name != "." && filepath.Dir(name) == rel {
				expected = append(expected, filepath.Base(name))
			}
		}
		sort.Strings(expected)
		if !reflect.DeepEqual(actual, expected) {
			return ErrRemoteMaterials
		}
	}
	return nil
}
func (m *RootRemoteMaterials) checkLocked() error {
	if m == nil || m.self != m || m.closed || m.failed.Load() || m.zero {
		return ErrRemoteMaterials
	}
	for rel, f := range m.files {
		if e := m.checkFile(rel, f, uint32(os.Geteuid())); e != nil {
			return e
		}
	}
	return m.checkNamespace()
}
func (m *RootRemoteMaterials) scopeHash() string {
	type entry struct{ Path, Hash string }
	entries := []entry{}
	for rel, f := range m.files {
		entries = append(entries, entry{rel, f.hash})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return runtimeDigest(entries)
}

// purge deletes only held, rechecked original registered names. It never uses
// RemoveAll or accepts a saved receipt. The intent and final receipt contain no
// bodies and are the only retained operation evidence. Unknown partial effects
// retain the exact intent and remaining files; they cannot be replayed.
func (m *RootRemoteMaterials) purge(ctx context.Context, b *RemoteBudget, l *Lease) (RemoteMaterialSnapshot, error) {
	if m == nil || m.self != m || m.local || b == nil || b.approval != m.approval || l == nil || l.approval != m.approval || l.runtimeObservation == nil {
		return RemoteMaterialSnapshot{}, ErrRemoteMaterials
	}
	if _, e := l.runtimeObservation.Snapshot(); e != nil {
		return RemoteMaterialSnapshot{}, e
	}
	q, c, e := b.ForwardContext(ctx)
	if e != nil {
		return RemoteMaterialSnapshot{}, e
	}
	defer c()
	if e := b.beginOwnedPurge(q, m); e != nil {
		return RemoteMaterialSnapshot{}, e
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.terminal || m.checkLocked() != nil {
		m.failed.Store(true)
		return RemoteMaterialSnapshot{}, ErrRemoteMaterials
	}
	return m.purgeRegistered(q, 0, b.state.StartSHA256, b.state.Counter)
}

// Private filesystem kernel, called in production only after beginOwnedPurge.
// Non-root tests exercise actual inode/FD/delete contracts in their temporary
// fixture; they never construct a public root owner or original budget proof.
func (m *RootRemoteMaterials) purgeRegistered(ctx context.Context, uid uint32, start string, counter uint64) (RemoteMaterialSnapshot, error) {
	if m == nil || m.self != m || m.closed || m.terminal || m.failed.Load() || ctx == nil || ctx.Err() != nil || m.dirs["."] == nil {
		return RemoteMaterialSnapshot{}, ErrRemoteMaterials
	}
	m.terminal = true
	root := m.dirs["."]
	scope := m.scopeHash()
	intent, e := json.Marshal(struct {
		Kind, SourceSHA, OperationID, OriginalRunID, WindowStartSHA256, ScopeSHA256 string
		Counter                                                                     uint64
	}{"qs_remote_owned_purge_intent", m.approval.descriptor.ToolSourceSHA, m.approval.descriptor.OperationID, m.approval.descriptor.OriginalRunID, start, scope, counter})
	if e != nil || writeRootExclusive(int(root.file.Fd()), "remote-owned-purge.intent.json", intent) != nil {
		m.failed.Store(true)
		return RemoteMaterialSnapshot{}, ErrRemoteMaterials
	}
	names := make([]string, 0, len(m.files))
	for name := range m.files {
		names = append(names, name)
	}
	sort.Strings(names)
	receipt := RemoteMaterialSnapshot{ScopeSHA256: scope}
	for _, rel := range names {
		v := m.files[rel]
		if ctx.Err() != nil || m.checkFile(rel, v, uid) != nil {
			m.failed.Store(true)
			return receipt, ErrRemoteMaterials
		}
		parent := m.dirs[filepath.Dir(rel)]
		if unix.Unlinkat(int(parent.file.Fd()), filepath.Base(rel), 0) != nil || parent.file.Sync() != nil {
			m.failed.Store(true)
			return receipt, ErrRemoteMaterials
		}
		v.removed = true
		var st unix.Stat_t
		if e := unix.Fstatat(int(parent.file.Fd()), filepath.Base(rel), &st, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(e, unix.ENOENT) {
			m.failed.Store(true)
			return receipt, ErrRemoteMaterials
		}
		if e := v.file.Close(); e != nil {
			m.failed.Store(true)
			return receipt, ErrRemoteMaterials
		}
		v.closed = true
		receipt.FilesRemoved++
	}
	dirs := []string{}
	for rel := range m.dirs {
		if rel != "." {
			dirs = append(dirs, rel)
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j]) || len(dirs[i]) == len(dirs[j]) && dirs[i] < dirs[j]
	})
	for _, rel := range dirs {
		d := m.dirs[rel]
		names, e := directoryNames(d.file)
		if e != nil || len(names) != 0 || ctx.Err() != nil {
			m.failed.Store(true)
			return receipt, ErrRemoteMaterials
		}
		parent := m.dirs[filepath.Dir(rel)]
		current, e := os.Lstat(filepath.Join(m.root, rel))
		held, he := d.file.Stat()
		if e != nil || he != nil || !materialSameInfo(d.info, held, false, uid) || !materialSameInfo(held, current, false, uid) || unix.Unlinkat(int(parent.file.Fd()), filepath.Base(rel), unix.AT_REMOVEDIR) != nil || parent.file.Sync() != nil {
			m.failed.Store(true)
			return receipt, ErrRemoteMaterials
		}
		if e = d.file.Close(); e != nil {
			m.failed.Store(true)
			return receipt, ErrRemoteMaterials
		}
		delete(m.dirs, rel)
		receipt.DirectoriesRemoved++
	}
	remain, e := directoryNames(root.file)
	if e != nil || !reflect.DeepEqual(remain, []string{"remote-owned-purge.intent.json"}) {
		m.failed.Store(true)
		return receipt, ErrRemoteMaterials
	}
	raw, e := json.Marshal(receipt)
	if e != nil || writeRootExclusive(int(root.file.Fd()), "remote-owned-purge.receipt.json", raw) != nil {
		m.failed.Store(true)
		return receipt, ErrRemoteMaterials
	}
	remain, e = directoryNames(root.file)
	if e != nil || !reflect.DeepEqual(remain, []string{"remote-owned-purge.intent.json", "remote-owned-purge.receipt.json"}) || ctx.Err() != nil {
		m.failed.Store(true)
		return receipt, ErrRemoteMaterials
	}
	m.zero = true
	m.receipt = receipt
	return receipt, nil
}
func (m *RootRemoteMaterials) Close() error {
	if m == nil || m.self != m {
		return ErrRemoteMaterials
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	var result error
	for _, f := range m.files {
		if !f.closed {
			result = errors.Join(result, f.file.Close())
		}
	}
	for _, d := range m.dirs {
		result = errors.Join(result, d.file.Close())
	}
	if result != nil {
		return ErrRemoteMaterials
	}
	return nil
}

// The terminal budget is constructed only after the original grant, native
// ownership and original clock were rechecked together. It cannot be imported.
type remoteOwnedPurgeBudget struct {
	owner      *RootRemoteMaterials
	state      remoteBudgetState
	recordHash string
}

func (b *RemoteBudget) beginOwnedPurge(ctx context.Context, m *RootRemoteMaterials) error {
	if b == nil || b.self != b || m == nil || m.self != m || m.approval != b.approval {
		return ErrRemoteMaterials
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	_, now, e := b.clockLocked(ctx)
	if e != nil || b.pending != nil || b.terminalBudget != nil || b.state.Counter == 0 || b.state.RecoverySHA256 != "" || now >= b.state.ForwardDeadline {
		return ErrRemoteMaterials
	}
	// The immediately preceding real signed grant must authorize this exact
	// terminal action. Reading a old saved ordinary grant cannot authorize purge.
	raw, e := readRootBudgetFile(filepath.Join(b.dir, fmt.Sprintf("grant-%04d.json", b.state.Counter)))
	var rec remoteBudgetRecord
	if e != nil || digest(raw) != b.recordHash || exactJSON(raw, &rec) != nil || rec.Grant.Payload.Action != "purge_materials" {
		return ErrRemoteMaterials
	}
	m.mu.Lock()
	e = m.checkLocked()
	m.mu.Unlock()
	if e != nil {
		m.markUnknown()
		return e
	}
	b.terminalBudget = &remoteOwnedPurgeBudget{m, b.state, b.recordHash}
	return nil
}
