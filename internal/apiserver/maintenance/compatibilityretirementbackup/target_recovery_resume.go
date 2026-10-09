package compatibilityretirementbackup

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	"golang.org/x/sys/unix"
)

const targetJournalFileLimit = 64 << 10
const targetJournalEntryLimit = 37 // original closed entries plus two exact B-migration files

// This is reconciliation input, not execution permission. A new run retains the
// original binding and the original window start; it cannot reset either.
type TargetRecoveryResumeRequest struct {
	Original          TargetRecoveryRequest
	CurrentRunID      string
	JournalSHA256     string
	WindowStartSHA256 string
}

type TargetRecoveryJournalSummary struct {
	JournalSHA256           string
	RequestSHA256           string
	WindowStartSHA256       string
	Files                   int
	ProductionFenceVerified bool
	MutationAllowed         bool
}

type targetJournalEntryBinding struct {
	Name     string
	SHA256   string
	Bytes    int64
	Device   uint64
	Inode    uint64
	UID      uint32
	GID      uint32
	Mode     uint32
	Links    uint64
	Modified int64
	Changed  int64
}

type targetJournalDirectoryBinding struct {
	Device uint64
	Inode  uint64
	UID    uint32
	GID    uint32
	Mode   uint32
	Links  uint64
}

// A new restore records its real run and dedicated connection. It never
// changes the original DROP binding or claims that an older client restored.
type targetResumeBindingRecord struct {
	OriginalRequestSHA256 string                `json:"original_request_sha256"`
	WindowStartSHA256     string                `json:"window_start_sha256"`
	Request               TargetRecoveryRequest `json:"request"`
	SQLConnectionID       string                `json:"sql_connection_id"`
}

type targetJournalSnapshot struct {
	request     TargetRecoveryRequest
	windowStart string
	directory   targetJournalDirectoryBinding
	files       map[string][]byte
	entries     []targetJournalEntryBinding
	hash        string
}

type targetJournalDirectory struct {
	fd     int
	parent int
	name   string
	stamp  unix.Stat_t
}

func closeTargetJournalDirectories(dirs []targetJournalDirectory) error {
	var result error
	for i := len(dirs) - 1; i >= 0; i-- {
		if unix.Close(dirs[i].fd) != nil {
			result = ErrRecoveryJournal
		}
	}
	return result
}

func targetDirectoryIdentity(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Uid == b.Uid &&
		a.Gid == b.Gid && a.Mode == b.Mode && a.Nlink == b.Nlink
}

func openTargetJournalDirectories(path string) (dirs []targetJournalDirectory, result error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return nil, ErrRecoveryJournal
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrRecoveryJournal
	}
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil {
		if unix.Close(fd) != nil {
			return nil, ErrRecoveryJournal
		}
		return nil, ErrRecoveryJournal
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != 0 || st.Mode&0022 != 0 {
		if unix.Close(fd) != nil {
			return nil, ErrRecoveryJournal
		}
		return nil, ErrRecoveryJournal
	}
	dirs = append(dirs, targetJournalDirectory{fd: fd, parent: -1, name: "/", stamp: st})
	var opened []targetJournalDirectory
	defer func() {
		if result != nil {
			if closeTargetJournalDirectories(opened) != nil {
				result = ErrRecoveryJournal
			}
			dirs = nil
		}
	}()
	opened = dirs
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, ErrRecoveryJournal
		}
		parent := dirs[len(dirs)-1].fd
		fd, e = unix.Openat(parent, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if e != nil {
			return nil, ErrRecoveryJournal
		}
		if unix.Fstat(fd, &st) != nil {
			if unix.Close(fd) != nil {
				return nil, ErrRecoveryJournal
			}
			return nil, ErrRecoveryJournal
		}
		dirs = append(dirs, targetJournalDirectory{fd: fd, parent: parent, name: part, stamp: st})
		opened = dirs
		if st.Mode&unix.S_IFMT != unix.S_IFDIR || (st.Uid != 0 && st.Uid != uint32(os.Geteuid())) {
			return nil, ErrRecoveryJournal
		}
		// Existing task-private roots may be below root-owned sticky /tmp. This
		// reader grants no mutation permission or protected-executor installation.
		if st.Mode&0022 != 0 && (st.Mode&unix.S_ISVTX == 0 || st.Uid != 0) {
			return nil, ErrRecoveryJournal
		}
		if i == len(parts)-1 && (st.Uid != uint32(os.Geteuid()) || st.Mode&07777 != 0700) {
			return nil, ErrRecoveryJournal
		}
	}
	if verifyTargetJournalDirectories(dirs) != nil {
		return nil, ErrRecoveryJournal
	}
	return dirs, nil
}

func verifyTargetJournalDirectories(dirs []targetJournalDirectory) error {
	for _, d := range dirs {
		var fd, named unix.Stat_t
		if unix.Fstat(d.fd, &fd) != nil || !targetDirectoryIdentity(d.stamp, fd) {
			return ErrRecoveryJournal
		}
		if d.parent >= 0 {
			if unix.Fstatat(d.parent, d.name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || !targetDirectoryIdentity(fd, named) {
				return ErrRecoveryJournal
			}
		}
	}
	return nil
}

func listTargetJournalFiles(fd int) (names []string, result error) {
	copyFD, e := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrRecoveryJournal
	}
	f := os.NewFile(uintptr(copyFD), "target recovery private directory")
	if f == nil {
		if unix.Close(copyFD) != nil {
			return nil, ErrRecoveryJournal
		}
		return nil, ErrRecoveryJournal
	}
	defer func() {
		if f.Close() != nil {
			result = ErrRecoveryJournal
		}
	}()
	entries, e := f.ReadDir(targetJournalEntryLimit + 1)
	if e != nil && e != io.EOF {
		return nil, ErrRecoveryJournal
	}
	if len(entries) == 0 || len(entries) > targetJournalEntryLimit {
		return nil, ErrRecoveryJournal
	}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func targetJournalNamesEqual(a, b []string) bool { return reflect.DeepEqual(a, b) }

func readTargetJournalEntry(ctx context.Context, fd int, name string) (raw []byte, binding targetJournalEntryBinding, result error) {
	if ctx == nil || ctx.Err() != nil || filepath.Base(name) != name || name == "" {
		return nil, binding, ErrRecoveryJournal
	}
	var named unix.Stat_t
	if unix.Fstatat(fd, name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil {
		return nil, binding, ErrRecoveryJournal
	}
	fileFD, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, binding, ErrRecoveryJournal
	}
	f := os.NewFile(uintptr(fileFD), "target recovery private record")
	if f == nil {
		if unix.Close(fileFD) != nil {
			return nil, binding, ErrRecoveryJournal
		}
		return nil, binding, ErrRecoveryJournal
	}
	defer func() {
		if f.Close() != nil {
			result = ErrRecoveryJournal
		}
	}()
	before, e := f.Stat()
	bs, be := targetStamp(before)
	if e != nil || be != nil || !bs.mode.IsRegular() || bs.mode.Perm() != 0600 || named.Mode&07777 != 0600 || bs.owner != uint32(os.Geteuid()) || bs.links != 1 || bs.size < 1 || bs.size > targetJournalFileLimit || uint64(named.Dev) != bs.device || uint64(named.Ino) != bs.inode {
		return nil, binding, ErrRecoveryJournal
	}
	raw, e = io.ReadAll(io.LimitReader(f, targetJournalFileLimit+1))
	after, se := f.Stat()
	as, ae := targetStamp(after)
	var end unix.Stat_t
	ne := unix.Fstatat(fd, name, &end, unix.AT_SYMLINK_NOFOLLOW)
	if e != nil || se != nil || ae != nil || ne != nil || bs != as || int64(len(raw)) != bs.size || uint64(end.Dev) != bs.device || uint64(end.Ino) != bs.inode || end.Mode != named.Mode || end.Uid != named.Uid || end.Gid != named.Gid || end.Nlink != named.Nlink || ctx.Err() != nil {
		return nil, binding, ErrRecoveryJournal
	}
	binding = targetJournalEntryBinding{Name: name, SHA256: sha(raw), Bytes: bs.size, Device: bs.device, Inode: bs.inode, UID: named.Uid, GID: named.Gid, Mode: uint32(bs.mode.Perm()), Links: bs.links, Modified: bs.modified, Changed: bs.changed}
	return raw, binding, nil
}

func targetJournalNameSupported(name string) bool {
	if name == "target-recovery-binding.json" || name == "target-recovery-b-migration-intent.json" || name == "target-recovery-b-migration-result.json" {
		return true
	}
	for i := 0; i < 4; i++ {
		for _, phase := range []string{"drop-intent", "drop-result", "create-intent", "create-result", "load-intent", "load-result", "verify-result", "resume-binding"} {
			if name == "target-recovery-"+string(rune('0'+i))+"-"+phase+".json" {
				return true
			}
		}
	}
	return name == "target-recovery-3-index-intent.json" || name == "target-recovery-3-index-result.json"
}

func inspectTargetJournal(ctx context.Context, path string, request TargetRecoveryRequest, windowStart string) (snapshot *targetJournalSnapshot, result error) {
	return inspectTargetJournalKind(ctx, path, request, windowStart, targetBJournalComplete)
}

func inspectTargetJournalKind(ctx context.Context, path string, request TargetRecoveryRequest, windowStart string, kind targetBJournalKind) (snapshot *targetJournalSnapshot, result error) {
	if !targetBJournalKindValid(kind) {
		return nil, ErrRecoveryBinding
	}
	if ctx == nil || ctx.Err() != nil || !hashPattern.MatchString(windowStart) {
		return nil, ErrRecoveryJournal
	}
	dirs, e := openTargetJournalDirectories(path)
	if e != nil {
		return nil, e
	}
	defer func() {
		if closeTargetJournalDirectories(dirs) != nil {
			result = ErrRecoveryJournal
		}
	}()
	fd := dirs[len(dirs)-1].fd
	names, e := listTargetJournalFiles(fd)
	if e != nil {
		return nil, e
	}
	leaf := dirs[len(dirs)-1].stamp
	s := &targetJournalSnapshot{files: make(map[string][]byte, len(names)), windowStart: windowStart, directory: targetJournalDirectoryBinding{uint64(leaf.Dev), uint64(leaf.Ino), leaf.Uid, leaf.Gid, uint32(leaf.Mode), uint64(leaf.Nlink)}}
	for _, name := range names {
		if !targetJournalNameSupported(name) {
			return nil, ErrRecoveryJournal
		}
		raw, binding, e := readTargetJournalEntry(ctx, fd, name)
		if e != nil {
			return nil, e
		}
		s.files[name] = raw
		s.entries = append(s.entries, binding)
	}
	raw, ok := s.files["target-recovery-binding.json"]
	if !ok || exactJSON(raw, &s.request) != nil || s.request != request {
		return nil, ErrRecoveryBinding
	}
	canonical, e := json.Marshal(s.request)
	if e != nil || !reflect.DeepEqual(raw, canonical) {
		return nil, ErrRecoveryJournal
	}
	if e := targetValidateBMigrationJournalKind(s, kind); e != nil {
		return nil, e
	}
	namesAfter, e := listTargetJournalFiles(fd)
	if e != nil || !targetJournalNamesEqual(names, namesAfter) || verifyTargetJournalDirectories(dirs) != nil {
		return nil, ErrRecoveryJournal
	}
	// Re-read every physical file after the complete directory read. Stamp and
	// content are both bound; a replacement or later modification is rejected.
	for i, name := range names {
		_, binding, e := readTargetJournalEntry(ctx, fd, name)
		if e != nil || binding != s.entries[i] {
			return nil, ErrRecoveryJournal
		}
	}
	finalNames, e := listTargetJournalFiles(fd)
	if e != nil || !targetJournalNamesEqual(names, finalNames) || verifyTargetJournalDirectories(dirs) != nil {
		return nil, ErrRecoveryJournal
	}
	s.hash = jsonSHA(struct {
		Request     TargetRecoveryRequest
		WindowStart string
		Directory   targetJournalDirectoryBinding
		Files       []targetJournalEntryBinding
	}{s.request, windowStart, s.directory, s.entries})
	if !hashPattern.MatchString(s.hash) || ctx.Err() != nil {
		return nil, ErrRecoveryJournal
	}
	return s, nil
}

// SealTargetRecoveryJournal reads actual original files, including their
// physical metadata. The returned hash can be independently registered by the
// host; it is not a writer fence or an instruction to adopt an unknown intent.
func SealTargetRecoveryJournal(ctx context.Context, p *TargetRecoveryPlan) (TargetRecoveryJournalSummary, error) {
	var out TargetRecoveryJournalSummary
	if p == nil || p.self != p || ctx == nil || ctx.Err() != nil {
		return out, ErrRecoveryBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.journal == nil || p.window == nil || p.journal.validate() != nil {
		return out, ErrRecoveryJournal
	}
	window, e := p.window.Diagnostic(ctx)
	if e != nil || window.Binding.SourceSHA != p.request.SourceSHA || window.Binding.OperationID != p.request.OperationID || window.Binding.ManifestSHA256 != p.request.ManifestSHA256 || window.Binding.OriginalRunID != p.request.OriginalRunID || !window.DirectoryLeaseHeld || !hashPattern.MatchString(window.StartSHA256) {
		return out, ErrRecoveryBinding
	}
	s, e := inspectTargetJournal(ctx, p.journal.dir, p.request, window.StartSHA256)
	if e != nil {
		return out, e
	}
	out = TargetRecoveryJournalSummary{JournalSHA256: s.hash, RequestSHA256: jsonSHA(p.request), WindowStartSHA256: window.StartSHA256, Files: len(s.entries)}
	return out, nil
}

func targetJournalRecord(snapshot *targetJournalSnapshot, name string) (*targetStatementRecord, error) {
	if snapshot == nil {
		return nil, ErrRecoveryJournal
	}
	raw, ok := snapshot.files[name]
	if !ok {
		return nil, nil
	}
	var record targetStatementRecord
	if exactJSON(raw, &record) != nil {
		return nil, ErrRecoveryJournal
	}
	// Only the exact original json.Marshal writer form is accepted. Whitespace,
	// missing fields and duplicate aliases cannot masquerade as original output.
	expected, e := json.Marshal(record)
	if e != nil || !reflect.DeepEqual(raw, expected) {
		return nil, ErrRecoveryJournal
	}
	return &record, nil
}

// Restore rows may use a separately recorded current run/connection; DROP rows
// always retain their actual original request and connection. A file alone is
// no permission: callers still require successful DROP + actual absence.
func targetJournalRestoreBinding(s *targetJournalSnapshot, i int) (TargetRecoveryRequest, string, bool, error) {
	if s == nil || i < 0 || i >= 4 {
		return TargetRecoveryRequest{}, "", false, ErrRecoveryBinding
	}
	name := "target-recovery-" + string(rune('0'+i)) + "-resume-binding.json"
	raw, ok := s.files[name]
	if !ok {
		return s.request, "", false, nil
	}
	var binding targetResumeBindingRecord
	if exactJSON(raw, &binding) != nil {
		return TargetRecoveryRequest{}, "", false, ErrRecoveryJournal
	}
	canonical, e := json.Marshal(binding)
	want := s.request
	want.ActualRunID = binding.Request.ActualRunID
	if e != nil || !reflect.DeepEqual(raw, canonical) || binding.Request != want || !runPattern.MatchString(binding.Request.ActualRunID) || binding.OriginalRequestSHA256 != jsonSHA(s.request) || binding.WindowStartSHA256 != s.windowStart || !targetOriginalConnectionID.MatchString(binding.SQLConnectionID) {
		return TargetRecoveryRequest{}, "", false, ErrRecoveryJournal
	}
	return binding.Request, binding.SQLConnectionID, true, nil
}

// This reopens only the just-inspected physical ledger, not a serialized DROP
// proof. Original native targetDropProof values are deliberately never minted.
func reopenTargetRecoveryJournal(ctx context.Context, path string, s *targetJournalSnapshot) (*targetRecoveryJournal, error) {
	return reopenTargetRecoveryJournalKind(ctx, path, s, targetBJournalComplete)
}
func reopenTargetRecoveryJournalKind(ctx context.Context, path string, s *targetJournalSnapshot, kind targetBJournalKind) (*targetRecoveryJournal, error) {
	if !targetBJournalKindValid(kind) {
		return nil, ErrRecoveryBinding
	}
	if ctx == nil || ctx.Err() != nil || s == nil || privateDirectory(path) != nil {
		return nil, ErrRecoveryJournal
	}
	st, e := os.Lstat(path)
	if e != nil {
		return nil, ErrRecoveryJournal
	}
	stamp, e := targetStamp(st)
	if e != nil || stamp.device != s.directory.Device || stamp.inode != s.directory.Inode || stamp.owner != s.directory.UID || uint64(stamp.links) != s.directory.Links || stamp.mode.Perm() != 0700 {
		return nil, ErrRecoveryJournal
	}
	j := &targetRecoveryJournal{dir: path, inode: stamp, files: make(map[string]targetJournalAsset, len(s.entries))}
	j.self = j
	for _, entry := range s.entries {
		info, e := os.Lstat(filepath.Join(path, entry.Name))
		if e != nil {
			return nil, ErrRecoveryJournal
		}
		actual, e := targetStamp(info)
		if e != nil || actual.device != entry.Device || actual.inode != entry.Inode || actual.owner != entry.UID || actual.links != entry.Links || actual.mode.Perm() != os.FileMode(entry.Mode) || actual.size != entry.Bytes || actual.modified != entry.Modified || actual.changed != entry.Changed {
			return nil, ErrRecoveryJournal
		}
		j.files[entry.Name] = targetJournalAsset{hash: entry.SHA256, stamp: actual}
	}
	if j.validate() != nil {
		return nil, ErrRecoveryJournal
	}
	fresh, e := inspectTargetJournalKind(ctx, path, s.request, s.windowStart, kind)
	if e != nil || fresh.hash != s.hash {
		return nil, ErrRecoveryJournal
	}
	return j, nil
}

// InspectTargetRecoveryJournal lets a restarted fixed host register the actual
// physical journal digest before requesting reconciliation. It reads complete
// bounded files under the original window/backup binding; it never accepts a
// receipt as proof, invents a DROP result, or authorizes a database mutation.
func InspectTargetRecoveryJournal(ctx context.Context, a *Archive, r TargetRecoveryRequest, path string, w *fence.MaintenanceWindow) (TargetRecoveryJournalSummary, error) {
	return inspectTargetRecoveryJournalKind(ctx, a, r, path, w, targetBJournalComplete)
}
func inspectTargetRecoveryJournalKind(ctx context.Context, a *Archive, r TargetRecoveryRequest, path string, w *fence.MaintenanceWindow, kind targetBJournalKind) (TargetRecoveryJournalSummary, error) {
	var out TargetRecoveryJournalSummary
	if ctx == nil || ctx.Err() != nil || a == nil || !hashPattern.MatchString(r.ManifestSHA256) || !hashPattern.MatchString(r.SQLNonTargetSHA256) || !hashPattern.MatchString(r.MongoNonTargetSHA256) || !runPattern.MatchString(r.ActualRunID) || r.SourceSHA != a.data.Approval.SourceSHA || r.OperationID != a.data.Approval.OperationID || r.OriginalRunID != a.data.Approval.RunID || r.ArchiveSHA256 != a.digest || !sourcePattern.MatchString(r.SourceSHA) || !runPattern.MatchString(r.OperationID) {
		return out, ErrRecoveryBinding
	}
	if !targetSupportedHead(r.SQLHead, a.data.Inventory.Bindings["mysql"].Version) || !targetSupportedHead(r.MongoHead, a.data.Inventory.Bindings["mongodb"].Version) {
		return out, ErrRecoveryHead
	}
	if a.verifyAssets(ctx) != nil {
		return out, ErrSource
	}
	start, e := targetWindowMatches(ctx, w, r, "")
	if e != nil {
		return out, e
	}
	s, e := inspectTargetJournalKind(ctx, path, r, start, kind)
	if e != nil {
		return out, e
	}
	// Parse every target record but keep unknown/intents unchanged. This is a
	// seal for independent registration, not a successful recovery conclusion.
	for i := 0; i < 4; i++ {
		if _, e = classifyTargetJournal(s, a, i); e != nil {
			return out, e
		}
	}
	if _, e = targetWindowMatches(ctx, w, r, start); e != nil {
		return out, e
	}
	if a.verifyAssets(ctx) != nil {
		return out, ErrSource
	}
	out = TargetRecoveryJournalSummary{JournalSHA256: s.hash, RequestSHA256: jsonSHA(r), WindowStartSHA256: start, Files: len(s.entries)}
	return out, nil
}
