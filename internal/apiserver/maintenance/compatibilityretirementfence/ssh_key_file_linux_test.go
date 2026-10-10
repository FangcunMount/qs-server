//go:build linux

package compatibilityretirementfence

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/pkg/version"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"
)

func sshKeyTestExchange(dir int, a, b string) error {
	return unix.Renameat2(dir, a, dir, b, unix.RENAME_EXCHANGE)
}
func sshKeyTestMetadata(fd int) error { return sshKeyNoExtendedMetadata(fd) }

const sshKeyNativeParent = "/var/lib/qs-retirement-keyfile-ci"

type sshKeyNativeRegistry struct {
	dirs   map[string]windowIdentity
	files  map[string]windowSavedFile
	retain bool
}

type sshKeyNativeFixture struct {
	root, operation, keys string
	prep                  *SSHPreparation
	window                *MaintenanceWindow
	lease                 *SSHKeyFileLease
	registry              *sshKeyNativeRegistry
}

// This test is mandatory when selected by CI. It uses the public Linux/root
// constructors and the actual kernel boot clock; private owner, clock,
// metadata, exchange and hook injection are not used in the native path.
// The generated key is stored only in an isolated fixture, never an account's
// SSH files. No sshd, full writer fence or DROP capability is produced.
func TestSSHKeyFileActualLinuxRootPublic(t *testing.T) {
	enabled, required := os.Getenv("QS_SSH_KEY_FILE_NATIVE"), os.Getenv("QS_SSH_KEY_FILE_NATIVE_REQUIRED")
	if enabled == "" && required == "" {
		t.Skip("actual Linux root key-file fixture not requested")
	}
	if enabled != "1" || required != "1" || os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Fatal("native key-file test requires the explicit mandatory Linux/root selection")
	}
	source := os.Getenv("QS_SSH_KEY_FILE_SOURCE_SHA")
	operation := os.Getenv("QS_SSH_KEY_FILE_RUN_ID") + "-" + os.Getenv("QS_SSH_KEY_FILE_RUN_ATTEMPT")
	approvedBinary := os.Getenv("QS_SSH_KEY_FILE_BINARY_SHA256")
	if !sha40.MatchString(source) || version.GitCommit != source || !operationID.MatchString(operation) || !sha64.MatchString(approvedBinary) {
		t.Fatal("native source/run/binary input binding rejected")
	}
	executable, e := os.Executable()
	if e != nil {
		t.Fatal("native executable identity unavailable")
	}
	actualBinary, e := os.ReadFile(executable)
	if e != nil || digest(actualBinary) != approvedBinary {
		t.Fatal("native executable bytes do not match selected test binary")
	}
	registry := &sshKeyNativeRegistry{dirs: map[string]windowIdentity{}, files: map[string]windowSavedFile{}}
	root := filepath.Join(sshKeyNativeParent, operation)
	// An existing parent must already be protected. No existing directory owner
	// or mode is changed in order to satisfy the production gates.
	t.Cleanup(func() { sshKeyNativeCleanup(t, registry) })
	sshKeyNativeMakeDirectory(t, registry, sshKeyNativeParent, true)
	sshKeyNativeMakeDirectory(t, registry, root, false)
	for _, kind := range []string{"exact_restore", "close_preserves", "xattr", "acl", "owner", "nlink"} {
		t.Run(kind, func(t *testing.T) {
			f := sshKeyNativeCreate(t, registry, filepath.Join(root, kind), source, operation)
			t.Cleanup(func() {
				if f.lease != nil && f.lease.Close() != nil {
					registry.retain = true
					t.Error("native key-file descriptor close failed; fixture retained")
				}
			})
			switch kind {
			case "xattr", "acl", "owner", "nlink":
				sshKeyNativeRejectedAttributes(t, f, kind)
				return
			}
			l, e := OpenSSHKeyFileLease(context.Background(), f.operation, f.keys, f.prep, f.window)
			if e != nil {
				t.Fatal("public native key-file open rejected", e)
			}
			f.lease = l
			initial, e := f.window.Diagnostic(context.Background())
			if e != nil || initial.ClockSource != "linux_clock_boottime_v1" || !initial.BudgetOnly || initial.WholeWriterFence || initial.DropReady || initial.Binding.SourceSHA != source || initial.Binding.OperationID != operation || !sha64.MatchString(initial.StartSHA256) || initial.RecoverySHA256 != "" {
				t.Fatal("native original window/source binding unavailable")
			}
			original := sshKeyStat(t, f.keys)
			knownMutation := false
			t.Cleanup(func() {
				if !knownMutation {
					registry.retain = true
				}
			})
			installed, e := l.Install(context.Background())
			if e != nil || !installed.ExactFileInstalled || installed.StateUnknown || installed.WindowStartSHA256 != initial.StartSHA256 {
				t.Fatal("public native install did not reach known terminal", e)
			}
			sshKeyNoAuthority(t, installed)
			slot := filepath.Join(filepath.Dir(f.keys), ".qs-retirement-"+operation+".original")
			active := sshKeyStat(t, f.keys)
			if active.Identity == original.Identity || sshKeyStat(t, slot).Identity != original.Identity || !bytes.Equal(sshKeyRead(t, slot), f.prep.original) || !bytes.Equal(sshKeyRead(t, f.keys), f.prep.restricted) {
				t.Fatal("native exchange did not retain the actual original inode")
			}
			sshKeyNativeCaptureOwned(t, registry, f.keys, f.prep.restricted)
			sshKeyNativeCaptureOwned(t, registry, slot, f.prep.original)
			sshKeyNativeCaptureRecords(t, f)
			knownMutation = true
			if kind == "exact_restore" {
				knownMutation = false
				restored, e := l.Restore(context.Background())
				if e != nil || !restored.ExactOriginalRestored || restored.ExactFileInstalled || restored.StateUnknown || restored.WindowStartSHA256 != initial.StartSHA256 {
					t.Fatal("public native restore did not reach known terminal", e)
				}
				sshKeyNoAuthority(t, restored)
				final := sshKeyStat(t, f.keys)
				if final.Identity != original.Identity || final.Mode != original.Mode || final.Owner != original.Owner || final.Group != original.Group || final.Links != original.Links || final.Size != original.Size || final.ModifiedNanos != original.ModifiedNanos || !bytes.Equal(sshKeyRead(t, f.keys), f.prep.original) || sshKeyStat(t, slot).Identity != active.Identity {
					t.Fatal("native restore did not return the original inode and facts")
				}
				diagnostic, e := f.window.Diagnostic(context.Background())
				if e != nil || diagnostic.ClockSource != "linux_clock_boottime_v1" || !diagnostic.BudgetOnly || diagnostic.WholeWriterFence || diagnostic.DropReady || diagnostic.StartSHA256 != initial.StartSHA256 || diagnostic.Binding != initial.Binding || !sha64.MatchString(diagnostic.RecoverySHA256) || diagnostic.RemainingMilliseconds <= 0 || diagnostic.RemainingMilliseconds > 600000 {
					t.Fatal("native original boot-clock window not preserved")
				}
				sshKeyNativeCaptureOwned(t, registry, f.keys, f.prep.original)
				sshKeyNativeCaptureOwned(t, registry, slot, f.prep.restricted)
				sshKeyNativeCaptureRecords(t, f)
				knownMutation = true
			}
			// Close must not implicitly restore, delete either inode, or delete any
			// durable record. Read back the complete registered file facts after it.
			if e := l.Close(); e != nil {
				registry.retain = true
				t.Fatal("native key-file close failed")
			}
			f.lease = nil
			if sshKeyNativeVerifyFiles(registry) != nil {
				registry.retain = true
				t.Fatal("close changed native files or durable records")
			}
			t.Log("native public key-file terminal known; original inode and records independently read back; writer/drop flags false")
		})
	}
	if registry.retain {
		t.Fatal("native fixture has an unknown result; all material retained")
	}
}

func sshKeyNativeCreate(t *testing.T, registry *sshKeyNativeRegistry, root, source, operation string) *sshKeyNativeFixture {
	t.Helper()
	for _, p := range []string{root, filepath.Join(root, "keys"), filepath.Join(root, "records"), filepath.Join(root, "records", operation), filepath.Join(root, "window")} {
		sshKeyNativeMakeDirectory(t, registry, p, false)
	}
	public, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal("native fixture key generation failed")
	}
	key, e := ssh.NewPublicKey(public)
	if e != nil {
		t.Fatal("native fixture public key rejected")
	}
	original := append([]byte("# isolated native CI fixture\ncommand=\"bash -s\" "), ssh.MarshalAuthorizedKey(key)...)
	prep, e := PrepareSSH(original, digest(original), "/opt/qs-fence/gate", "/etc/qs-fence/policy.json", strings.Repeat("a", 64), "/etc/qs-fence/api-read-token", []string{ssh.FingerprintSHA256(key)})
	if e != nil {
		t.Fatal("native isolated SSH preparation rejected")
	}
	keys := filepath.Join(root, "keys", "authorized_keys.fixture")
	fd, e := unix.Open(keys, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		t.Fatal("native fixture key file creation refused")
	}
	n, writeErr := unix.Write(fd, original)
	syncErr := unix.Fsync(fd)
	closeErr := unix.Close(fd)
	if writeErr != nil || n != len(original) || syncErr != nil || closeErr != nil {
		registry.retain = true
		t.Fatal("native fixture key creation result unknown")
	}
	sshKeyNativeCaptureOwned(t, registry, keys, original)
	b := WindowBinding{MaintenanceWindowTargetSHA256(), source, operation, digest([]byte("isolated-native-key-file/" + source + "/" + operation)), operation}
	w, e := StartMaintenanceWindow(context.Background(), filepath.Join(root, "window"), b)
	if e != nil {
		registry.retain = true
		t.Fatal("public native window start failed; fixture retained", e)
	}
	t.Cleanup(func() {
		if w.Close() != nil {
			registry.retain = true
			t.Error("native window close failed; fixture retained")
		}
	})
	f := &sshKeyNativeFixture{root, filepath.Join(root, "records", operation), keys, prep, w, nil, registry}
	sshKeyNativeCaptureRecords(t, f)
	return f
}

func sshKeyNativeRejectedAttributes(t *testing.T, f *sshKeyNativeFixture, kind string) {
	t.Helper()
	fd, e := unix.Open(f.keys, unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		t.Fatal("native attribute fixture open failed")
	}
	var mutateErr error
	switch kind {
	case "xattr":
		mutateErr = unix.Fsetxattr(fd, "user.qs_keyfile_native", []byte("fixture-only"), 0)
		if mutateErr == nil {
			n, e := unix.Fgetxattr(fd, "user.qs_keyfile_native", nil)
			if e != nil || n != len("fixture-only") {
				mutateErr = ErrSSHKeyFile
			}
		}
	case "acl":
		// Actual Linux POSIX access ACL: owner rw, a named UID r, group r,
		// mask r, other r. This is a real extended ACL, not an injected metadata
		// validator. The named fixture UID does not require creating an account.
		acl := make([]byte, 4+5*8)
		binary.LittleEndian.PutUint32(acl, 2)
		entries := [][3]uint32{{1, 6, 0xffffffff}, {2, 4, 12345}, {4, 4, 0xffffffff}, {16, 4, 0xffffffff}, {32, 4, 0xffffffff}}
		for i, v := range entries {
			off := 4 + i*8
			binary.LittleEndian.PutUint16(acl[off:], uint16(v[0]))
			binary.LittleEndian.PutUint16(acl[off+2:], uint16(v[1]))
			binary.LittleEndian.PutUint32(acl[off+4:], v[2])
		}
		mutateErr = unix.Fsetxattr(fd, "system.posix_acl_access", acl, 0)
		if mutateErr == nil {
			n, e := unix.Fgetxattr(fd, "system.posix_acl_access", nil)
			if e != nil || n <= 4 {
				mutateErr = ErrSSHKeyFile
			}
		}
	case "owner":
		mutateErr = unix.Fchown(fd, 1, 0)
	case "nlink":
		mutateErr = unix.Link(f.keys, f.keys+".owned-link")
	}
	closeErr := unix.Close(fd)
	if mutateErr != nil || closeErr != nil {
		f.registry.retain = true
		t.Fatal("actual Linux attribute fixture could not be produced; no mocked substitute")
	}
	sshKeyNativeCaptureOwned(t, f.registry, f.keys, f.prep.original)
	if kind == "nlink" {
		sshKeyNativeCaptureOwned(t, f.registry, f.keys+".owned-link", f.prep.original)
	}
	l, e := OpenSSHKeyFileLease(context.Background(), f.operation, f.keys, f.prep, f.window)
	if e == nil || l != nil {
		f.lease = l
		f.registry.retain = true
		t.Fatal("public native open accepted actual unsafe file attribute")
	}
	names, e := os.ReadDir(f.operation)
	if e != nil || len(names) != 0 || sshKeyNativeVerifyFiles(f.registry) != nil {
		f.registry.retain = true
		t.Fatal("native attribute rejection mutated files or reserved an operation")
	}
	if kind == "nlink" {
		// Remove only the precise link just created by this case, then register
		// the original's known own-unlink metadata before generic fixture cleanup.
		path := f.keys + ".owned-link"
		if sshKeyNativeUnlink(path, f.registry.files[path]) != nil {
			f.registry.retain = true
			t.Fatal("native owned link cleanup refused")
		}
		delete(f.registry.files, path)
		sshKeyNativeCaptureOwned(t, f.registry, f.keys, f.prep.original)
	}
	t.Log("actual Linux unsafe attribute refused by public native constructor; original body and operation absence read back")
}

func sshKeyNativeProtectedDirectory(path string) (int, error) {
	if filepath.Clean(path) != path || !filepath.IsAbs(path) {
		return -1, ErrSSHKeyFile
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return -1, e
	}
	check := func(fd int) bool {
		var st unix.Stat_t
		return unix.Fstat(fd, &st) == nil && st.Mode&unix.S_IFMT == unix.S_IFDIR && st.Uid == 0 && st.Nlink > 0 && st.Mode&0022 == 0
	}
	if !check(fd) {
		_ = unix.Close(fd)
		return -1, ErrSSHKeyFile
	}
	for _, name := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if name == "" {
			continue
		}
		child, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		closeErr := unix.Close(fd)
		if e != nil || closeErr != nil || !check(child) {
			if child >= 0 {
				_ = unix.Close(child)
			}
			return -1, ErrSSHKeyFile
		}
		fd = child
	}
	return fd, nil
}
func sshKeyNativeMakeDirectory(t *testing.T, r *sshKeyNativeRegistry, path string, allowExisting bool) {
	t.Helper()
	parent, e := sshKeyNativeProtectedDirectory(filepath.Dir(path))
	if e != nil {
		t.Fatal("native fixture ancestor not already root protected")
	}
	e = unix.Mkdirat(parent, filepath.Base(path), 0700)
	created := e == nil
	if e != nil && (!allowExisting || !errors.Is(e, unix.EEXIST)) {
		_ = unix.Close(parent)
		t.Fatal("native exclusive fixture directory refused")
	}
	var st unix.Stat_t
	statErr := unix.Fstatat(parent, filepath.Base(path), &st, unix.AT_SYMLINK_NOFOLLOW)
	closeErr := unix.Close(parent)
	if statErr != nil || closeErr != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != 0 || st.Gid != 0 || st.Mode&07777 != 0700 {
		t.Fatal("native fixture directory identity/owner/mode rejected")
	}
	if created {
		r.dirs[path] = windowInode(st)
	}
}
func sshKeyNativeCaptureOwned(t *testing.T, r *sshKeyNativeRegistry, path string, expected []byte) {
	t.Helper()
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		r.retain = true
		t.Fatal("native owned file readback open failed")
	}
	var before, after unix.Stat_t
	read := make([]byte, len(expected)+1)
	statErr := unix.Fstat(fd, &before)
	n, readErr := unix.Pread(fd, read, 0)
	afterErr := unix.Fstat(fd, &after)
	closeErr := unix.Close(fd)
	if statErr != nil || afterErr != nil || closeErr != nil || readErr != nil || n != len(expected) || !bytes.Equal(read[:n], expected) || windowFileStamp(before) != windowFileStamp(after) || before.Mode&unix.S_IFMT != unix.S_IFREG {
		r.retain = true
		t.Fatal("native owned file readback changed")
	}
	r.files[path] = windowSavedFile{stamp: windowFileStamp(before), raw: bytes.Clone(expected)}
}
func sshKeyNativeCaptureRecords(t *testing.T, f *sshKeyNativeFixture) {
	t.Helper()
	// These records were produced by the original live native objects, not
	// imported as authority. Access is only for exact fixture cleanup/readback.
	for name, saved := range f.window.files {
		sshKeyNativeCaptureOwned(t, f.registry, filepath.Join(f.root, "window", name), saved.raw)
	}
	if f.lease != nil {
		for name, saved := range f.lease.records {
			sshKeyNativeCaptureOwned(t, f.registry, filepath.Join(f.operation, name), saved.raw)
		}
	}
}
func sshKeyNativeVerifyFiles(r *sshKeyNativeRegistry) error {
	for path, saved := range r.files {
		var st unix.Stat_t
		if unix.Lstat(path, &st) != nil || windowFileStamp(st) != saved.stamp {
			return ErrChanged
		}
		raw, e := os.ReadFile(path)
		if e != nil || !bytes.Equal(raw, saved.raw) {
			return ErrChanged
		}
	}
	return nil
}
func sshKeyNativeUnlink(path string, saved windowSavedFile) error {
	parent, e := sshKeyNativeProtectedDirectory(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer func() { _ = unix.Close(parent) }()
	var st unix.Stat_t
	if unix.Fstatat(parent, filepath.Base(path), &st, unix.AT_SYMLINK_NOFOLLOW) != nil || windowFileStamp(st) != saved.stamp {
		return ErrChanged
	}
	return unix.Unlinkat(parent, filepath.Base(path), 0)
}
func sshKeyNativeCleanup(t *testing.T, r *sshKeyNativeRegistry) {
	t.Helper()
	if r.retain {
		t.Log("native fixture unknown: exact files and intents retained; no cleanup attempted")
		return
	}
	if sshKeyNativeVerifyFiles(r) != nil {
		r.retain = true
		t.Error("native fixture readback drifted; no cleanup attempted")
		return
	}
	// Validate the full catalog before deleting the first byte. Unregistered
	// entries, changed directory inodes, and symlinks preserve the entire fixture.
	for path, id := range r.dirs {
		fd, e := sshKeyNativeProtectedDirectory(path)
		if e != nil {
			r.retain = true
			t.Error("native cleanup protected directory rejected")
			return
		}
		var st unix.Stat_t
		statErr := unix.Fstat(fd, &st)
		f := os.NewFile(uintptr(fd), "native-owned-directory")
		names, readErr := f.Readdirnames(-1)
		closeErr := f.Close()
		if statErr != nil || readErr != nil || closeErr != nil || windowInode(st) != id || st.Mode&07777 != 0700 || st.Gid != 0 {
			r.retain = true
			t.Error("native cleanup directory drifted")
			return
		}
		for _, name := range names {
			child := filepath.Join(path, name)
			_, file := r.files[child]
			_, dir := r.dirs[child]
			if !file && !dir {
				r.retain = true
				t.Error("native cleanup found an unregistered entry; fixture retained")
				return
			}
		}
	}
	files := make([]string, 0, len(r.files))
	for path := range r.files {
		files = append(files, path)
	}
	sort.Strings(files)
	for _, path := range files {
		if sshKeyNativeUnlink(path, r.files[path]) != nil {
			r.retain = true
			t.Error("native exact file cleanup failed; remaining fixture retained")
			return
		}
	}
	dirs := make([]string, 0, len(r.dirs))
	for path := range r.dirs {
		dirs = append(dirs, path)
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, path := range dirs {
		parent, e := sshKeyNativeProtectedDirectory(filepath.Dir(path))
		if e != nil {
			r.retain = true
			t.Error("native cleanup parent rejected")
			return
		}
		var st unix.Stat_t
		statErr := unix.Fstatat(parent, filepath.Base(path), &st, unix.AT_SYMLINK_NOFOLLOW)
		unlinkErr := error(nil)
		if statErr != nil || windowInode(st) != r.dirs[path] || st.Mode&07777 != 0700 || st.Uid != 0 || st.Gid != 0 {
			unlinkErr = ErrChanged
		} else {
			unlinkErr = unix.Unlinkat(parent, filepath.Base(path), unix.AT_REMOVEDIR)
		}
		closeErr := unix.Close(parent)
		if unlinkErr != nil || closeErr != nil {
			r.retain = true
			t.Error("native exact directory cleanup failed; remaining fixture retained")
			return
		}
	}
	t.Log("native fixture exact owned-file/inode catalog removed; no account keys, SSH service, backup purge or database operation performed")
}
