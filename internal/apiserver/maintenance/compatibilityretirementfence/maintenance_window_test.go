package compatibilityretirementfence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type windowTestClock struct {
	mu     sync.Mutex
	sample windowClockSample
	err    error
}

func (c *windowTestClock) read() (windowClockSample, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sample, c.err
}
func (c *windowTestClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sample.nanos += int64(d)
	c.sample.utc = c.sample.utc.Add(d)
}
func windowFixture(t *testing.T) (string, WindowBinding, windowOptions, *windowTestClock) {
	t.Helper()
	anchor := t.TempDir()
	if os.Chmod(anchor, 0700) != nil {
		t.Fatal("private root fixture chmod")
	}
	dir := filepath.Join(anchor, "window")
	if os.Mkdir(dir, 0700) != nil {
		t.Fatal("private root fixture mkdir")
	}
	b := WindowBinding{MaintenanceWindowTargetSHA256(), strings.Repeat("a", 40), "800-1", strings.Repeat("b", 64), "801-1"}
	c := &windowTestClock{sample: windowClockSample{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", int64(5000 * time.Second), time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC)}}
	return dir, b, windowOptions{owner: uint32(os.Geteuid()), anchor: anchor, clockSource: "private_test_clock_v1", clock: c.read}, c
}
func windowClose(t *testing.T, w *MaintenanceWindow) {
	t.Helper()
	if w.Close() != nil {
		t.Fatal("own lease close")
	}
}
func windowCancel(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("budget context did not cancel")
	}
}

func TestMaintenanceWindowAbsoluteBudgetAndAttemptReopen(t *testing.T) {
	dir, b, opts, c := windowFixture(t)
	w, e := startMaintenanceWindow(context.Background(), dir, b, opts)
	if e != nil {
		t.Fatal(e)
	}
	q, cancel, e := w.ForwardContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	windowCancel(t, q)
	original := w.files[windowStartName].raw
	c.advance(1100 * time.Second)
	q, cancel, e = w.RecoveryContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	windowCancel(t, q)
	deadline := w.recovery.DeadlineNanos
	if deadline != w.start.StartedNanos+int64(1700*time.Second) {
		t.Fatal("recovery not first start plus ten minutes")
	}
	if _, _, e = w.ForwardContext(context.Background()); !errors.Is(e, ErrWindowExpired) {
		t.Fatal("forward still allowed after recovery")
	}
	diagnostic, e := w.Diagnostic(context.Background())
	if e != nil || !diagnostic.BudgetOnly || !diagnostic.DirectoryLeaseHeld || diagnostic.WholeWriterFence || diagnostic.MutationAllowed || diagnostic.DropReady || diagnostic.RecoveryBudgetMeasured {
		t.Fatal("diagnostic minted authority")
	}
	if _, e = json.Marshal(w); !errors.Is(e, ErrWindowBinding) {
		t.Fatal("opaque lease serialized")
	}
	if _, e = startMaintenanceWindow(context.Background(), dir, b, opts); !errors.Is(e, ErrWindowExists) {
		t.Fatal("duplicate start accepted")
	}
	if _, e = openMaintenanceWindow(context.Background(), dir, b, opts); !errors.Is(e, ErrWindowBusy) {
		t.Fatal("second process lease accepted")
	}
	windowClose(t, w)
	c.advance(100 * time.Second)
	reopened, e := openMaintenanceWindow(context.Background(), dir, b, opts)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(original, reopened.files[windowStartName].raw) || reopened.recovery.DeadlineNanos != deadline {
		t.Fatal("attempt rewrote original start or deadline")
	}
	q, cancel, e = reopened.RecoveryContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	windowCancel(t, q)
	windowClose(t, reopened)
	c.advance(500 * time.Second)
	if _, e = openMaintenanceWindow(context.Background(), dir, b, opts); !errors.Is(e, ErrWindowExpired) {
		t.Fatal("expired attempt got a new budget")
	}
}
func TestMaintenanceWindowForwardAndThirtyMinuteBoundary(t *testing.T) {
	for _, seconds := range []int{1199, 1200, 1799, 1800} {
		t.Run(strconv.Itoa(seconds), func(t *testing.T) {
			dir, b, opts, c := windowFixture(t)
			w, e := startMaintenanceWindow(context.Background(), dir, b, opts)
			if e != nil {
				t.Fatal(e)
			}
			defer windowClose(t, w)
			c.advance(time.Duration(seconds) * time.Second)
			_, cancel, e := w.ForwardContext(context.Background())
			if cancel != nil {
				cancel()
			}
			if seconds < 1200 && e != nil || seconds >= 1200 && !errors.Is(e, ErrWindowExpired) {
				t.Fatal("forward boundary wrong")
			}
			_, cancel, e = w.RecoveryContext(context.Background())
			if cancel != nil {
				cancel()
			}
			if seconds >= 1800 {
				if !errors.Is(e, ErrWindowExpired) {
					t.Fatal("recovery after absolute end")
				}
			} else if e != nil || w.recovery.DeadlineNanos != min(w.start.WindowDeadline, w.start.StartedNanos+int64(time.Duration(seconds)*time.Second)+windowRecoveryNanos) {
				t.Fatal("window end did not cap recovery")
			}
		})
	}
}
func TestMaintenanceWindowContextParentCloseAndBootClock(t *testing.T) {
	for _, kind := range []string{"parent", "close", "forward_boot_deadline", "recovery_boot_deadline", "boot_change", "clock_backwards", "clock_unavailable"} {
		t.Run(kind, func(t *testing.T) {
			dir, b, opts, c := windowFixture(t)
			w, e := startMaintenanceWindow(context.Background(), dir, b, opts)
			if e != nil {
				t.Fatal(e)
			}
			defer windowClose(t, w)
			parent, parentCancel := context.WithCancel(context.Background())
			defer parentCancel()
			var q context.Context
			var cancel context.CancelFunc
			if kind == "recovery_boot_deadline" {
				q, cancel, e = w.RecoveryContext(parent)
			} else {
				q, cancel, e = w.ForwardContext(parent)
			}
			if e != nil {
				t.Fatal(e)
			}
			defer cancel()
			switch kind {
			case "parent":
				parentCancel()
			case "close":
				windowClose(t, w)
			case "forward_boot_deadline":
				c.advance(1200 * time.Second)
			case "recovery_boot_deadline":
				c.advance(600 * time.Second)
			case "boot_change":
				c.mu.Lock()
				c.sample.bootID = "ffffffff-bbbb-cccc-dddd-eeeeeeeeeeee"
				c.mu.Unlock()
			case "clock_backwards":
				c.advance(-time.Second)
			case "clock_unavailable":
				c.mu.Lock()
				c.err = ErrWindowClock
				c.mu.Unlock()
			}
			windowCancel(t, q)
			want := ErrWindowClock
			switch kind {
			case "parent":
				want = context.Canceled
			case "close":
				want = ErrWindowClosed
			case "forward_boot_deadline", "recovery_boot_deadline":
				want = ErrWindowExpired
			}
			if !errors.Is(context.Cause(q), want) {
				t.Fatal("fixed budget cancellation cause lost")
			}
		})
	}
}
func TestMaintenanceWindowInvalidBindingAndOpaqueZero(t *testing.T) {
	for _, kind := range []string{"target", "source", "op", "manifest", "originalrun"} {
		t.Run(kind, func(t *testing.T) {
			dir, b, opts, _ := windowFixture(t)
			switch kind {
			case "target":
				b.TargetSHA256 = strings.Repeat("c", 64)
			case "source":
				b.SourceSHA = "bad"
			case "op":
				b.OperationID = "0-1"
			case "manifest":
				b.ManifestSHA256 = "bad"
			case "originalrun":
				b.OriginalRunID = "901"
			}
			if _, e := startMaintenanceWindow(context.Background(), dir, b, opts); !errors.Is(e, ErrWindowBinding) {
				t.Fatal("invalid binding accepted")
			}
			files, e := os.ReadDir(dir)
			if e != nil || len(files) != 0 {
				t.Fatal("invalid binding touched store")
			}
		})
	}
	for _, w := range []*MaintenanceWindow{nil, {}} {
		if _, _, e := w.ForwardContext(context.Background()); !errors.Is(e, ErrWindowClosed) {
			t.Fatal("zero lease accepted")
		}
		if w.Close() != nil {
			t.Fatal("zero close unsafe")
		}
	}
}
func TestMaintenanceWindowPartialUnknownAndFilesystem(t *testing.T) {
	for _, kind := range []string{"torn_start", "unknown_start_field", "duplicate_start_field", "missing_start_seal", "partial_recovery", "torn_recovery", "unknown_file", "same_bytes_new_inode", "same_bytes_metadata_change", "seal_inode_change", "lock_inode_change", "lock_metadata_change", "symlink", "fifo", "hardlink", "file_mode", "dir_mode", "ancestor_mode", "wrong_owner", "binding_change", "boot_change", "clock_source_change", "recovery_formula_change"} {
		t.Run(kind, func(t *testing.T) {
			dir, b, opts, c := windowFixture(t)
			w, e := startMaintenanceWindow(context.Background(), dir, b, opts)
			if e != nil {
				t.Fatal(e)
			}
			if kind == "torn_recovery" || kind == "recovery_formula_change" {
				_, cancel, e := w.RecoveryContext(context.Background())
				if e != nil {
					t.Fatal(e)
				}
				cancel()
			}
			windowClose(t, w)
			key := filepath.Join(dir, windowStartName)
			write := func(path string, data []byte) {
				t.Helper()
				if os.WriteFile(path, data, 0600) != nil {
					t.Fatal("fixture edit")
				}
			}
			replace := func(path string) {
				t.Helper()
				// Keep the unlinked original open so the filesystem cannot reuse
				// its inode for the replacement this test intends to exercise.
				original, e := os.Open(path)
				if e != nil {
					t.Fatal("fixture original open")
				}
				t.Cleanup(func() {
					if original.Close() != nil {
						t.Error("fixture original close")
					}
				})
				before, e := original.Stat()
				if e != nil {
					t.Fatal("fixture original identity")
				}
				raw, e := io.ReadAll(original)
				if e != nil || os.Remove(path) != nil {
					t.Fatal("fixture replacement")
				}
				write(path, raw)
				after, e := os.Stat(path)
				if e != nil || os.SameFile(before, after) {
					t.Fatal("fixture did not replace the original inode")
				}
			}
			switch kind {
			case "torn_start":
				write(key, []byte("{"))
			case "unknown_start_field":
				raw, e := os.ReadFile(key)
				if e != nil {
					t.Fatal(e)
				}
				write(key, append(raw[:len(raw)-1], []byte(",\"complete\":true}")...))
			case "duplicate_start_field":
				raw, e := os.ReadFile(key)
				if e != nil {
					t.Fatal(e)
				}
				write(key, append(raw[:len(raw)-1], []byte(",\"kind\":\"start\"}")...))
			case "missing_start_seal":
				if os.Remove(filepath.Join(dir, windowStartSealName)) != nil {
					t.Fatal("fixture remove")
				}
			case "partial_recovery":
				write(filepath.Join(dir, windowRecoverName), []byte("{}"))
			case "torn_recovery":
				write(filepath.Join(dir, windowRecoverName), []byte("{"))
			case "unknown_file":
				write(filepath.Join(dir, "unexpected.json"), []byte("{}"))
			case "same_bytes_new_inode":
				replace(key)
			case "same_bytes_metadata_change":
				stamp := time.Now().Add(time.Hour)
				if os.Chtimes(key, stamp, stamp) != nil {
					t.Fatal("fixture time edit")
				}
			case "seal_inode_change":
				replace(filepath.Join(dir, windowStartSealName))
			case "lock_inode_change":
				replace(filepath.Join(dir, windowLockName))
			case "lock_metadata_change":
				stamp := time.Now().Add(time.Hour)
				if os.Chtimes(filepath.Join(dir, windowLockName), stamp, stamp) != nil {
					t.Fatal("fixture lock metadata")
				}
			case "symlink":
				if os.Remove(key) != nil || os.Symlink("missing", key) != nil {
					t.Fatal("fixture symlink")
				}
			case "fifo":
				if os.Remove(key) != nil || unix.Mkfifo(key, 0600) != nil {
					t.Fatal("fixture FIFO")
				}
			case "hardlink":
				if os.Link(key, filepath.Join(dir, "other")) != nil {
					t.Fatal("fixture link")
				}
			case "file_mode":
				if os.Chmod(key, 0644) != nil {
					t.Fatal("fixture mode")
				}
			case "dir_mode":
				if os.Chmod(dir, 0777) != nil {
					t.Fatal("fixture dir mode")
				}
			case "ancestor_mode":
				if os.Chmod(opts.anchor, 0777) != nil {
					t.Fatal("fixture ancestor")
				}
			case "wrong_owner":
				opts.owner++
			case "binding_change":
				b.ManifestSHA256 = strings.Repeat("c", 64)
			case "boot_change":
				c.mu.Lock()
				c.sample.bootID = "ffffffff-bbbb-cccc-dddd-eeeeeeeeeeee"
				c.mu.Unlock()
			case "clock_source_change":
				opts.clockSource = "linux_clock_boottime_v1"
			case "recovery_formula_change":
				raw, e := os.ReadFile(filepath.Join(dir, windowRecoverName))
				if e != nil {
					t.Fatal(e)
				}
				var record windowRecoveryRecord
				if json.Unmarshal(raw, &record) != nil {
					t.Fatal("fixture decode")
				}
				record.DeadlineNanos++
				raw, e = json.Marshal(record)
				if e != nil {
					t.Fatal(e)
				}
				write(filepath.Join(dir, windowRecoverName), raw)
			}
			started := time.Now()
			if _, e = openMaintenanceWindow(context.Background(), dir, b, opts); e == nil {
				t.Fatal("invalid or unknown state adopted")
			}
			if time.Since(started) > time.Second {
				t.Fatal("nonregular source blocked open")
			}
		})
	}
}
func TestMaintenanceWindowDurabilityInterruption(t *testing.T) {
	for _, stage := range []string{"lease_created", "lease_durable", "start_created", "start_durable", "start_committed_created", "start_committed_durable", "recovery_created", "recovery_durable", "recovery_committed_created", "recovery_committed_durable"} {
		t.Run(stage, func(t *testing.T) {
			dir, b, opts, _ := windowFixture(t)
			if strings.HasPrefix(stage, "recovery") {
				w, e := startMaintenanceWindow(context.Background(), dir, b, opts)
				if e != nil {
					t.Fatal(e)
				}
				w.opts.after = func(s string) error {
					if s == stage {
						return ErrWindowUnknown
					}
					return nil
				}
				if _, _, e = w.RecoveryContext(context.Background()); !errors.Is(e, ErrWindowUnknown) {
					t.Fatal("durability failure accepted")
				}
				windowClose(t, w)
			} else {
				opts.after = func(s string) error {
					if s == stage {
						return ErrWindowUnknown
					}
					return nil
				}
				if _, e := startMaintenanceWindow(context.Background(), dir, b, opts); !errors.Is(e, ErrWindowUnknown) {
					t.Fatal("durability failure accepted")
				}
			}
			opts.after = nil
			if _, e := startMaintenanceWindow(context.Background(), dir, b, opts); !errors.Is(e, ErrWindowExists) {
				t.Fatal("interrupted state reinitialized")
			}
			// A post-fsync hook can fail after a fully sealed primary+seal. A later Open
			// may verify that complete immutable state, but can never rewrite it.
			if strings.HasSuffix(stage, "committed_durable") {
				opened, e := openMaintenanceWindow(context.Background(), dir, b, opts)
				if e != nil {
					t.Fatal("full seal not reopenable")
				}
				windowClose(t, opened)
			} else if _, e := openMaintenanceWindow(context.Background(), dir, b, opts); e == nil {
				t.Fatal("partial state adopted")
			}
		})
	}
}

func TestMaintenanceWindowProcessHelper(t *testing.T) {
	mode := os.Getenv("QS_WINDOW_TEST_CHILD")
	if mode == "" {
		return
	}
	anchor, dir := os.Getenv("QS_WINDOW_TEST_ANCHOR"), os.Getenv("QS_WINDOW_TEST_DIR")
	b := WindowBinding{MaintenanceWindowTargetSHA256(), strings.Repeat("a", 40), "800-1", strings.Repeat("b", 64), "801-1"}
	nanos, e := strconv.ParseInt(os.Getenv("QS_WINDOW_TEST_NANOS"), 10, 64)
	if e != nil {
		os.Exit(90)
	}
	opts := windowOptions{owner: uint32(os.Geteuid()), anchor: anchor, clockSource: "private_test_clock_v1", clock: func() (windowClockSample, error) {
		return windowClockSample{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", nanos, time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC)}, nil
	}}
	if mode == "crash_start" {
		opts.after = func(s string) error {
			if s == "start_created" {
				os.Exit(77)
			}
			return nil
		}
	}
	if mode == "start" || mode == "crash_start" {
		if _, e := startMaintenanceWindow(context.Background(), dir, b, opts); e != nil {
			os.Exit(91)
		}
		os.Exit(78)
	}
	w, e := openMaintenanceWindow(context.Background(), dir, b, opts)
	if e != nil {
		if mode == "busy" && errors.Is(e, ErrWindowBusy) {
			os.Exit(80)
		}
		os.Exit(92)
	}
	if mode == "recover" {
		if _, _, e = w.RecoveryContext(context.Background()); e != nil {
			os.Exit(93)
		}
		os.Exit(79)
	}
	os.Exit(94)
}
func windowRunChild(t *testing.T, mode, dir string, nanos int64, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMaintenanceWindowProcessHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "QS_WINDOW_TEST_CHILD="+mode, "QS_WINDOW_TEST_ANCHOR="+filepath.Dir(dir), "QS_WINDOW_TEST_DIR="+dir, "QS_WINDOW_TEST_NANOS="+strconv.FormatInt(nanos, 10))
	err := cmd.Run()
	var exit *exec.ExitError
	if ctx.Err() != nil || !errors.As(err, &exit) || exit.ExitCode() != want {
		t.Fatal("child terminal classification mismatch")
	}
}
func TestMaintenanceWindowActualProcessCrashAndCrossAttempt(t *testing.T) {
	t.Run("crash_without_seal", func(t *testing.T) {
		dir, b, opts, c := windowFixture(t)
		windowRunChild(t, "crash_start", dir, c.sample.nanos, 77)
		if _, e := openMaintenanceWindow(context.Background(), dir, b, opts); !errors.Is(e, ErrWindowUnknown) {
			t.Fatal("crashed unsealed start adopted")
		}
		if _, e := startMaintenanceWindow(context.Background(), dir, b, opts); !errors.Is(e, ErrWindowExists) {
			t.Fatal("crashed start reset")
		}
	})
	t.Run("sealed_budget_survives_process_exit", func(t *testing.T) {
		dir, b, opts, c := windowFixture(t)
		windowRunChild(t, "start", dir, c.sample.nanos, 78)
		c.advance(100 * time.Second)
		windowRunChild(t, "recover", dir, c.sample.nanos, 79)
		c.advance(100 * time.Second)
		w, e := openMaintenanceWindow(context.Background(), dir, b, opts)
		if e != nil {
			t.Fatal(e)
		}
		defer windowClose(t, w)
		if w.recovery.FirstRecoveryNanos != w.start.StartedNanos+int64(100*time.Second) || w.recovery.DeadlineNanos != w.start.StartedNanos+int64(700*time.Second) {
			t.Fatal("new process reset recovery")
		}
		windowRunChild(t, "busy", dir, c.sample.nanos, 80)
	})
}
func TestMaintenanceWindowActualPlatformClockBoundary(t *testing.T) {
	if runtime.GOOS != "linux" {
		if _, e := StartMaintenanceWindow(context.Background(), "/", WindowBinding{}); !errors.Is(e, ErrWindowUnavailable) {
			t.Fatal("non-Linux production start enabled")
		}
		if _, e := OpenMaintenanceWindow(context.Background(), "/", WindowBinding{}); !errors.Is(e, ErrWindowUnavailable) {
			t.Fatal("non-Linux production open enabled")
		}
		if _, e := systemWindowClock(); !errors.Is(e, ErrWindowUnavailable) {
			t.Fatal("non-Linux kernel clock emulated")
		}
		return
	}
	before, e := systemWindowClock()
	if e != nil || !windowClockValid(before) {
		t.Fatal("actual Linux boot clock unavailable")
	}
	after, e := systemWindowClock()
	if e != nil || after.bootID != before.bootID || after.nanos < before.nanos {
		t.Fatal("actual Linux boot clock not stable")
	}
}

func TestMaintenanceWindowRecoveryCancelsExistingForwardContext(t *testing.T) {
	dir, b, opts, _ := windowFixture(t)
	w, e := startMaintenanceWindow(context.Background(), dir, b, opts)
	if e != nil {
		t.Fatal(e)
	}
	defer windowClose(t, w)
	forward, cancel, e := w.ForwardContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer cancel()
	_, recoveryCancel, e := w.RecoveryContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer recoveryCancel()
	windowCancel(t, forward)
}

// This opt-in test needs an independently owned empty root-protected directory.
// It never injects a test clock/owner or deletes any record after completion.
// A Darwin filesystem test is not evidence that this Linux production API ran.
func TestMaintenanceWindowActualLinuxRootLease(t *testing.T) {
	dir := os.Getenv("QS_WINDOW_NATIVE_ROOT_DIRECTORY")
	if dir == "" {
		t.Skip("actual Linux root fixture not supplied")
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Fatal("native root fixture supplied on unsupported host")
	}
	b := WindowBinding{MaintenanceWindowTargetSHA256(), strings.Repeat("a", 40), "800-1", strings.Repeat("b", 64), "801-1"}
	w, e := StartMaintenanceWindow(context.Background(), dir, b)
	if e != nil {
		t.Fatal(e)
	}
	q, cancel, e := w.ForwardContext(context.Background())
	if e != nil {
		windowClose(t, w)
		t.Fatal(e)
	}
	cancel()
	windowCancel(t, q)
	if _, e := OpenMaintenanceWindow(context.Background(), dir, b); !errors.Is(e, ErrWindowBusy) {
		windowClose(t, w)
		t.Fatal("actual root flock not exclusive")
	}
	_, cancel, e = w.RecoveryContext(context.Background())
	if e != nil {
		windowClose(t, w)
		t.Fatal(e)
	}
	cancel()
	originalRecovery := w.recovery.DeadlineNanos
	originalStart := w.start.StartedNanos
	windowClose(t, w)
	reopened, e := OpenMaintenanceWindow(context.Background(), dir, b)
	if e != nil {
		t.Fatal(e)
	}
	defer windowClose(t, reopened)
	if reopened.start.StartedNanos != originalStart || reopened.recovery.DeadlineNanos != originalRecovery {
		t.Fatal("actual root re-open reset boot budget")
	}
	receipt, e := reopened.Diagnostic(context.Background())
	if e != nil || receipt.ClockSource != "linux_clock_boottime_v1" || !receipt.BudgetOnly || receipt.WholeWriterFence || receipt.DropReady {
		t.Fatal("native lease minted authority")
	}
}

func TestMaintenanceWindowZeroValueStdinChild(t *testing.T) {
	if os.Getenv("QS_WINDOW_ZERO_CLOSE_CHILD") != "1" {
		return
	}
	var before, after unix.Stat_t
	if unix.Fstat(0, &before) != nil {
		os.Exit(90)
	}
	_ = (&MaintenanceWindow{}).Close()
	if unix.Fstat(0, &after) != nil || !challengeSameInode(before, after) {
		os.Exit(81)
	}
	body, err := io.ReadAll(io.LimitReader(os.Stdin, 128))
	if err != nil || string(body) != "QS_WINDOW_STDIN_OWNERSHIP_SENTINEL_V1" {
		os.Exit(82)
	}
	os.Exit(0)
}
func TestMaintenanceWindowZeroValueClosePreservesHostStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMaintenanceWindowZeroValueStdinChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), "QS_WINDOW_ZERO_CLOSE_CHILD=1")
	cmd.Stdin = strings.NewReader("QS_WINDOW_STDIN_OWNERSHIP_SENTINEL_V1")
	err := cmd.Run()
	var exit *exec.ExitError
	if ctx.Err() != nil {
		t.Fatal("owned stdin child timeout")
	}
	if errors.As(err, &exit) && exit.ExitCode() == 81 {
		t.Fatal("zero lease Close invalidated host stdin")
	}
	if err != nil {
		t.Fatal("owned stdin child terminal classification mismatch")
	}
}

func TestMaintenanceWindowCopiedReceiverCannotUseOrReleaseLease(t *testing.T) {
	dir, b, opts, _ := windowFixture(t)
	w, e := startMaintenanceWindow(context.Background(), dir, b, opts)
	if e != nil {
		t.Fatal(e)
	}
	defer windowClose(t, w)
	// Reflect deliberately creates the otherwise unsafe external value-copy
	// counterexample without silently copying a sync.Mutex in ordinary code.
	w.mu.Lock() // A copied locked mutex must never be acquired by the copy.
	reflected := reflect.New(reflect.TypeOf(w).Elem())
	reflected.Elem().Set(reflect.ValueOf(w).Elem())
	w.mu.Unlock()
	copied := reflected.Interface().(*MaintenanceWindow)
	var before, after unix.Stat_t
	if unix.Fstat(w.lockFD, &before) != nil {
		t.Fatal("owned lock absent before copy test")
	}
	if copied.Close() != ErrWindowClosed {
		t.Fatal("copied lease released ownership")
	}
	if _, _, e = copied.ForwardContext(context.Background()); e != ErrWindowClosed {
		t.Fatal("copy obtained forward budget")
	}
	if _, _, e = copied.RecoveryContext(context.Background()); e != ErrWindowClosed {
		t.Fatal("copy obtained recovery budget")
	}
	if _, e = copied.Diagnostic(context.Background()); e != ErrWindowClosed {
		t.Fatal("copy claimed directory lease")
	}
	if unix.Fstat(w.lockFD, &after) != nil || !challengeSameInode(before, after) {
		t.Fatal("copy closed original descriptor")
	}
	if _, e := os.Lstat(filepath.Join(dir, windowRecoverName)); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("copy wrote recovery record")
	}
	if _, e := w.Diagnostic(context.Background()); e != nil {
		t.Fatal("copy invalidated original lease")
	}
	if _, e := openMaintenanceWindow(context.Background(), dir, b, opts); e != ErrWindowBusy {
		t.Fatal("copy released original flock")
	}
}
