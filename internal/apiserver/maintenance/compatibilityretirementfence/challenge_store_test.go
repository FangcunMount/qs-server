package compatibilityretirementfence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func challengeFixture(t *testing.T, claimChange func(map[string]any)) (Policy, *Permit, string, []byte) {
	t.Helper()
	p := fixturePolicy()
	token, keys := fixtureToken(t, p, claimChange)
	permit, err := AuthorizeProbe(context.Background(), &fixtureClient{p: p, keys: keys}, p, p.Command(), p.SSHKeyFingerprint, token, p.LoginUID, time.Now())
	if err != nil {
		t.Fatal("real signed fixture authorization rejected")
	}
	return p, permit, token, keys
}

func challengeTestDir(t *testing.T) (string, challengeOptions) {
	t.Helper()
	anchor := t.TempDir()
	if err := os.Chmod(anchor, 0700); err != nil {
		t.Fatal("fixture directory permission failed")
	}
	dir := filepath.Join(anchor, "protected", "consumption")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal("fixture directory creation failed")
	}
	return dir, challengeOptions{owner: uint32(os.Geteuid()), anchor: anchor}
}

func challengeRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("fixture state read failed")
	}
	return b
}

func TestChallengePermitCapturesActualSignedExpiry(t *testing.T) {
	p, permit, token, _ := challengeFixture(t, nil)
	if permit.challengeHash != p.ChallengeSHA256 || permit.verifiedAt.IsZero() || !permit.verified || !permit.expiresAt.Before(p.ExpiresAt) || permit.expiresAt.UnixNano()%int64(time.Second) != 0 {
		t.Fatal("opaque verified binding incomplete")
	}
	if !permit.Receipt().ReplayProtectionUnproven || bytes.Contains(mustJSON(t, permit.Receipt()), []byte(token)) {
		t.Fatal("probe receipt claims replay protection or leaks token")
	}
}

func TestChallengeConsumptionDurableAndNoAuthority(t *testing.T) {
	p, permit, token, _ := challengeFixture(t, nil)
	dir, opts := challengeTestDir(t)
	r, err := consumeProbeChallenge(context.Background(), dir, p, permit, opts)
	if err != nil || !r.ProbeChallengeDurable || r.PrivilegedExecutorBound || r.WholeSystemWritersFenced || r.ProductionInstalled || r.MutationBackendEnabled || r.DropReady || !permit.Receipt().ReplayProtectionUnproven {
		t.Fatal("challenge persistence failed or overclaimed authority")
	}
	for _, suffix := range []string{"intent", "result"} {
		path := filepath.Join(dir, p.ChallengeSHA256+"."+suffix+".json")
		b := challengeRead(t, path)
		if bytes.Contains(b, []byte(token)) || bytes.Contains(mustJSON(t, r), []byte(token)) {
			t.Fatal("raw signed token leaked")
		}
		var state challengeState
		if json.Unmarshal(b, &state) != nil || state.PolicySHA256 != p.digest() || state.ChallengeSHA256 != p.ChallengeSHA256 || state.SourceSHA != p.SourceSHA || state.VerifiedAt != r.VerifiedAt || state.ExpiresAt != r.ExpiresAt {
			t.Fatal("state binding changed")
		}
		info, e := os.Lstat(path)
		if e != nil || info.Mode().Perm() != 0600 || !info.Mode().IsRegular() {
			t.Fatal("state file protection failed")
		}
	}
	before := challengeRead(t, filepath.Join(dir, p.ChallengeSHA256+".result.json"))
	if _, err = consumeProbeChallenge(context.Background(), dir, p, permit, opts); !errors.Is(err, ErrChallengeUsed) {
		t.Fatal("successful consumption replay accepted")
	}
	if !bytes.Equal(before, challengeRead(t, filepath.Join(dir, p.ChallengeSHA256+".result.json"))) {
		t.Fatal("success record overwritten")
	}
}

func TestChallengeRejectsWrongCompleteBinding(t *testing.T) {
	p, permit, _, _ := challengeFixture(t, nil)
	cases := []struct {
		name string
		edit func(*Policy)
	}{
		{"source", func(q *Policy) { q.SourceSHA = strings.Repeat("d", 40); q.Audience = q.ExpectedAudience() }},
		{"operation", func(q *Policy) { q.OperationID = "901-1"; q.Audience = q.ExpectedAudience() }},
		{"run", func(q *Policy) { q.RunID = "901"; q.Audience = q.ExpectedAudience() }},
		{"attempt", func(q *Policy) { q.RunAttempt = "2"; q.Audience = q.ExpectedAudience() }},
		{"request", func(q *Policy) { q.RequestSHA256 = strings.Repeat("d", 64); q.Audience = q.ExpectedAudience() }},
		{"challenge", func(q *Policy) { q.ChallengeSHA256 = strings.Repeat("d", 64); q.Audience = q.ExpectedAudience() }},
		{"policy_expiry", func(q *Policy) { q.ExpiresAt = q.ExpiresAt.Add(-time.Second) }},
		{"invalid_policy", func(q *Policy) { q.Version = 2 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir, opts := challengeTestDir(t)
			q := p
			c.edit(&q)
			if _, err := consumeProbeChallenge(context.Background(), dir, q, permit, opts); !errors.Is(err, ErrChallengeBinding) {
				t.Fatal("mismatched complete approval accepted")
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 0 {
				t.Fatal("mismatched approval touched persistent state")
			}
		})
	}
	dir, opts := challengeTestDir(t)
	for _, bad := range []*Permit{nil, {}} {
		if _, err := consumeProbeChallenge(context.Background(), dir, p, bad, opts); !errors.Is(err, ErrChallengeBinding) {
			t.Fatal("nonopaque receipt or empty permit accepted")
		}
	}
}

func TestChallengeUnknownExistingStateAlwaysBlocks(t *testing.T) {
	p, permit, _, _ := challengeFixture(t, nil)
	for _, c := range []struct{ name, suffix, content string }{{"empty_intent", "intent", ""}, {"torn_intent", "intent", "{\"private"}, {"unknown_result_without_intent", "result", "unknown"}, {"success_looking_result", "result", "{\"complete\":true}"}} {
		t.Run(c.name, func(t *testing.T) {
			dir, opts := challengeTestDir(t)
			path := filepath.Join(dir, p.ChallengeSHA256+"."+c.suffix+".json")
			if os.WriteFile(path, []byte(c.content), 0600) != nil {
				t.Fatal("fixture state create failed")
			}
			if _, err := consumeProbeChallenge(context.Background(), dir, p, permit, opts); !errors.Is(err, ErrChallengeUsed) || string(challengeRead(t, path)) != c.content {
				t.Fatal("unknown state adopted or overwritten")
			}
		})
	}
}

func TestChallengeSameKeyCannotBeReusedByNewApproval(t *testing.T) {
	p, permit, _, _ := challengeFixture(t, nil)
	dir, opts := challengeTestDir(t)
	if _, err := consumeProbeChallenge(context.Background(), dir, p, permit, opts); err != nil {
		t.Fatal("first consumption failed")
	}
	// A genuinely reauthorized token with changed request/source/op still cannot
	// reuse the same challenge key within the host's fixed protected store.
	q := p
	q.SourceSHA = strings.Repeat("d", 40)
	q.RequestSHA256 = strings.Repeat("e", 64)
	q.OperationID = "901-1"
	q.Audience = q.ExpectedAudience()
	token, keys := fixtureToken(t, q, nil)
	reauthorized, err := AuthorizeProbe(context.Background(), &fixtureClient{p: q, keys: keys}, q, q.Command(), q.SSHKeyFingerprint, token, q.LoginUID, time.Now())
	if err != nil {
		t.Fatal("second signed fixture rejected")
	}
	if _, err = consumeProbeChallenge(context.Background(), dir, q, reauthorized, opts); !errors.Is(err, ErrChallengeUsed) {
		t.Fatal("same challenge reused through different genuine approval")
	}
}

func TestChallengeRealTimeExpiryBeforeAndDuringFiles(t *testing.T) {
	for _, during := range []bool{false, true} {
		name := "before_files"
		if during {
			name = "after_durable_intent"
		}
		t.Run(name, func(t *testing.T) {
			p, permit, _, _ := challengeFixture(t, func(c map[string]any) { c["exp"] = time.Now().Unix() + 2 })
			dir, opts := challengeTestDir(t)
			wait := func() { time.Sleep(time.Until(permit.expiresAt.Add(20 * time.Millisecond))) }
			if during {
				opts.after = func(stage string) error {
					if stage == "intent_durable" {
						wait()
					}
					return nil
				}
			} else {
				wait()
			}
			if _, err := consumeProbeChallenge(context.Background(), dir, p, permit, opts); !errors.Is(err, ErrChallengeExpiry) {
				t.Fatal("actual expiry bypassed")
			}
			files, e := os.ReadDir(dir)
			want := 0
			if during {
				want = 1
			}
			if e != nil || len(files) != want {
				t.Fatal("expired consumption wrote result or removed durable intent")
			}
		})
	}
}

func TestChallengeFilesystemNoFollowAndOwnership(t *testing.T) {
	p, permit, _, _ := challengeFixture(t, nil)
	for _, kind := range []string{"symlink_directory", "symlink_ancestor", "writable_directory", "writable_ancestor", "wrong_owner", "fifo_state", "symlink_state", "hardlink_state"} {
		t.Run(kind, func(t *testing.T) {
			dir, opts := challengeTestDir(t)
			key := filepath.Join(dir, p.ChallengeSHA256+".intent.json")
			switch kind {
			case "symlink_directory":
				if os.Remove(dir) != nil || os.Symlink(t.TempDir(), dir) != nil {
					t.Fatal("fixture symlink setup")
				}
			case "symlink_ancestor":
				parent := filepath.Dir(dir)
				if os.Remove(dir) != nil || os.Remove(parent) != nil || os.Symlink(t.TempDir(), parent) != nil {
					t.Fatal("fixture ancestor setup")
				}
			case "writable_directory":
				if os.Chmod(dir, 0777) != nil {
					t.Fatal("fixture mode setup")
				}
			case "writable_ancestor":
				if os.Chmod(filepath.Dir(dir), 0777) != nil {
					t.Fatal("fixture mode setup")
				}
			case "wrong_owner":
				opts.owner++
			case "fifo_state":
				if unix.Mkfifo(key, 0600) != nil {
					t.Fatal("fixture FIFO setup")
				}
			case "symlink_state":
				if os.Symlink(filepath.Join(dir, "missing"), key) != nil {
					t.Fatal("fixture symlink setup")
				}
			case "hardlink_state":
				other := filepath.Join(dir, "existing")
				if os.WriteFile(other, []byte("unknown"), 0600) != nil || os.Link(other, key) != nil {
					t.Fatal("fixture link setup")
				}
			}
			started := time.Now()
			if _, err := consumeProbeChallenge(context.Background(), dir, p, permit, opts); err == nil || time.Since(started) > time.Second {
				t.Fatal("unsafe filesystem accepted or FIFO blocked")
			}
		})
	}
}

func TestChallengeReadbackRejectsNonregularLinksAndChangedInode(t *testing.T) {
	dir, opts := challengeTestDir(t)
	dirs, err := openChallengeDirs(dir, opts)
	if err != nil {
		t.Fatal("fixture protected open failed")
	}
	t.Cleanup(func() {
		for _, d := range dirs {
			if unix.Close(d.fd) != nil {
				t.Error("fixture FD close failed")
			}
		}
	})
	fd := dirs[len(dirs)-1].fd
	for _, kind := range []string{"hardlink", "symlink", "fifo", "new_inode", "wrong_mode", "changed_bytes"} {
		t.Run(kind, func(t *testing.T) {
			name := kind + ".json"
			original := []byte("fixed-body-free-state")
			saved, e := writeChallengeFile(fd, name, original, opts, "unused")
			if e != nil {
				t.Fatal("fixture write failed")
			}
			path := filepath.Join(dir, name)
			switch kind {
			case "hardlink":
				if os.Link(path, path+".linked") != nil {
					t.Fatal("fixture hardlink failed")
				}
			case "symlink", "fifo", "new_inode":
				if os.Rename(path, path+".original") != nil {
					t.Fatal("fixture rename failed")
				}
				if kind == "symlink" && os.Symlink(path+".missing", path) != nil {
					t.Fatal("fixture symlink failed")
				}
				if kind == "fifo" && unix.Mkfifo(path, 0600) != nil {
					t.Fatal("fixture FIFO failed")
				}
				if kind == "new_inode" && os.WriteFile(path, original, 0600) != nil {
					t.Fatal("fixture replacement failed")
				}
			case "wrong_mode":
				if os.Chmod(path, 0644) != nil {
					t.Fatal("fixture mode failed")
				}
			case "changed_bytes":
				if os.WriteFile(path, []byte(strings.Repeat("x", len(original))), 0600) != nil {
					t.Fatal("fixture byte change failed")
				}
			}
			if readChallengeFile(fd, saved, original, opts.owner) == nil {
				t.Fatal("unsafe final readback accepted")
			}
		})
	}
}

func TestChallengeFaultsKeepUnknownStateBlocking(t *testing.T) {
	p, permit, _, _ := challengeFixture(t, nil)
	for _, stage := range []string{"intent_created", "intent_durable", "result_created", "result_durable"} {
		t.Run(stage, func(t *testing.T) {
			dir, opts := challengeTestDir(t)
			opts.after = func(s string) error {
				if s == stage {
					return errors.New("private-failure-token-never-export")
				}
				return nil
			}
			if _, err := consumeProbeChallenge(context.Background(), dir, p, permit, opts); !errors.Is(err, ErrChallengeUnknown) || strings.Contains(err.Error(), "private-failure") {
				t.Fatal("unknown durability or privacy failure")
			}
			opts.after = nil
			if _, err := consumeProbeChallenge(context.Background(), dir, p, permit, opts); !errors.Is(err, ErrChallengeUsed) {
				t.Fatal("unknown operation automatically retried")
			}
		})
	}
}

func TestChallengeRejectsDirectoryAndFileReplacement(t *testing.T) {
	p, permit, _, _ := challengeFixture(t, nil)
	for _, kind := range []string{"ancestor", "directory", "result_inode", "result_bytes"} {
		t.Run(kind, func(t *testing.T) {
			dir, opts := challengeTestDir(t)
			opts.after = func(stage string) error {
				if stage != "result_durable" {
					return nil
				}
				if kind == "ancestor" || kind == "directory" {
					target := dir
					if kind == "ancestor" {
						target = filepath.Dir(dir)
					}
					if e := os.Rename(target, target+"-old"); e != nil {
						return e
					}
					return os.MkdirAll(dir, 0700)
				}
				path := filepath.Join(dir, p.ChallengeSHA256+".result.json")
				b := challengeRead(t, path)
				if kind == "result_inode" {
					if e := os.Rename(path, path+".original"); e != nil {
						return e
					}
				}
				if kind == "result_bytes" {
					b = bytes.Repeat([]byte("x"), len(b))
				}
				return os.WriteFile(path, b, 0600)
			}
			if _, err := consumeProbeChallenge(context.Background(), dir, p, permit, opts); !errors.Is(err, ErrChallengeUnknown) {
				t.Fatal("post-write namespace or content replacement accepted")
			}
		})
	}
}

func TestChallengePathsAcceptOnlyExactRootOrProtectedAncestor(t *testing.T) {
	for _, pair := range [][2]string{{"/opt/qs-retirement/state", "/"}, {"/opt/qs-retirement/state", "/opt/qs-retirement"}, {"/opt/qs-retirement", "/opt/qs-retirement"}} {
		if !challengePathsValid(pair[0], pair[1]) {
			t.Fatal("valid protected anchor rejected")
		}
	}
	for _, pair := range [][2]string{{"/opt/qs-retirement/state", "//"}, {"/opt/qs-retirement/state", ""}, {"/opt/qs-retirement/state", "opt"}, {"/opt/qs-retirement/state", "/opt/../opt"}, {"/opt/qs-retirement-else/state", "/opt/qs-retirement"}, {"/opt/qs-retirement/../state", "/"}, {"/opt/qs-retirement/with space", "/"}} {
		if challengePathsValid(pair[0], pair[1]) {
			t.Fatal("unsafe or nonancestor path accepted")
		}
	}
}

func TestChallengeReservationDuringDirectoryOpenRejectsAsUsed(t *testing.T) {
	p, permit, _, _ := challengeFixture(t, nil)
	dir, opts := challengeTestDir(t)
	winnerOpts := opts
	winnerRan := false
	opts.after = func(stage string) error {
		if stage != "directories_opened" {
			return nil
		}
		winnerRan = true
		_, err := consumeProbeChallenge(context.Background(), dir, p, permit, winnerOpts)
		return err
	}
	if _, err := consumeProbeChallenge(context.Background(), dir, p, permit, opts); !errors.Is(err, ErrChallengeUsed) {
		t.Fatal("same-key reservation during protected open became retryable")
	}
	if !winnerRan {
		t.Fatal("protected-open counterexample was not exercised")
	}
}

func TestChallengeConcurrentGoroutinesConsumeExactlyOnce(t *testing.T) {
	p, permit, _, _ := challengeFixture(t, nil)
	dir, opts := challengeTestDir(t)
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := consumeProbeChallenge(context.Background(), dir, p, permit, opts)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	ok, used := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else if errors.Is(err, ErrChallengeUsed) {
			used++
		} else {
			t.Fatal("unexpected concurrency category")
		}
	}
	if ok != 1 || used != 15 {
		t.Fatal("atomic single use failed")
	}
}

type challengeChildInput struct {
	Policy      Policy
	Token       string
	Keys        []byte
	Dir, Anchor string
	Owner       uint32
}

func TestChallengeStoreProcessHelper(t *testing.T) {
	mode := os.Getenv("QS_CHALLENGE_CHILD")
	if mode == "" {
		return
	}
	b, err := os.ReadFile(os.Getenv("QS_CHALLENGE_INPUT"))
	if err != nil {
		os.Exit(35)
	}
	var in challengeChildInput
	if json.Unmarshal(b, &in) != nil {
		os.Exit(35)
	}
	permit, err := AuthorizeProbe(context.Background(), &fixtureClient{p: in.Policy, keys: in.Keys}, in.Policy, in.Policy.Command(), in.Policy.SSHKeyFingerprint, in.Token, in.Policy.LoginUID, time.Now())
	if err != nil {
		os.Exit(35)
	}
	opts := challengeOptions{owner: in.Owner, anchor: in.Anchor}
	if mode == "crash" {
		opts.after = func(stage string) error {
			if stage == "intent_durable" {
				os.Exit(73)
			}
			return nil
		}
	}
	_, err = consumeProbeChallenge(context.Background(), in.Dir, in.Policy, permit, opts)
	if err == nil {
		os.Exit(0)
	}
	if errors.Is(err, ErrChallengeUsed) {
		os.Exit(32)
	}
	os.Exit(34)
}

func challengeChild(t *testing.T, input, mode string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal("fixture test executable unavailable")
	}
	cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestChallengeStoreProcessHelper$")
	// Each child owns its coverage directory; neither missing nor inherited
	// GOCOVERDIR may leak coverage warnings or mix parent/other-child assets.
	coverageDir := t.TempDir()
	if os.Chmod(coverageDir, 0700) != nil {
		t.Fatal("fixture child coverage protection failed")
	}
	cmd.Env = []string{"PATH=/usr/bin:/bin", "GOCOVERDIR=" + coverageDir, "QS_CHALLENGE_CHILD=" + mode, "QS_CHALLENGE_INPUT=" + input}
	cmd.Stdout = &challengeOutput{}
	cmd.Stderr = &challengeOutput{}
	return cmd
}

type challengeOutput struct {
	buffer   bytes.Buffer
	overflow bool
}

func (b *challengeOutput) Write(p []byte) (int, error) {
	if len(p) > 4096-b.buffer.Len() {
		b.overflow = true
		return len(p), nil
	}
	return b.buffer.Write(p)
}

func challengeChildQuiet(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	out, okOut := cmd.Stdout.(*challengeOutput)
	err, okErr := cmd.Stderr.(*challengeOutput)
	if !okOut || !okErr || out.overflow || err.overflow || out.buffer.Len() != 0 || err.buffer.Len() != 0 {
		t.Fatal("child emitted unexpected output; raw output withheld")
	}
}

func TestChallengeChildCoverageAndEnvironmentAreIndependentlyOwned(t *testing.T) {
	parent := t.TempDir()
	t.Setenv("GOCOVERDIR", parent)
	t.Setenv("QS_PRIVATE_SENTINEL", "not-forwarded")
	cmd := challengeChild(t, "private-fixture-input", "consume")
	if len(cmd.Env) != 4 {
		t.Fatal("unexpected child environment")
	}
	for _, value := range cmd.Env {
		if strings.Contains(value, "not-forwarded") || value == "GOCOVERDIR="+parent {
			t.Fatal("parent environment or coverage borrowed")
		}
		if strings.HasPrefix(value, "GOCOVERDIR=") {
			info, err := os.Lstat(strings.TrimPrefix(value, "GOCOVERDIR="))
			if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
				t.Fatal("owned child coverage directory invalid")
			}
		}
	}
}

func TestChallengeRealSubprocessCrashAndReplay(t *testing.T) {
	p, _, token, keys := challengeFixture(t, nil)
	dir, opts := challengeTestDir(t)
	in := challengeChildInput{Policy: p, Token: token, Keys: keys, Dir: dir, Anchor: opts.anchor, Owner: opts.owner}
	input := filepath.Join(opts.anchor, "child-input.private.json")
	if os.WriteFile(input, mustJSON(t, in), 0600) != nil {
		t.Fatal("fixture private input create failed")
	}
	crashed := challengeChild(t, input, "crash")
	err := crashed.Run()
	challengeChildQuiet(t, crashed)
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 73 {
		t.Fatal("actual subprocess crash point not reached")
	}
	if _, err = os.Stat(filepath.Join(dir, p.ChallengeSHA256+".intent.json")); err != nil {
		t.Fatal("durable intent missing after process crash")
	}
	if _, err = os.Stat(filepath.Join(dir, p.ChallengeSHA256+".result.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("crashed process wrote result")
	}
	retried := challengeChild(t, input, "consume")
	err = retried.Run()
	challengeChildQuiet(t, retried)
	if !errors.As(err, &exit) || exit.ExitCode() != 32 {
		t.Fatal("crashed challenge replay adopted")
	}
}

func TestChallengeRealConcurrentProcessesConsumeExactlyOnce(t *testing.T) {
	p, _, token, keys := challengeFixture(t, nil)
	dir, opts := challengeTestDir(t)
	input := filepath.Join(opts.anchor, "child-input.private.json")
	if os.WriteFile(input, mustJSON(t, challengeChildInput{Policy: p, Token: token, Keys: keys, Dir: dir, Anchor: opts.anchor, Owner: opts.owner}), 0600) != nil {
		t.Fatal("fixture private input create failed")
	}
	children := make([]*exec.Cmd, 8)
	for i := range children {
		children[i] = challengeChild(t, input, "consume")
		if children[i].Start() != nil {
			t.Fatal("fixture concurrent process start failed")
		}
	}
	ok, used := 0, 0
	for _, child := range children {
		err := child.Wait()
		challengeChildQuiet(t, child)
		var exit *exec.ExitError
		if err == nil {
			ok++
		} else if errors.As(err, &exit) && exit.ExitCode() == 32 {
			used++
		} else {
			t.Fatal("unexpected subprocess result")
		}
	}
	if ok != 1 || used != 7 {
		t.Fatal("cross-process O_EXCL single use failed")
	}
}

func TestChallengeProductionRootBoundaryNotTestSeam(t *testing.T) {
	p, permit, _, _ := challengeFixture(t, nil)
	dir, _ := challengeTestDir(t)
	if _, err := ConsumeProbeChallenge(context.Background(), dir, p, permit); !errors.Is(err, ErrChallengeStore) {
		t.Fatal("nonroot/test-owned path treated as production root proof")
	}
	dir, opts := challengeTestDir(t)
	for _, ctx := range []context.Context{nil, func() context.Context { c, cancel := context.WithCancel(context.Background()); cancel(); return c }()} {
		if _, err := consumeProbeChallenge(ctx, dir, p, permit, opts); !errors.Is(err, ErrChallengeBinding) {
			t.Fatal("invalid context accepted")
		}
	}
}
