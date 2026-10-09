package compatibilityretirementfence

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"
)

type sshKeyFixture struct {
	operation, keys string
	prep            *SSHPreparation
	window          *MaintenanceWindow
	clock           *windowTestClock
	opts            sshKeyFileOptions
}

func sshKeyFixtureNew(t *testing.T) sshKeyFixture {
	t.Helper()
	dir, b, opts, clock := windowFixture(t)
	operation := filepath.Join(opts.anchor, b.OperationID)
	keysDir := filepath.Join(opts.anchor, "keys")
	if os.Mkdir(operation, 0700) != nil || os.Mkdir(keysDir, 0755) != nil {
		t.Fatal("private fixture directories")
	}
	public, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	key, e := ssh.NewPublicKey(public)
	if e != nil {
		t.Fatal(e)
	}
	original := append([]byte("# exact original\ncommand=\"bash -s\" "), ssh.MarshalAuthorizedKey(key)...)
	prep, e := PrepareSSH(original, digest(original), "/opt/qs-fence/gate", "/etc/qs-fence/policy.json", strings.Repeat("a", 64), "/etc/qs-fence/api-read-token", []string{ssh.FingerprintSHA256(key)})
	if e != nil {
		t.Fatal(e)
	}
	keys := filepath.Join(keysDir, "authorized_keys")
	if os.WriteFile(keys, original, 0644) != nil || os.Chmod(keys, 0644) != nil {
		t.Fatal("private fixture original")
	}
	w, e := startMaintenanceWindow(context.Background(), dir, b, opts)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if w.Close() != nil {
			t.Error("window close")
		}
	})
	leafOpts := sshKeyFileOptions{owner: opts.owner, anchor: opts.anchor, exchange: sshKeyTestExchange, metadata: sshKeyTestMetadata}
	return sshKeyFixture{operation, keys, prep, w, clock, leafOpts}
}
func (f sshKeyFixture) open(t *testing.T) *SSHKeyFileLease {
	t.Helper()
	l, e := openSSHKeyFileLease(context.Background(), f.operation, f.keys, f.prep, f.window, f.opts)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if l.Close() != nil {
			t.Error("key lease close")
		}
	})
	return l
}
func sshKeyNoAuthority(t *testing.T, r SSHKeyFileReceipt) {
	t.Helper()
	if r.RootEffectiveSSHDVerified || r.WholeSystemWritersFenced || r.MutationBackendEnabled || r.DropReady {
		t.Fatal("file receipt minted authority")
	}
}
func sshKeyRead(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func sshKeyStat(t *testing.T, path string) windowStamp {
	t.Helper()
	var st unix.Stat_t
	if unix.Lstat(path, &st) != nil {
		t.Fatal("stat fixture")
	}
	return windowFileStamp(st)
}

func TestSSHKeyFileExactInstallRestorePrivateFilesystem(t *testing.T) {
	f := sshKeyFixtureNew(t)
	l := f.open(t)
	original := sshKeyStat(t, f.keys)
	if _, e := json.Marshal(l); !errors.Is(e, ErrSSHKeyFile) {
		t.Fatal("serialized lease")
	}
	if strings.Contains(l.String(), string(f.prep.original)) {
		t.Fatal("raw key in string")
	}
	r, e := l.Install(context.Background())
	if e != nil || !r.ExactFileInstalled || r.ExactOriginalRestored || r.StateUnknown || !sha64.MatchString(r.InstallResultSHA256) {
		t.Fatal("private install", e, r)
	}
	sshKeyNoAuthority(t, r)
	installed := sshKeyStat(t, f.keys)
	if installed.Identity == original.Identity || installed.Mode != original.Mode || installed.Owner != original.Owner || installed.Group != original.Group || installed.Links != 1 || !bytes.Equal(sshKeyRead(t, f.keys), f.prep.restricted) {
		t.Fatal("identity/mode/owner changed incorrectly")
	}
	contender, e := unix.Open(f.keys, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		t.Fatal("active inode contender")
	}
	lockErr := unix.Flock(contender, unix.LOCK_EX|unix.LOCK_NB)
	closeErr := unix.Close(contender)
	if lockErr == nil || closeErr != nil {
		t.Fatal("installed inode cooperative lease not held")
	}
	slot := filepath.Join(filepath.Dir(f.keys), l.slotName)
	if sshKeyStat(t, slot).Identity != original.Identity || !bytes.Equal(sshKeyRead(t, slot), f.prep.original) {
		t.Fatal("original inode not retained")
	}
	if _, e = l.Install(context.Background()); !errors.Is(e, ErrSSHKeyFileUsed) {
		t.Fatal("duplicate install")
	}
	f.clock.advance(1100 * time.Second)
	r, e = l.Restore(context.Background())
	if e != nil || r.ExactFileInstalled || !r.ExactOriginalRestored || r.StateUnknown || !sha64.MatchString(r.RestoreResultSHA256) {
		t.Fatal("private restore", e, r)
	}
	sshKeyNoAuthority(t, r)
	restored := sshKeyStat(t, f.keys)
	if restored.Identity != original.Identity || restored.Mode != original.Mode || restored.Owner != original.Owner || restored.Group != original.Group || restored.Links != original.Links || restored.ModifiedNanos != original.ModifiedNanos || !bytes.Equal(sshKeyRead(t, f.keys), f.prep.original) {
		t.Fatal("did not restore exact original")
	}
	if sshKeyStat(t, slot).Identity != installed.Identity || !bytes.Equal(sshKeyRead(t, slot), f.prep.restricted) {
		t.Fatal("installed inode lost")
	}
	if f.window.recovery == nil || f.window.recovery.DeadlineNanos != f.window.start.StartedNanos+int64(1700*time.Second) {
		t.Fatal("not original recovery budget")
	}
	if _, e = l.Restore(context.Background()); !errors.Is(e, ErrSSHKeyFileUsed) {
		t.Fatal("duplicate restore")
	}
	for _, name := range []string{"install-intent.json", "install-result.json", "restore-intent.json", "restore-result.json"} {
		raw := sshKeyRead(t, filepath.Join(f.operation, name))
		if bytes.Contains(raw, f.prep.original) || bytes.Contains(raw, f.prep.restricted) || !bytes.Contains(raw, []byte(l.windowStart)) {
			t.Fatal("record leaked bytes or omitted budget binding")
		}
	}
	copyLease := new(SSHKeyFileLease)
	reflect.ValueOf(copyLease).Elem().Set(reflect.ValueOf(l).Elem())
	if _, e = copyLease.Install(context.Background()); !errors.Is(e, ErrSSHKeyFile) {
		t.Fatal("value copy gained descriptors")
	}
	if e = copyLease.Close(); !errors.Is(e, ErrSSHKeyFile) {
		t.Fatal("copy released original descriptors")
	}
	if l.Close() != nil {
		t.Fatal("first close")
	}
	if l.Close() != nil {
		t.Fatal("idempotent close")
	}
	if _, e = openSSHKeyFileLease(context.Background(), f.operation, f.keys, f.prep, f.window, f.opts); e == nil {
		t.Fatal("adopted existing operation")
	}
}

func TestSSHKeyFilePublicRootPlatformBoundary(t *testing.T) {
	f := sshKeyFixtureNew(t)
	if _, e := OpenSSHKeyFileLease(context.Background(), f.operation, f.keys, f.prep, f.window); e == nil {
		t.Fatal("private test seam enabled public API")
	}
	names, e := os.ReadDir(f.operation)
	if e != nil || len(names) != 0 || !bytes.Equal(sshKeyRead(t, f.keys), f.prep.original) {
		t.Fatal("public reject mutated")
	}
}

func TestSSHKeyFileOpenRejectsUnprotectedOrUnboundInputs(t *testing.T) {
	for _, kind := range []string{"absent", "symlink", "hardlink", "writable", "unsafe_parent", "nonempty_operation", "operation_mode", "wrong_operation", "hash", "nil_window", "copied_window", "nil_preparation", "cancelled", "slot_exists", "duplicate_lease", "extended_metadata"} {
		t.Run(kind, func(t *testing.T) {
			f := sshKeyFixtureNew(t)
			ctx := context.Background()
			switch kind {
			case "absent":
				if os.Remove(f.keys) != nil {
					t.Fatal("remove")
				}
			case "symlink":
				target := f.keys + ".target"
				if os.Rename(f.keys, target) != nil || os.Symlink(target, f.keys) != nil {
					t.Fatal("symlink")
				}
			case "hardlink":
				if os.Link(f.keys, f.keys+".link") != nil {
					t.Fatal("hardlink")
				}
			case "writable":
				if os.Chmod(f.keys, 0666) != nil {
					t.Fatal("chmod")
				}
			case "unsafe_parent":
				if os.Chmod(filepath.Dir(f.keys), 0777) != nil {
					t.Fatal("chmod")
				}
			case "nonempty_operation":
				if os.WriteFile(filepath.Join(f.operation, "existing"), []byte("existing"), 0600) != nil {
					t.Fatal("write")
				}
			case "operation_mode":
				if os.Chmod(f.operation, 0755) != nil {
					t.Fatal("chmod")
				}
			case "wrong_operation":
				f.operation = filepath.Join(filepath.Dir(f.operation), "999-1")
				if os.Mkdir(f.operation, 0700) != nil {
					t.Fatal("mkdir")
				}
			case "hash":
				copyPrep := *f.prep
				copyPrep.originalHash = strings.Repeat("f", 64)
				f.prep = &copyPrep
			case "nil_window":
				f.window = nil
			case "copied_window":
				copied := new(MaintenanceWindow)
				reflect.ValueOf(copied).Elem().Set(reflect.ValueOf(f.window).Elem())
				f.window = copied
			case "nil_preparation":
				f.prep = nil
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			case "slot_exists":
				if os.WriteFile(filepath.Join(filepath.Dir(f.keys), ".qs-retirement-"+f.window.binding.OperationID+".original"), []byte("other"), 0600) != nil {
					t.Fatal("write")
				}
			case "duplicate_lease":
				_ = f.open(t)
			case "extended_metadata":
				if unix.Setxattr(f.keys, "user.qs_retirement_fixture", []byte("x"), 0) != nil {
					t.Fatal("actual xattr fixture")
				}
			}
			l, e := openSSHKeyFileLease(ctx, f.operation, f.keys, f.prep, f.window, f.opts)
			if e == nil {
				_ = l.Close()
				t.Fatal("accepted unprotected or unbound input")
			}
		})
	}
}

func TestSSHKeyFileCASPreconditionRejectsWithoutMutation(t *testing.T) {
	for _, kind := range []string{"bytes", "mode", "inode", "parent_replaced", "expired", "window_closed"} {
		t.Run(kind, func(t *testing.T) {
			f := sshKeyFixtureNew(t)
			l := f.open(t)
			switch kind {
			case "bytes":
				if os.WriteFile(f.keys, []byte("external"), 0644) != nil {
					t.Fatal("write")
				}
			case "mode":
				if os.Chmod(f.keys, 0600) != nil {
					t.Fatal("chmod")
				}
			case "inode":
				if os.Rename(f.keys, f.keys+".old") != nil || os.WriteFile(f.keys, f.prep.original, 0644) != nil {
					t.Fatal("replace")
				}
			case "parent_replaced":
				parent := filepath.Dir(f.keys)
				if os.Rename(parent, parent+".old") != nil || os.Mkdir(parent, 0755) != nil || os.WriteFile(f.keys, []byte("external"), 0644) != nil {
					t.Fatal("replace directory")
				}
			case "expired":
				f.clock.advance(1200 * time.Second)
			case "window_closed":
				if f.window.Close() != nil {
					t.Fatal("close window")
				}
			}
			before := sshKeyRead(t, f.keys)
			if _, e := l.Install(context.Background()); e == nil {
				t.Fatal("CAS mismatch accepted")
			}
			names, e := os.ReadDir(f.operation)
			if e != nil || len(names) != 0 || !bytes.Equal(before, sshKeyRead(t, f.keys)) {
				t.Fatal("precondition reject wrote")
			}
		})
	}
}

func TestSSHKeyFileRestoreRejectsExternalChangesAndPreservesAssets(t *testing.T) {
	for _, kind := range []string{"bytes", "mode", "inode", "hardlink", "original_slot", "intent_bytes", "intent_mode", "extra_record", "new_window"} {
		t.Run(kind, func(t *testing.T) {
			f := sshKeyFixtureNew(t)
			l := f.open(t)
			if _, e := l.Install(context.Background()); e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "bytes":
				if os.WriteFile(f.keys, []byte("external"), 0644) != nil {
					t.Fatal("write")
				}
			case "mode":
				if os.Chmod(f.keys, 0600) != nil {
					t.Fatal("chmod")
				}
			case "inode":
				if os.Rename(f.keys, f.keys+".external") != nil || os.WriteFile(f.keys, f.prep.restricted, 0644) != nil {
					t.Fatal("replace")
				}
			case "hardlink":
				if os.Link(f.keys, f.keys+".link") != nil {
					t.Fatal("link")
				}
			case "original_slot":
				if os.WriteFile(filepath.Join(filepath.Dir(f.keys), l.slotName), []byte("external original"), 0644) != nil {
					t.Fatal("write")
				}
			case "intent_bytes":
				if os.WriteFile(filepath.Join(f.operation, "install-intent.json"), []byte("external"), 0600) != nil {
					t.Fatal("write")
				}
			case "intent_mode":
				if os.Chmod(filepath.Join(f.operation, "install-intent.json"), 0400) != nil {
					t.Fatal("chmod")
				}
			case "extra_record":
				if os.WriteFile(filepath.Join(f.operation, "extra"), []byte("external"), 0600) != nil {
					t.Fatal("write")
				}
			case "new_window":
				original := l.window
				if original.Close() != nil {
					t.Fatal("close")
				}
				reopened, e := openMaintenanceWindow(context.Background(), filepath.Join(f.opts.anchor, "window"), original.binding, original.opts)
				if e != nil {
					t.Fatal(e)
				}
				defer func() {
					if reopened.Close() != nil {
						t.Error("close")
					}
				}()
			}
			before := sshKeyRead(t, f.keys)
			if _, e := l.Restore(context.Background()); e == nil {
				t.Fatal("restore accepted external edit/new lease")
			}
			if !bytes.Equal(before, sshKeyRead(t, f.keys)) {
				t.Fatal("restore overwrote external fact")
			}
			if _, e := os.Stat(filepath.Join(f.operation, "install-intent.json")); e != nil {
				t.Fatal("lost actual intent")
			}
		})
	}
}

func TestSSHKeyFileUnknownMutationNeverReplayed(t *testing.T) {
	for _, stage := range []string{"install-intent_durable", "original_durable", "restricted_durable", "before_install_exchange", "install_exchanged", "install_directory_durable", "install-result_durable", "restore-intent_durable", "before_restore_exchange", "restore_exchanged", "restore_directory_durable", "restore-result_durable"} {
		t.Run(stage, func(t *testing.T) {
			f := sshKeyFixtureNew(t)
			l := f.open(t)
			restore := strings.HasPrefix(stage, "restore") || stage == "before_restore_exchange"
			if restore {
				if _, e := l.Install(context.Background()); e != nil {
					t.Fatal(e)
				}
			}
			fixtureHook := l.opts.after
			l.opts.after = func(actual string) error {
				if fixtureHook != nil {
					if e := fixtureHook(actual); e != nil {
						return e
					}
				}
				if actual == stage {
					return errors.New("injected durability observation failure")
				}
				return nil
			}
			var r SSHKeyFileReceipt
			var e error
			if restore {
				r, e = l.Restore(context.Background())
			} else {
				r, e = l.Install(context.Background())
			}
			if !errors.Is(e, ErrSSHKeyFileUnknown) || !r.StateUnknown || r.ExactFileInstalled || r.ExactOriginalRestored {
				t.Fatal("unknown was success", e, r)
			}
			sshKeyNoAuthority(t, r)
			before := sshKeyRead(t, f.keys)
			l.opts.after = nil
			if _, e = l.Install(context.Background()); !errors.Is(e, ErrSSHKeyFileUsed) {
				t.Fatal("unknown install replay")
			}
			if _, e = l.Restore(context.Background()); !errors.Is(e, ErrSSHKeyFileUsed) {
				t.Fatal("unknown restore replay")
			}
			if !bytes.Equal(before, sshKeyRead(t, f.keys)) {
				t.Fatal("unknown operation changed on replay")
			}
			if _, e = os.Stat(filepath.Join(f.operation, "install-intent.json")); e != nil {
				t.Fatal("unknown intent not preserved")
			}
			if l.Close() != nil {
				t.Fatal("close")
			}
			if _, e = openSSHKeyFileLease(context.Background(), f.operation, f.keys, f.prep, f.window, f.opts); e == nil {
				t.Fatal("unknown adopted")
			}
		})
	}
}

func TestSSHKeyFileExchangeFailureOrRootRaceIsUnknown(t *testing.T) {
	for _, kind := range []string{"error_before", "error_after", "root_race", "metadata_race"} {
		t.Run(kind, func(t *testing.T) {
			f := sshKeyFixtureNew(t)
			native := f.opts.exchange
			f.opts.exchange = func(dir int, a, b string) error {
				if kind == "error_before" {
					return unix.EIO
				}
				if kind == "root_race" {
					if os.WriteFile(f.keys, []byte("concurrent root-like writer"), 0644) != nil {
						return unix.EIO
					}
				}
				if e := native(dir, a, b); e != nil {
					return e
				}
				if kind == "error_after" {
					return unix.EIO
				}
				return nil
			}
			l := f.open(t)
			if kind == "metadata_race" {
				fixtureHook := l.opts.after
				l.opts.after = func(stage string) error {
					if fixtureHook != nil {
						if e := fixtureHook(stage); e != nil {
							return e
						}
					}
					if stage == "install-result_durable" {
						return os.Chmod(f.keys, 0600)
					}
					return nil
				}
			}
			r, e := l.Install(context.Background())
			if !errors.Is(e, ErrSSHKeyFileUnknown) || !r.StateUnknown || r.ExactFileInstalled {
				t.Fatal("exchange uncertainty/root race accepted", e, r)
			}
			before := sshKeyRead(t, f.keys)
			if _, e = l.Restore(context.Background()); !errors.Is(e, ErrSSHKeyFileUsed) || !bytes.Equal(before, sshKeyRead(t, f.keys)) {
				t.Fatal("unknown root race automatically restored")
			}
		})
	}
}

func TestSSHKeyFileModeAndOriginalRecoveryDeadline(t *testing.T) {
	for _, mode := range []os.FileMode{0400, 0600, 0644} {
		t.Run(mode.String(), func(t *testing.T) {
			f := sshKeyFixtureNew(t)
			if os.Chmod(f.keys, mode) != nil {
				t.Fatal("mode fixture")
			}
			l := f.open(t)
			if _, e := l.Install(context.Background()); e != nil {
				t.Fatal(e)
			}
			if sshKeyStat(t, f.keys).Mode&07777 != uint32(mode) {
				t.Fatal("install mode changed")
			}
			if _, e := l.Restore(context.Background()); e != nil {
				t.Fatal(e)
			}
			if sshKeyStat(t, f.keys).Mode&07777 != uint32(mode) {
				t.Fatal("restore mode changed")
			}
		})
	}
	t.Run("existing_recovery_deadline", func(t *testing.T) {
		f := sshKeyFixtureNew(t)
		l := f.open(t)
		if _, e := l.Install(context.Background()); e != nil {
			t.Fatal(e)
		}
		_, cancel, e := f.window.RecoveryContext(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		cancel()
		deadline := f.window.recovery.DeadlineNanos
		f.clock.advance(599 * time.Second)
		if _, e = l.Restore(context.Background()); e != nil || f.window.recovery.DeadlineNanos != deadline {
			t.Fatal("reset or lost original recovery budget", e)
		}
	})
	t.Run("recovery_expired_preserves_installed", func(t *testing.T) {
		f := sshKeyFixtureNew(t)
		l := f.open(t)
		if _, e := l.Install(context.Background()); e != nil {
			t.Fatal(e)
		}
		_, cancel, e := f.window.RecoveryContext(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		cancel()
		f.clock.advance(600 * time.Second)
		if _, e = l.Restore(context.Background()); !errors.Is(e, ErrWindowExpired) {
			t.Fatal("reset expired budget", e)
		}
		if !bytes.Equal(sshKeyRead(t, f.keys), f.prep.restricted) {
			t.Fatal("expired restore wrote")
		}
		if _, e = os.Stat(filepath.Join(f.operation, "restore-intent.json")); !errors.Is(e, os.ErrNotExist) {
			t.Fatal("expired restore reserved mutation")
		}
	})
}
