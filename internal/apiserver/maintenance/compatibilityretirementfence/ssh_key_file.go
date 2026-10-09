package compatibilityretirementfence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

var (
	ErrSSHKeyFile        = errors.New("retirement_ssh_key_file_binding_rejected")
	ErrSSHKeyFileUsed    = errors.New("retirement_ssh_key_file_already_reserved")
	ErrSSHKeyFileUnknown = errors.New("retirement_ssh_key_file_mutation_or_durability_unknown")
)

const sshKeyFileProtocol = "existing_protected_ssh_key_file_only/v1"

type sshKeyFileOptions struct {
	owner    uint32
	anchor   string
	exchange func(int, string, string) error
	metadata func(int) error
	// These options are inaccessible to callers. Production uses Linux/root only.
	after func(string) error
}

type sshKeyFileRecord struct {
	Protocol            string         `json:"protocol"`
	Kind                string         `json:"kind"`
	WindowBindingSHA256 string         `json:"window_binding_sha256"`
	WindowStartSHA256   string         `json:"window_start_sha256"`
	OperationDirectory  windowIdentity `json:"operation_directory"`
	KeysDirectory       windowIdentity `json:"keys_directory"`
	KeysPath            string         `json:"keys_path"`
	SlotName            string         `json:"slot_name"`
	Original            windowStamp    `json:"original"`
	Restricted          windowStamp    `json:"restricted"`
	OriginalSHA256      string         `json:"original_sha256"`
	RestrictedSHA256    string         `json:"restricted_sha256"`
	IntentSHA256        string         `json:"intent_sha256,omitempty"`
}

// SSHKeyFileReceipt describes only this exact existing-file operation. Neither
// successful installation nor restoration is an sshd, whole-writer-fence,
// privileged-controller, database mutation, or DROP authorization.
type SSHKeyFileReceipt struct {
	Protocol                  string `json:"protocol"`
	OriginalSHA256            string `json:"original_sha256"`
	RestrictedSHA256          string `json:"restricted_sha256"`
	WindowStartSHA256         string `json:"window_start_sha256"`
	InstallResultSHA256       string `json:"install_result_sha256"`
	RestoreResultSHA256       string `json:"restore_result_sha256"`
	ExactFileInstalled        bool   `json:"exact_file_installed"`
	ExactOriginalRestored     bool   `json:"exact_original_restored"`
	StateUnknown              bool   `json:"state_unknown"`
	RootEffectiveSSHDVerified bool   `json:"root_effective_sshd_verified"`
	WholeSystemWritersFenced  bool   `json:"whole_system_writers_fenced"`
	MutationBackendEnabled    bool   `json:"mutation_backend_enabled"`
	DropReady                 bool   `json:"drop_ready"`
}

// SSHKeyFileLease retains both original file and directory descriptors, an
// exclusive operation-directory flock and the original in-process budget. It
// has no reopen/adopt/replay API. Unknown outcomes preserve all actual assets.
// Close releases descriptors only; it does not undo writes or remove records.
// The original file must exist; absent files and account/key creation are not
// supported. File ownership and mode are preserved exactly. Files with ACLs or
// extended attributes are rejected; this leaf never silently drops them.
//
// Atomic exchange is not a kernel conditional rename: a concurrent root writer
// ignoring these locks can race between checks. Post-exchange mismatch becomes
// Unknown and never triggers automatic overwrite or rollback. This primitive
// must not be used as proof that those writers have been fenced.
type SSHKeyFileLease struct {
	self                                            *SSHKeyFileLease
	mu                                              sync.Mutex
	window                                          *MaintenanceWindow
	windowStart                                     string
	binding                                         WindowBinding
	prep                                            SSHPreparation
	opts                                            sshKeyFileOptions
	operationDirs, keyDirs                          []challengeDir
	keyName, keyPath, slotName                      string
	originalFD, restrictedFD                        int
	originalStamp, restrictedStamp                  windowStamp
	records                                         map[string]windowSavedFile
	installed, restored, attempted, unknown, closed bool
}

func (*SSHKeyFileLease) MarshalJSON() ([]byte, error) { return nil, ErrSSHKeyFile }
func (*SSHKeyFileLease) String() string {
	return "opaque existing SSH key-file lease; no writer-fence authority"
}
func (l *SSHKeyFileLease) GoString() string { return l.String() }

// OpenSSHKeyFileLease requires a root-preapproved, empty, protected 0700
// operation directory named for the original Window's operation. The original
// keys parent must also be root protected. No permissions are changed to make
// a caller-controlled path eligible. The Window grants only a time budget.
func OpenSSHKeyFileLease(ctx context.Context, operationDir, keysPath string, preparation *SSHPreparation, originalWindow *MaintenanceWindow) (*SSHKeyFileLease, error) {
	opts, err := productionSSHKeyFileOptions()
	if err != nil {
		return nil, err
	}
	return openSSHKeyFileLease(ctx, operationDir, keysPath, preparation, originalWindow, opts)
}

func openSSHKeyFileLease(ctx context.Context, operationDir, keysPath string, p *SSHPreparation, w *MaintenanceWindow, opts sshKeyFileOptions) (l *SSHKeyFileLease, err error) {
	if ctx == nil || ctx.Err() != nil || p == nil || w == nil || w.self != w || opts.exchange == nil || opts.metadata == nil || !challengePathsValid(keysPath, opts.anchor) || len(p.original) == 0 || len(p.restricted) == 0 || len(p.original) > MaxBodyBytes || len(p.restricted) > MaxBodyBytes || digest(p.original) != p.originalHash || digest(p.restricted) != p.restrictedHash || len(p.keys) == 0 {
		return nil, ErrSSHKeyFile
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	sample, e := w.validateLocked(ctx)
	if e != nil {
		return nil, e
	}
	if w.opts.owner != opts.owner || w.opts.anchor != opts.anchor || w.recovery != nil || sample.nanos >= w.start.ForwardDeadline || filepath.Base(operationDir) != w.binding.OperationID {
		return nil, ErrSSHKeyFile
	}
	keyDir := filepath.Dir(keysPath)
	// The retained Window descriptors bind its actual absolute path. Never put
	// leaf records inside that exact-names budget directory.
	windowDir := w.opts.anchor
	for _, d := range w.dirs[1:] {
		windowDir = filepath.Join(windowDir, d.name)
	}
	for _, pair := range [][2]string{{operationDir, keyDir}, {operationDir, windowDir}, {keyDir, windowDir}} {
		if sshKeyPathsOverlap(pair[0], pair[1]) {
			return nil, ErrSSHKeyFile
		}
	}
	l = &SSHKeyFileLease{window: w, windowStart: digest(w.files[windowStartName].raw), binding: w.binding, prep: SSHPreparation{original: bytes.Clone(p.original), restricted: bytes.Clone(p.restricted), originalHash: p.originalHash, restrictedHash: p.restrictedHash, keys: append([]string(nil), p.keys...)}, opts: opts, keyName: filepath.Base(keysPath), keyPath: keysPath, slotName: ".qs-retirement-" + w.binding.OperationID + ".original", originalFD: -1, restrictedFD: -1, records: map[string]windowSavedFile{}}
	l.self = l
	defer func() {
		if err != nil {
			_ = l.Close()
			l = nil
		}
	}()
	l.operationDirs, e = openChallengeDirs(operationDir, challengeOptions{owner: opts.owner, anchor: opts.anchor})
	if e != nil {
		return l, ErrSSHKeyFile
	}
	if unix.Flock(l.opFD(), unix.LOCK_EX|unix.LOCK_NB) != nil {
		return l, ErrSSHKeyFileUsed
	}
	if sshKeyDirectoryEmpty(l.opFD()) != nil {
		return l, ErrSSHKeyFileUsed
	}
	l.keyDirs, e = openSSHKeyDirectories(keyDir, opts)
	if e != nil {
		return l, e
	}
	l.originalFD, e = unix.Openat(l.keyFD(), l.keyName, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return l, ErrSSHKeyFile
	}
	if unix.Flock(l.originalFD, unix.LOCK_EX|unix.LOCK_NB) != nil {
		return l, ErrSSHKeyFileUsed
	}
	var st unix.Stat_t
	if unix.Fstat(l.originalFD, &st) != nil || !sshKeyFileValid(st, opts.owner) || opts.metadata(l.originalFD) != nil {
		return l, ErrSSHKeyFile
	}
	l.originalStamp = windowFileStamp(st)
	if sshKeyReadExact(l.originalFD, l.keyFD(), l.keyName, l.originalStamp, l.prep.original, opts.owner, opts.metadata) != nil || l.checkDirectories() != nil {
		return l, ErrSSHKeyFile
	}
	var slot unix.Stat_t
	if e = unix.Fstatat(l.keyFD(), l.slotName, &slot, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(e, unix.ENOENT) {
		return l, ErrSSHKeyFileUsed
	}
	return l, nil
}

func sshKeyPathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}
func (l *SSHKeyFileLease) opFD() int  { return l.operationDirs[len(l.operationDirs)-1].fd }
func (l *SSHKeyFileLease) keyFD() int { return l.keyDirs[len(l.keyDirs)-1].fd }
func sshKeyFileValid(st unix.Stat_t, owner uint32) bool {
	return st.Mode&unix.S_IFMT == unix.S_IFREG && st.Uid == owner && st.Nlink == 1 && st.Mode&0022 == 0 && st.Mode&07000 == 0 && st.Size > 0 && st.Size <= MaxBodyBytes
}

func openSSHKeyDirectories(path string, opts sshKeyFileOptions) (dirs []challengeDir, err error) {
	if !challengePathsValid(path, opts.anchor) {
		return nil, ErrSSHKeyFile
	}
	defer func() {
		if err != nil {
			for _, d := range dirs {
				_ = unix.Close(d.fd)
			}
			dirs = nil
		}
	}()
	fd, e := unix.Open(opts.anchor, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrSSHKeyFile
	}
	dirs = append(dirs, challengeDir{fd: fd, parent: -1, name: opts.anchor})
	if unix.Fstat(fd, &dirs[0].initial) != nil || !challengeDirValid(dirs[0].initial, opts.owner, false) {
		return dirs, ErrSSHKeyFile
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(path, opts.anchor), "/")
	if rel != "" {
		for _, name := range strings.Split(rel, "/") {
			parent := dirs[len(dirs)-1].fd
			child, e := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
			if e != nil {
				return dirs, ErrSSHKeyFile
			}
			dirs = append(dirs, challengeDir{fd: child, parent: parent, name: name})
			last := &dirs[len(dirs)-1]
			if unix.Fstat(child, &last.initial) != nil || !challengeDirValid(last.initial, opts.owner, false) {
				return dirs, ErrSSHKeyFile
			}
		}
	}
	if checkChallengeDirs(dirs, opts.owner) != nil {
		return dirs, ErrSSHKeyFile
	}
	return dirs, nil
}
func sshKeyDirectoryEmpty(fd int) (err error) {
	duplicate, e := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrSSHKeyFile
	}
	f := os.NewFile(uintptr(duplicate), "ssh-key-operation-directory")
	defer func() {
		if f.Close() != nil {
			err = ErrSSHKeyFileUnknown
		}
	}()
	names, e := f.Readdirnames(1)
	if e != io.EOF || len(names) != 0 {
		return ErrSSHKeyFileUsed
	}
	return nil
}
func sshKeyReadExact(fd, dir int, name string, stamp windowStamp, data []byte, owner uint32, metadata func(int) error) error {
	var before, after, linked unix.Stat_t
	if unix.Fstat(fd, &before) != nil || metadata(fd) != nil || !sshKeyFileValid(before, owner) || windowFileStamp(before) != stamp || unix.Fstatat(dir, name, &linked, unix.AT_SYMLINK_NOFOLLOW) != nil || windowFileStamp(linked) != stamp {
		return ErrChanged
	}
	actual := make([]byte, len(data)+1)
	n, e := unix.Pread(fd, actual, 0)
	if e != nil || n != len(data) || !bytes.Equal(actual[:n], data) || metadata(fd) != nil || unix.Fstat(fd, &after) != nil || windowFileStamp(after) != stamp || unix.Fstatat(dir, name, &linked, unix.AT_SYMLINK_NOFOLLOW) != nil || windowFileStamp(linked) != stamp {
		return ErrChanged
	}
	return nil
}
func (l *SSHKeyFileLease) checkDirectories() error {
	if checkChallengeDirs(l.operationDirs, l.opts.owner) != nil || checkChallengeDirs(l.keyDirs, l.opts.owner) != nil {
		return ErrChanged
	}
	for _, dirs := range [][]challengeDir{l.operationDirs, l.keyDirs} {
		for _, d := range dirs {
			var actual unix.Stat_t
			if unix.Fstat(d.fd, &actual) != nil || actual.Mode != d.initial.Mode || actual.Gid != d.initial.Gid {
				return ErrChanged
			}
		}
	}
	return nil
}
func (l *SSHKeyFileLease) windowValid(ctx context.Context, forward bool) error {
	w := l.window
	if w == nil || w.self != w {
		return ErrSSHKeyFile
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	sample, e := w.validateLocked(ctx)
	if e != nil {
		return e
	}
	if w.binding != l.binding || digest(w.files[windowStartName].raw) != l.windowStart {
		return ErrSSHKeyFile
	}
	limit := w.start.WindowDeadline
	if forward {
		if w.recovery != nil {
			return ErrWindowExpired
		}
		limit = w.start.ForwardDeadline
	} else if w.recovery != nil {
		limit = w.recovery.DeadlineNanos
	}
	if sample.nanos >= limit {
		return ErrWindowExpired
	}
	return nil
}
func (l *SSHKeyFileLease) hook(stage string) error {
	if l.opts.after != nil && l.opts.after(stage) != nil {
		return ErrSSHKeyFileUnknown
	}
	return nil
}
func (l *SSHKeyFileLease) record(kind, intentHash string) sshKeyFileRecord {
	return sshKeyFileRecord{Protocol: sshKeyFileProtocol, Kind: kind, WindowBindingSHA256: windowBindingHash(l.binding), WindowStartSHA256: l.windowStart, OperationDirectory: windowDirectory(l.operationDirs), KeysDirectory: windowDirectory(l.keyDirs), KeysPath: l.keyPath, SlotName: l.slotName, Original: l.originalStamp, Restricted: l.restrictedStamp, OriginalSHA256: l.prep.originalHash, RestrictedSHA256: l.prep.restrictedHash, IntentSHA256: intentHash}
}
func (l *SSHKeyFileLease) writeRecord(kind, intentHash string) error {
	raw, e := windowMarshal(l.record(kind, intentHash))
	if e != nil {
		return ErrSSHKeyFileUnknown
	}
	name := kind + ".json"
	saved, e := writeChallengeFile(l.opFD(), name, raw, challengeOptions{owner: l.opts.owner, anchor: l.opts.anchor}, "")
	if e != nil {
		return ErrSSHKeyFileUnknown
	}
	l.records[name] = windowSavedFile{file: saved, raw: raw}
	if updateChallengeDirAfterOwnFile(l.operationDirs, saved, l.opts.owner) != nil || unix.Fsync(l.opFD()) != nil || readChallengeFile(l.opFD(), saved, raw, l.opts.owner) != nil || l.checkDirectories() != nil || l.hook(kind+"_durable") != nil {
		return ErrSSHKeyFileUnknown
	}
	actual, e := readWindowFile(l.opFD(), name, l.opts.owner)
	if e != nil || !bytes.Equal(actual.raw, raw) {
		return ErrSSHKeyFileUnknown
	}
	l.records[name] = actual
	return nil
}
func (l *SSHKeyFileLease) checkRecords() error {
	fd, e := unix.Openat(l.opFD(), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrSSHKeyFileUnknown
	}
	f := os.NewFile(uintptr(fd), "ssh-key-operation-directory")
	names, e := f.Readdirnames(-1)
	closeErr := f.Close()
	if e != nil || closeErr != nil || len(names) != len(l.records) {
		return ErrSSHKeyFileUnknown
	}
	for _, name := range names {
		if _, ok := l.records[name]; !ok {
			return ErrSSHKeyFileUnknown
		}
	}
	for name, r := range l.records {
		actual, e := readWindowFile(l.opFD(), name, l.opts.owner)
		if e != nil || actual.stamp != r.stamp || !bytes.Equal(actual.raw, r.raw) {
			return ErrSSHKeyFileUnknown
		}
	}
	return l.checkDirectories()
}
func (l *SSHKeyFileLease) originalExact() error {
	return sshKeyReadExact(l.originalFD, l.keyFD(), l.keyName, l.originalStamp, l.prep.original, l.opts.owner, l.opts.metadata)
}
func (l *SSHKeyFileLease) restrictedExact() error {
	return sshKeyReadExact(l.restrictedFD, l.keyFD(), l.keyName, l.restrictedStamp, l.prep.restricted, l.opts.owner, l.opts.metadata)
}

// Install performs one attempt only. A durable O_EXCL intent precedes file
// creation/exchange. Any uncertain intermediate result retains both files and
// records and permanently disables Install/Restore on this receiver.
func (l *SSHKeyFileLease) Install(ctx context.Context) (receipt SSHKeyFileReceipt, err error) {
	if l == nil || l.self != l {
		return receipt, ErrSSHKeyFile
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.unknown || l.attempted {
		return l.receipt(), ErrSSHKeyFileUsed
	}
	if e := l.windowValid(ctx, true); e != nil {
		return l.receipt(), e
	}
	bound, cancel, e := l.window.ForwardContext(ctx)
	if e != nil {
		return l.receipt(), e
	}
	defer cancel()
	ctx = bound
	if l.checkDirectories() != nil || l.originalExact() != nil || sshKeyDirectoryEmpty(l.opFD()) != nil {
		return l.receipt(), ErrChanged
	}
	l.attempted = true
	defer func() {
		if err != nil {
			l.unknown = true
		}
		receipt = l.receipt()
	}()
	if l.writeRecord("install-intent", "") != nil {
		return receipt, ErrSSHKeyFileUnknown
	}
	if unix.Fsync(l.originalFD) != nil || l.originalExact() != nil || l.hook("original_durable") != nil {
		return receipt, ErrSSHKeyFileUnknown
	}
	if l.createRestricted() != nil {
		return receipt, ErrSSHKeyFileUnknown
	}
	if l.checkRecords() != nil || l.originalExact() != nil || sshKeyReadExact(l.restrictedFD, l.keyFD(), l.slotName, l.restrictedStamp, l.prep.restricted, l.opts.owner, l.opts.metadata) != nil || l.windowValid(ctx, true) != nil || l.hook("before_install_exchange") != nil {
		return receipt, ErrSSHKeyFileUnknown
	}
	if l.opts.exchange(l.keyFD(), l.keyName, l.slotName) != nil {
		return receipt, ErrSSHKeyFileUnknown
	}
	if l.hook("install_exchanged") != nil || l.readAfterExchange(false, true) != nil || unix.Fsync(l.keyFD()) != nil || l.hook("install_directory_durable") != nil || l.checkRecords() != nil || l.windowValid(ctx, true) != nil {
		return receipt, ErrSSHKeyFileUnknown
	}
	if l.writeRecord("install-result", digest(l.records["install-intent.json"].raw)) != nil {
		return receipt, ErrSSHKeyFileUnknown
	}
	if l.readAfterExchange(false, false) != nil || l.checkRecords() != nil || l.windowValid(ctx, true) != nil {
		return receipt, ErrSSHKeyFileUnknown
	}
	l.installed = true
	return receipt, nil
}
func (l *SSHKeyFileLease) createRestricted() (err error) {
	fd, e := unix.Openat(l.keyFD(), l.slotName, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if e != nil {
		return ErrSSHKeyFileUnknown
	}
	l.restrictedFD = fd
	if unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil {
		return ErrSSHKeyFileUnknown
	}
	if l.hook("restricted_created") != nil {
		return ErrSSHKeyFileUnknown
	}
	// Production root preserves the original root owner/group exactly. Tests
	// use their actual local owner; they cannot enable the public root API.
	if unix.Fchown(fd, int(l.originalStamp.Owner), int(l.originalStamp.Group)) != nil || unix.Fchmod(fd, l.originalStamp.Mode&07777) != nil {
		return ErrSSHKeyFileUnknown
	}
	if n, e := unix.Write(fd, l.prep.restricted); e != nil || n != len(l.prep.restricted) || unix.Fsync(fd) != nil {
		return ErrSSHKeyFileUnknown
	}
	var st, dir unix.Stat_t
	if unix.Fstat(fd, &st) != nil || !sshKeyFileValid(st, l.opts.owner) || uint32(st.Mode) != l.originalStamp.Mode || st.Gid != l.originalStamp.Group {
		return ErrSSHKeyFileUnknown
	}
	l.restrictedStamp = windowFileStamp(st)
	if sshKeyReadExact(fd, l.keyFD(), l.slotName, l.restrictedStamp, l.prep.restricted, l.opts.owner, l.opts.metadata) != nil {
		return ErrSSHKeyFileUnknown
	}
	last := &l.keyDirs[len(l.keyDirs)-1]
	if unix.Fstat(last.fd, &dir) != nil || !challengeSameInode(last.initial, dir) || !challengeDirValid(dir, l.opts.owner, false) || (dir.Nlink != last.initial.Nlink && dir.Nlink != last.initial.Nlink+1) {
		return ErrSSHKeyFileUnknown
	}
	last.initial.Nlink = dir.Nlink
	if l.checkDirectories() != nil || unix.Fsync(l.keyFD()) != nil || l.hook("restricted_durable") != nil {
		return ErrSSHKeyFileUnknown
	}
	return nil
}

// A rename may change ctime. Only the exact own exchange can update that stamp;
// all other identity, owner, mode, link, size and mtime fields remain immutable.
func sshKeyOwnRenameStamp(fd int, previous windowStamp) (windowStamp, error) {
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil {
		return windowStamp{}, ErrSSHKeyFileUnknown
	}
	actual := windowFileStamp(st)
	compare := actual
	compare.ChangedNanos = previous.ChangedNanos
	if compare != previous || actual.ChangedNanos < previous.ChangedNanos {
		return windowStamp{}, ErrChanged
	}
	return actual, nil
}
func (l *SSHKeyFileLease) readAfterExchange(restored, ownRename bool) error {
	original, restricted := l.originalStamp, l.restrictedStamp
	if ownRename {
		var e error
		original, e = sshKeyOwnRenameStamp(l.originalFD, l.originalStamp)
		if e != nil {
			return e
		}
		restricted, e = sshKeyOwnRenameStamp(l.restrictedFD, l.restrictedStamp)
		if e != nil {
			return e
		}
	}
	originalName, restrictedName := l.slotName, l.keyName
	if restored {
		originalName, restrictedName = l.keyName, l.slotName
	}
	if sshKeyReadExact(l.originalFD, l.keyFD(), originalName, original, l.prep.original, l.opts.owner, l.opts.metadata) != nil || sshKeyReadExact(l.restrictedFD, l.keyFD(), restrictedName, restricted, l.prep.restricted, l.opts.owner, l.opts.metadata) != nil {
		return ErrChanged
	}
	l.originalStamp, l.restrictedStamp = original, restricted
	return nil
}

// Restore exchanges back only this lease's exact original inode and this
// batch's exact installed inode/bytes. It never recreates an absent original,
// overwrites concurrent edits, resets a Window, or replays an unknown exchange.
func (l *SSHKeyFileLease) Restore(ctx context.Context) (receipt SSHKeyFileReceipt, err error) {
	if l == nil || l.self != l {
		return receipt, ErrSSHKeyFile
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.unknown || !l.installed || l.restored {
		return l.receipt(), ErrSSHKeyFileUsed
	}
	if e := l.windowValid(ctx, false); e != nil {
		return l.receipt(), e
	}
	if l.checkRecords() != nil || l.restrictedExact() != nil || sshKeyReadExact(l.originalFD, l.keyFD(), l.slotName, l.originalStamp, l.prep.original, l.opts.owner, l.opts.metadata) != nil {
		l.unknown = true
		return l.receipt(), ErrChanged
	}
	bound, cancel, e := l.window.RecoveryContext(ctx)
	if e != nil {
		return l.receipt(), e
	}
	defer cancel()
	ctx = bound
	// Presence of this intent prevents a second attempt even after interruption.
	l.unknown = true
	defer func() { receipt = l.receipt() }()
	if l.writeRecord("restore-intent", digest(l.records["install-result.json"].raw)) != nil || l.checkRecords() != nil || l.restrictedExact() != nil || sshKeyReadExact(l.originalFD, l.keyFD(), l.slotName, l.originalStamp, l.prep.original, l.opts.owner, l.opts.metadata) != nil || l.windowValid(ctx, false) != nil || l.hook("before_restore_exchange") != nil {
		return receipt, ErrSSHKeyFileUnknown
	}
	if l.opts.exchange(l.keyFD(), l.keyName, l.slotName) != nil {
		return receipt, ErrSSHKeyFileUnknown
	}
	if l.hook("restore_exchanged") != nil || l.readAfterExchange(true, true) != nil || unix.Fsync(l.keyFD()) != nil || l.hook("restore_directory_durable") != nil || l.checkRecords() != nil || l.windowValid(ctx, false) != nil {
		return receipt, ErrSSHKeyFileUnknown
	}
	if l.writeRecord("restore-result", digest(l.records["restore-intent.json"].raw)) != nil || l.readAfterExchange(true, false) != nil || l.checkRecords() != nil || l.windowValid(ctx, false) != nil {
		return receipt, ErrSSHKeyFileUnknown
	}
	l.restored = true
	l.installed = false
	l.unknown = false
	return receipt, nil
}
func (l *SSHKeyFileLease) receipt() SSHKeyFileReceipt {
	r := SSHKeyFileReceipt{Protocol: sshKeyFileProtocol, OriginalSHA256: l.prep.originalHash, RestrictedSHA256: l.prep.restrictedHash, WindowStartSHA256: l.windowStart, ExactFileInstalled: l.installed && !l.unknown, ExactOriginalRestored: l.restored && !l.unknown, StateUnknown: l.unknown}
	if f, ok := l.records["install-result.json"]; ok {
		r.InstallResultSHA256 = digest(f.raw)
	}
	if f, ok := l.records["restore-result.json"]; ok {
		r.RestoreResultSHA256 = digest(f.raw)
	}
	return r
}
func (l *SSHKeyFileLease) Close() error {
	if l == nil || l.self == nil {
		return nil
	}
	if l.self != l {
		return ErrSSHKeyFile
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	failed := false
	for _, fd := range []int{l.originalFD, l.restrictedFD} {
		if fd >= 0 && unix.Close(fd) != nil {
			failed = true
		}
	}
	l.originalFD, l.restrictedFD = -1, -1
	for _, dirs := range [][]challengeDir{l.keyDirs, l.operationDirs} {
		for i := len(dirs) - 1; i >= 0; i-- {
			if unix.Close(dirs[i].fd) != nil {
				failed = true
			}
		}
	}
	if failed {
		l.unknown = true
		return ErrSSHKeyFileUnknown
	}
	return nil
}

// Keep the lease state non-serializable; only metadata records are marshalled.
var _ json.Marshaler = (*SSHKeyFileLease)(nil)
