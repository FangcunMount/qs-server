package compatibilityretirementfence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

var (
	ErrWindowUnavailable = errors.New("maintenance_window_platform_unavailable")
	ErrWindowBinding     = errors.New("maintenance_window_binding_rejected")
	ErrWindowStore       = errors.New("maintenance_window_store_rejected")
	ErrWindowExists      = errors.New("maintenance_window_already_initialized_or_unknown")
	ErrWindowBusy        = errors.New("maintenance_window_directory_lease_busy")
	ErrWindowUnknown     = errors.New("maintenance_window_state_or_durability_unknown")
	ErrWindowClock       = errors.New("maintenance_window_boot_clock_unknown_or_changed")
	ErrWindowExpired     = errors.New("maintenance_window_absolute_deadline_exceeded")
	ErrWindowClosed      = errors.New("maintenance_window_lease_closed")
)

const (
	windowProtocol        = "root_directory_budget_only/v1"
	windowLockName        = "maintenance-window.lock"
	windowStartName       = "maintenance-window.start.json"
	windowStartSealName   = "maintenance-window.start.seal.json"
	windowRecoverName     = "maintenance-window.recovery.json"
	windowRecoverSealName = "maintenance-window.recovery.seal.json"
	windowForwardNanos    = int64(1200 * time.Second)
	windowTotalNanos      = int64(1800 * time.Second)
	windowRecoveryNanos   = int64(600 * time.Second)
)

// WindowBinding fixes a budget's original batch. Syntax and equality are checked;
// this DTO is not independent approval, a writer fence, or an execution permit.
type WindowBinding struct {
	TargetSHA256   string `json:"target_sha256"`
	SourceSHA      string `json:"source_sha"`
	OperationID    string `json:"operation_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	OriginalRunID  string `json:"original_run_id"`
}

func MaintenanceWindowTargetSHA256() string {
	raw, _ := json.Marshal([4][3]string{{"mysql", "domain_event_outbox", "base_table"}, {"mysql", "ai_bridge_commands", "base_table"}, {"mysql", "ai_messaging_legacy_commands", "base_table"}, {"mongodb", "domain_event_outbox", "collection"}})
	return digest(raw)
}

func (b WindowBinding) valid() bool {
	return b.TargetSHA256 == MaintenanceWindowTargetSHA256() && sha40.MatchString(b.SourceSHA) && operationID.MatchString(b.OperationID) && sha64.MatchString(b.ManifestSHA256) && operationID.MatchString(b.OriginalRunID)
}

type windowIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}
type windowStamp struct {
	Identity      windowIdentity `json:"identity"`
	Mode          uint32         `json:"mode"`
	Owner         uint32         `json:"owner"`
	Group         uint32         `json:"group"`
	Links         uint64         `json:"links"`
	Size          int64          `json:"size"`
	ModifiedNanos int64          `json:"modified_nanos"`
	ChangedNanos  int64          `json:"changed_nanos"`
}

func windowInode(st unix.Stat_t) windowIdentity {
	return windowIdentity{uint64(st.Dev), uint64(st.Ino)}
}
func windowFileStamp(st unix.Stat_t) windowStamp {
	m, c := windowStatTimes(st)
	return windowStamp{windowInode(st), uint32(st.Mode), st.Uid, st.Gid, uint64(st.Nlink), st.Size, m, c}
}

type windowClockSample struct {
	bootID string
	nanos  int64
	utc    time.Time
}
type windowOptions struct {
	owner               uint32
	anchor, clockSource string
	clock               func() (windowClockSample, error)
	// Test-only injection; the public API always supplies kernel/root options.
	after func(string) error
}
type windowLockRecord struct {
	Protocol  string         `json:"protocol"`
	Kind      string         `json:"kind"`
	Binding   WindowBinding  `json:"binding"`
	Directory windowIdentity `json:"directory"`
	Self      windowIdentity `json:"self"`
}
type windowStartRecord struct {
	Protocol        string         `json:"protocol"`
	Kind            string         `json:"kind"`
	Binding         WindowBinding  `json:"binding"`
	Directory       windowIdentity `json:"directory"`
	Lock            windowStamp    `json:"lock"`
	Self            windowIdentity `json:"self"`
	ClockSource     string         `json:"clock_source"`
	BootID          string         `json:"boot_id"`
	StartedNanos    int64          `json:"started_boot_nanos"`
	ForwardDeadline int64          `json:"forward_deadline_boot_nanos"`
	WindowDeadline  int64          `json:"window_deadline_boot_nanos"`
	ObservedUTC     string         `json:"observed_utc"`
}
type windowRecoveryRecord struct {
	Protocol           string         `json:"protocol"`
	Kind               string         `json:"kind"`
	BindingSHA256      string         `json:"binding_sha256"`
	StartSHA256        string         `json:"start_sha256"`
	Self               windowIdentity `json:"self"`
	BootID             string         `json:"boot_id"`
	FirstRecoveryNanos int64          `json:"first_recovery_boot_nanos"`
	DeadlineNanos      int64          `json:"recovery_deadline_boot_nanos"`
	ObservedUTC        string         `json:"observed_utc"`
}
type windowSeal struct {
	Protocol      string         `json:"protocol"`
	Kind          string         `json:"kind"`
	BindingSHA256 string         `json:"binding_sha256"`
	PrimarySHA256 string         `json:"primary_sha256"`
	PrimaryStamp  windowStamp    `json:"primary_stamp"`
	Self          windowIdentity `json:"self"`
}
type windowSavedFile struct {
	file  challengeFile
	raw   []byte
	stamp windowStamp
}

// MaintenanceWindow owns only its directory flock and private file/directory FDs.
// It cannot authorize probe, DDL, restore, deployment, or any database operation.
// Close releases these FDs; it never deletes or resets durable budget records.
type MaintenanceWindow struct {
	// self binds descriptor ownership to this constructor-created receiver.
	// A zero value or reflected/value copy cannot use or release its handles.
	self      *MaintenanceWindow
	mu        sync.Mutex
	dirs      []challengeDir
	lockFD    int
	binding   WindowBinding
	opts      windowOptions
	start     windowStartRecord
	recovery  *windowRecoveryRecord
	files     map[string]windowSavedFile
	lastNanos int64
	closed    bool
	done      context.Context
	cancel    context.CancelFunc
}

func (*MaintenanceWindow) MarshalJSON() ([]byte, error) { return nil, ErrWindowBinding }
func (*MaintenanceWindow) String() string               { return "opaque budget-only root-directory lease" }
func (w *MaintenanceWindow) GoString() string           { return w.String() }

// StartMaintenanceWindow reserves a new immutable start. Every existing lock or
// state, including an interrupted initialization, rejects duplicate Start.
// The terminal directory must be root-owned 0700; all ancestors must be root
// owned and not group/world writable. Nothing changes their permissions.
func StartMaintenanceWindow(ctx context.Context, rootDir string, binding WindowBinding) (*MaintenanceWindow, error) {
	opts, err := productionWindowOptions()
	if err != nil {
		return nil, err
	}
	return startMaintenanceWindow(ctx, rootDir, binding, opts)
}

// OpenMaintenanceWindow reacquires only a fully sealed original budget with the
// same binding, boot, directory and file identities. It never adopts incomplete
// records, starts a new clock, or rewrites an existing record on another attempt.
func OpenMaintenanceWindow(ctx context.Context, rootDir string, binding WindowBinding) (*MaintenanceWindow, error) {
	opts, err := productionWindowOptions()
	if err != nil {
		return nil, err
	}
	return openMaintenanceWindow(ctx, rootDir, binding, opts)
}
func windowInput(ctx context.Context, b WindowBinding, opts windowOptions) error {
	if ctx == nil || ctx.Err() != nil || !b.valid() || opts.clock == nil || opts.clockSource == "" {
		return ErrWindowBinding
	}
	return nil
}
func windowMarshal(v any) ([]byte, error) {
	b, e := json.Marshal(v)
	if e != nil || len(b) == 0 || len(b) > 4096 {
		return nil, ErrWindowUnknown
	}
	return b, nil
}
func windowDecode(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		return ErrWindowUnknown
	}
	expected, e := windowMarshal(v)
	if e != nil || !bytes.Equal(raw, expected) {
		return ErrWindowUnknown
	}
	return nil
}
func windowBindingHash(b WindowBinding) string { raw, _ := json.Marshal(b); return digest(raw) }
func windowHook(opts windowOptions, s string) error {
	if opts.after != nil && opts.after(s) != nil {
		return ErrWindowUnknown
	}
	return nil
}
func windowDirectory(dirs []challengeDir) windowIdentity {
	return windowInode(dirs[len(dirs)-1].initial)
}
func windowNames(fd int) (names map[string]bool, err error) {
	f, e := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrWindowStore
	}
	file := os.NewFile(uintptr(f), "maintenance-window-directory")
	defer func() {
		if file.Close() != nil {
			err = ErrWindowUnknown
		}
	}()
	entries, e := file.ReadDir(-1)
	if e != nil {
		return nil, ErrWindowStore
	}
	names = map[string]bool{}
	for _, entry := range entries {
		switch entry.Name() {
		case windowLockName, windowStartName, windowStartSealName, windowRecoverName, windowRecoverSealName:
			names[entry.Name()] = true
		default:
			return nil, ErrWindowUnknown
		}
	}
	return names, nil
}
func readWindowFile(fd int, name string, owner uint32) (saved windowSavedFile, err error) {
	n, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return saved, ErrWindowUnknown
	}
	f := os.NewFile(uintptr(n), "maintenance-window-record")
	defer func() {
		if f.Close() != nil {
			err = ErrWindowUnknown
		}
	}()
	var before, after, linked unix.Stat_t
	if unix.Fstat(n, &before) != nil || !challengeFileValid(before, owner) || before.Size < 1 || before.Size > 4096 {
		return saved, ErrWindowUnknown
	}
	raw, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil || int64(len(raw)) != before.Size || unix.Fstat(n, &after) != nil || windowFileStamp(before) != windowFileStamp(after) || unix.Fstatat(fd, name, &linked, unix.AT_SYMLINK_NOFOLLOW) != nil || windowFileStamp(after) != windowFileStamp(linked) {
		return saved, ErrWindowUnknown
	}
	return windowSavedFile{challengeFile{name, after}, raw, windowFileStamp(after)}, nil
}
func createWindowFile(w *MaintenanceWindow, name, stage string, build func(windowIdentity) any) (saved windowSavedFile, err error) {
	dir := w.dirs[len(w.dirs)-1].fd
	fd, e := unix.Openat(dir, name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if errors.Is(e, unix.EEXIST) {
		return saved, ErrWindowExists
	}
	if e != nil {
		return saved, ErrWindowUnknown
	}
	f := os.NewFile(uintptr(fd), "maintenance-window-record")
	defer func() {
		if f.Close() != nil {
			err = ErrWindowUnknown
		}
	}()
	var before, after unix.Stat_t
	if unix.Fstat(fd, &before) != nil || !challengeFileValid(before, w.opts.owner) || windowHook(w.opts, stage+"_created") != nil {
		return saved, ErrWindowUnknown
	}
	raw, e := windowMarshal(build(windowInode(before)))
	if e != nil {
		return saved, e
	}
	if n, e := f.Write(raw); e != nil || n != len(raw) || f.Sync() != nil || unix.Fstat(fd, &after) != nil || !challengeSameInode(before, after) || !challengeFileValid(after, w.opts.owner) || after.Size != int64(len(raw)) {
		return saved, ErrWindowUnknown
	}
	saved = windowSavedFile{challengeFile{name, after}, raw, windowFileStamp(after)}
	if updateChallengeDirAfterOwnFile(w.dirs, saved.file, w.opts.owner) != nil || unix.Fsync(dir) != nil || windowHook(w.opts, stage+"_durable") != nil || checkChallengeDirs(w.dirs, w.opts.owner) != nil {
		return saved, ErrWindowUnknown
	}
	actual, e := readWindowFile(dir, name, w.opts.owner)
	if e != nil || actual.stamp != saved.stamp || !bytes.Equal(actual.raw, saved.raw) {
		return saved, ErrWindowUnknown
	}
	return saved, nil
}
func newWindow(dirs []challengeDir, b WindowBinding, opts windowOptions) *MaintenanceWindow {
	done, cancel := context.WithCancel(context.Background())
	w := &MaintenanceWindow{dirs: dirs, lockFD: -1, binding: b, opts: opts, files: map[string]windowSavedFile{}, done: done, cancel: cancel}
	w.self = w
	return w
}
func (w *MaintenanceWindow) acquireLock(existing bool) error {
	dir := w.dirs[len(w.dirs)-1].fd
	if existing {
		saved, e := readWindowFile(dir, windowLockName, w.opts.owner)
		if e != nil {
			return e
		}
		var record windowLockRecord
		if windowDecode(saved.raw, &record) != nil || record.Protocol != windowProtocol || record.Kind != "directory_lease" || record.Binding != w.binding || record.Directory != windowDirectory(w.dirs) || record.Self != saved.stamp.Identity {
			return ErrWindowUnknown
		}
		w.files[windowLockName] = saved
	} else {
		saved, e := createWindowFile(w, windowLockName, "lease", func(id windowIdentity) any {
			return windowLockRecord{windowProtocol, "directory_lease", w.binding, windowDirectory(w.dirs), id}
		})
		if e != nil {
			return e
		}
		w.files[windowLockName] = saved
	}
	fd, e := unix.Openat(dir, windowLockName, unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrWindowUnknown
	}
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || windowFileStamp(st) != w.files[windowLockName].stamp {
		_ = unix.Close(fd)
		return ErrWindowUnknown
	}
	if e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); e != nil {
		_ = unix.Close(fd)
		if errors.Is(e, unix.EWOULDBLOCK) || errors.Is(e, unix.EAGAIN) {
			return ErrWindowBusy
		}
		return ErrWindowUnknown
	}
	w.lockFD = fd
	return nil
}
func startMaintenanceWindow(ctx context.Context, dir string, b WindowBinding, opts windowOptions) (out *MaintenanceWindow, err error) {
	if err = windowInput(ctx, b, opts); err != nil {
		return nil, err
	}
	dirs, e := openChallengeDirs(dir, challengeOptions{owner: opts.owner, anchor: opts.anchor})
	if e != nil {
		return nil, ErrWindowStore
	}
	w := newWindow(dirs, b, opts)
	defer func() {
		if out == nil && w.Close() != nil {
			err = ErrWindowUnknown
		}
	}()
	names, e := windowNames(dirs[len(dirs)-1].fd)
	if e != nil {
		return nil, e
	}
	if len(names) != 0 {
		return nil, ErrWindowExists
	}
	if e = w.acquireLock(false); e != nil {
		return nil, e
	}
	sample, e := w.opts.clock()
	if e != nil || !windowClockValid(sample) || sample.nanos > math.MaxInt64-windowTotalNanos {
		return nil, ErrWindowClock
	}
	saved, e := createWindowFile(w, windowStartName, "start", func(id windowIdentity) any {
		return windowStartRecord{windowProtocol, "start", b, windowDirectory(w.dirs), w.files[windowLockName].stamp, id, opts.clockSource, sample.bootID, sample.nanos, sample.nanos + windowForwardNanos, sample.nanos + windowTotalNanos, sample.utc.UTC().Format(time.RFC3339Nano)}
	})
	if e != nil {
		return nil, e
	}
	w.files[windowStartName] = saved
	if windowDecode(saved.raw, &w.start) != nil {
		return nil, ErrWindowUnknown
	}
	seal, e := w.createSeal(windowStartSealName, "start_committed", saved)
	if e != nil {
		return nil, e
	}
	w.files[windowStartSealName] = seal
	if _, e = w.validateLocked(ctx); e != nil {
		return nil, e
	}
	return w, nil
}
func (w *MaintenanceWindow) createSeal(name, kind string, primary windowSavedFile) (windowSavedFile, error) {
	return createWindowFile(w, name, kind, func(id windowIdentity) any {
		return windowSeal{windowProtocol, kind, windowBindingHash(w.binding), digest(primary.raw), primary.stamp, id}
	})
}
func openMaintenanceWindow(ctx context.Context, dir string, b WindowBinding, opts windowOptions) (out *MaintenanceWindow, err error) {
	if err = windowInput(ctx, b, opts); err != nil {
		return nil, err
	}
	dirs, e := openChallengeDirs(dir, challengeOptions{owner: opts.owner, anchor: opts.anchor})
	if e != nil {
		return nil, ErrWindowStore
	}
	w := newWindow(dirs, b, opts)
	defer func() {
		if out == nil && w.Close() != nil {
			err = ErrWindowUnknown
		}
	}()
	if e = w.acquireLock(true); e != nil {
		return nil, e
	}
	for _, name := range []string{windowStartName, windowStartSealName} {
		saved, e := readWindowFile(dirs[len(dirs)-1].fd, name, opts.owner)
		if e != nil {
			return nil, e
		}
		w.files[name] = saved
	}
	if windowDecode(w.files[windowStartName].raw, &w.start) != nil || w.start.Protocol != windowProtocol || w.start.Kind != "start" || w.start.Binding != b || w.start.ClockSource != opts.clockSource || w.start.Directory != windowDirectory(dirs) || w.start.Lock != w.files[windowLockName].stamp || w.start.Self != w.files[windowStartName].stamp.Identity || !windowStartValid(w.start) || w.verifySeal(windowStartSealName, "start_committed", windowStartName) != nil {
		return nil, ErrWindowUnknown
	}
	names, e := windowNames(dirs[len(dirs)-1].fd)
	if e != nil {
		return nil, e
	}
	if names[windowRecoverName] != names[windowRecoverSealName] {
		return nil, ErrWindowUnknown
	}
	if names[windowRecoverName] {
		for _, name := range []string{windowRecoverName, windowRecoverSealName} {
			saved, e := readWindowFile(dirs[len(dirs)-1].fd, name, opts.owner)
			if e != nil {
				return nil, e
			}
			w.files[name] = saved
		}
		var recovery windowRecoveryRecord
		if windowDecode(w.files[windowRecoverName].raw, &recovery) != nil || !w.recoveryValid(recovery) || w.verifySeal(windowRecoverSealName, "recovery_committed", windowRecoverName) != nil {
			return nil, ErrWindowUnknown
		}
		w.recovery = &recovery
	}
	sample, e := w.validateLocked(ctx)
	if e != nil {
		return nil, e
	}
	limit := w.start.WindowDeadline
	if w.recovery != nil {
		limit = w.recovery.DeadlineNanos
	}
	if sample.nanos >= limit {
		return nil, ErrWindowExpired
	}
	return w, nil
}
func windowClockValid(s windowClockSample) bool {
	return windowBootIDValid(s.bootID) && s.nanos > 0 && !s.utc.IsZero()
}
func windowStartValid(s windowStartRecord) bool {
	t, e := time.Parse(time.RFC3339Nano, s.ObservedUTC)
	return e == nil && t.UTC().Format(time.RFC3339Nano) == s.ObservedUTC && windowBootIDValid(s.BootID) && s.StartedNanos > 0 && s.StartedNanos <= math.MaxInt64-windowTotalNanos && s.ForwardDeadline == s.StartedNanos+windowForwardNanos && s.WindowDeadline == s.StartedNanos+windowTotalNanos
}
func (w *MaintenanceWindow) recoveryValid(r windowRecoveryRecord) bool {
	t, e := time.Parse(time.RFC3339Nano, r.ObservedUTC)
	if e != nil || t.UTC().Format(time.RFC3339Nano) != r.ObservedUTC || r.Protocol != windowProtocol || r.Kind != "recovery" || r.BindingSHA256 != windowBindingHash(w.binding) || r.StartSHA256 != digest(w.files[windowStartName].raw) || r.Self != w.files[windowRecoverName].stamp.Identity || r.BootID != w.start.BootID || r.FirstRecoveryNanos < w.start.StartedNanos || r.FirstRecoveryNanos >= w.start.WindowDeadline || r.FirstRecoveryNanos > math.MaxInt64-windowRecoveryNanos {
		return false
	}
	return r.DeadlineNanos == min(w.start.WindowDeadline, r.FirstRecoveryNanos+windowRecoveryNanos)
}
func (w *MaintenanceWindow) verifySeal(name, kind, primary string) error {
	var seal windowSeal
	saved := w.files[name]
	original := w.files[primary]
	if windowDecode(saved.raw, &seal) != nil || seal.Protocol != windowProtocol || seal.Kind != kind || seal.BindingSHA256 != windowBindingHash(w.binding) || seal.PrimarySHA256 != digest(original.raw) || seal.PrimaryStamp != original.stamp || seal.Self != saved.stamp.Identity {
		return ErrWindowUnknown
	}
	return nil
}
func (w *MaintenanceWindow) validateLocked(ctx context.Context) (windowClockSample, error) {
	if ctx == nil || ctx.Err() != nil {
		return windowClockSample{}, ErrWindowBinding
	}
	if w.self != w || w.closed || len(w.dirs) == 0 || w.lockFD < 0 || w.opts.clock == nil || w.done == nil || w.start.Binding != w.binding {
		return windowClockSample{}, ErrWindowClosed
	}
	if checkChallengeDirs(w.dirs, w.opts.owner) != nil {
		return windowClockSample{}, ErrWindowStore
	}
	names, e := windowNames(w.dirs[len(w.dirs)-1].fd)
	if e != nil || len(names) != len(w.files) {
		return windowClockSample{}, ErrWindowUnknown
	}
	for name, saved := range w.files {
		if !names[name] || readChallengeFile(w.dirs[len(w.dirs)-1].fd, saved.file, saved.raw, w.opts.owner) != nil {
			return windowClockSample{}, ErrWindowUnknown
		}
		actual, e := readWindowFile(w.dirs[len(w.dirs)-1].fd, name, w.opts.owner)
		if e != nil || actual.stamp != saved.stamp {
			return windowClockSample{}, ErrWindowUnknown
		}
	}
	sample, e := w.opts.clock()
	if e != nil || !windowClockValid(sample) || sample.bootID != w.start.BootID || sample.nanos < w.start.StartedNanos || sample.nanos < w.lastNanos {
		return windowClockSample{}, ErrWindowClock
	}
	w.lastNanos = sample.nanos
	return sample, nil
}

// ForwardContext cannot be obtained after recovery has started or after the
// original twenty-minute boundary. Parent cancellation is always retained.
func (w *MaintenanceWindow) ForwardContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if w == nil || w.self != w {
		return nil, nil, ErrWindowClosed
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	sample, e := w.validateLocked(ctx)
	if e != nil {
		return nil, nil, e
	}
	if w.recovery != nil || sample.nanos >= w.start.ForwardDeadline {
		return nil, nil, ErrWindowExpired
	}
	return w.boundContext(ctx, w.start.ForwardDeadline, sample, true)
}

// RecoveryContext seals the first recovery instant once. Every later attempt
// shares min(original start+30m, first recovery+10m), never a new now+10m.
func (w *MaintenanceWindow) RecoveryContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if w == nil || w.self != w {
		return nil, nil, ErrWindowClosed
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	sample, e := w.validateLocked(ctx)
	if e != nil {
		return nil, nil, e
	}
	if sample.nanos >= w.start.WindowDeadline {
		return nil, nil, ErrWindowExpired
	}
	if w.recovery == nil {
		if sample.nanos > math.MaxInt64-windowRecoveryNanos {
			return nil, nil, ErrWindowClock
		}
		saved, e := createWindowFile(w, windowRecoverName, "recovery", func(id windowIdentity) any {
			return windowRecoveryRecord{windowProtocol, "recovery", windowBindingHash(w.binding), digest(w.files[windowStartName].raw), id, sample.bootID, sample.nanos, min(w.start.WindowDeadline, sample.nanos+windowRecoveryNanos), sample.utc.UTC().Format(time.RFC3339Nano)}
		})
		if e != nil {
			return nil, nil, e
		}
		w.files[windowRecoverName] = saved
		var recovery windowRecoveryRecord
		if windowDecode(saved.raw, &recovery) != nil || !w.recoveryValid(recovery) {
			return nil, nil, ErrWindowUnknown
		}
		seal, e := w.createSeal(windowRecoverSealName, "recovery_committed", saved)
		if e != nil {
			return nil, nil, e
		}
		w.files[windowRecoverSealName] = seal
		w.recovery = &recovery
		sample, e = w.validateLocked(ctx)
		if e != nil {
			return nil, nil, e
		}
	}
	if sample.nanos >= w.recovery.DeadlineNanos {
		return nil, nil, ErrWindowExpired
	}
	return w.boundContext(ctx, w.recovery.DeadlineNanos, sample, false)
}
func (w *MaintenanceWindow) boundContext(parent context.Context, limit int64, sample windowClockSample, forward bool) (context.Context, context.CancelFunc, error) {
	if sample.nanos >= limit {
		return nil, nil, ErrWindowExpired
	}
	timed, timerCancel := context.WithDeadline(parent, time.Now().Add(time.Duration(limit-sample.nanos)))
	q, cancelCause := context.WithCancelCause(timed)
	cancel := func() { cancelCause(context.Canceled); timerCancel() }
	// Go timers need not include suspend time. Independently re-read the Linux
	// boot clock after scheduling/resume and cancel if the real budget expired.
	go func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		defer timerCancel()
		for {
			select {
			case <-q.Done():
				return
			case <-w.done.Done():
				cancelCause(ErrWindowClosed)
				return
			case <-ticker.C:
				w.mu.Lock()
				now, e := w.validateLocked(q)
				phaseChanged := forward && w.recovery != nil
				w.mu.Unlock()
				if e != nil {
					cancelCause(e)
					return
				}
				if now.nanos >= limit || phaseChanged {
					cancelCause(ErrWindowExpired)
					return
				}
			}
		}
	}()
	return q, cancel, nil
}

type WindowBudgetReceipt struct {
	Protocol               string        `json:"protocol"`
	Binding                WindowBinding `json:"binding"`
	BudgetOnly             bool          `json:"budget_only"`
	DirectoryLeaseHeld     bool          `json:"directory_lease_held"`
	WholeWriterFence       bool          `json:"whole_writer_fence"`
	MutationAllowed        bool          `json:"mutation_allowed"`
	DropReady              bool          `json:"drop_ready"`
	RecoveryBudgetMeasured bool          `json:"recovery_budget_measured"`
	ClockSource            string        `json:"clock_source"`
	StartSHA256            string        `json:"start_sha256"`
	RecoverySHA256         string        `json:"recovery_sha256"`
	RemainingMilliseconds  int64         `json:"remaining_milliseconds"`
}

func (w *MaintenanceWindow) Diagnostic(ctx context.Context) (WindowBudgetReceipt, error) {
	if w == nil || w.self != w {
		return WindowBudgetReceipt{}, ErrWindowClosed
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	sample, e := w.validateLocked(ctx)
	if e != nil {
		return WindowBudgetReceipt{}, e
	}
	limit := w.start.WindowDeadline
	recoveryHash := ""
	if w.recovery != nil {
		limit = w.recovery.DeadlineNanos
		recoveryHash = digest(w.files[windowRecoverName].raw)
	}
	return WindowBudgetReceipt{Protocol: windowProtocol, Binding: w.binding, BudgetOnly: true, DirectoryLeaseHeld: true, ClockSource: w.opts.clockSource, StartSHA256: digest(w.files[windowStartName].raw), RecoverySHA256: recoveryHash, RemainingMilliseconds: max(0, limit-sample.nanos) / int64(time.Millisecond)}, nil
}
func (w *MaintenanceWindow) Close() error {
	if w == nil || w.self == nil {
		return nil
	}
	if w.self != w {
		return ErrWindowClosed
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.cancel != nil {
		w.cancel()
	}
	var failure bool
	if w.lockFD >= 0 {
		if unix.Flock(w.lockFD, unix.LOCK_UN) != nil {
			failure = true
		}
		if unix.Close(w.lockFD) != nil {
			failure = true
		}
		w.lockFD = -1
	}
	for i := len(w.dirs) - 1; i >= 0; i-- {
		if unix.Close(w.dirs[i].fd) != nil {
			failure = true
		}
	}
	if failure {
		return ErrWindowUnknown
	}
	return nil
}
