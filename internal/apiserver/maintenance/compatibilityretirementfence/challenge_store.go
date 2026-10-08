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
	"time"

	"golang.org/x/sys/unix"
)

var (
	ErrChallengeStore   = errors.New("retirement_fence_challenge_store_rejected")
	ErrChallengeBinding = errors.New("retirement_fence_challenge_binding_rejected")
	ErrChallengeUsed    = errors.New("retirement_fence_challenge_already_consumed_or_unknown")
	ErrChallengeExpiry  = errors.New("retirement_fence_challenge_expired")
	ErrChallengeUnknown = errors.New("retirement_fence_challenge_durability_unknown")
)

// ChallengeConsumptionReceipt proves only persistence of this probe's challenge
// consumption. It is not a permit for privileged execution or a writer fence.
type ChallengeConsumptionReceipt struct {
	Protocol                 string    `json:"protocol"`
	PolicySHA256             string    `json:"policy_sha256"`
	SourceSHA                string    `json:"source_sha"`
	OperationID              string    `json:"operation_id"`
	RunID                    string    `json:"run_id"`
	RequestSHA256            string    `json:"request_sha256"`
	ChallengeSHA256          string    `json:"challenge_sha256"`
	TokenSHA256              string    `json:"token_sha256"`
	VerifiedAt               time.Time `json:"verified_at"`
	ExpiresAt                time.Time `json:"expires_at"`
	IntentSHA256             string    `json:"intent_sha256"`
	ResultSHA256             string    `json:"result_sha256"`
	ProbeChallengeDurable    bool      `json:"probe_challenge_consumption_durable"`
	ProductionInstalled      bool      `json:"production_installed"`
	PrivilegedExecutorBound  bool      `json:"privileged_executor_bound"`
	WholeSystemWritersFenced bool      `json:"whole_system_writers_fenced"`
	MutationBackendEnabled   bool      `json:"mutation_backend_enabled"`
	DropReady                bool      `json:"drop_ready"`
}

type challengeState struct {
	Protocol        string    `json:"protocol"`
	Kind            string    `json:"kind"`
	PolicySHA256    string    `json:"policy_sha256"`
	SourceSHA       string    `json:"source_sha"`
	OperationID     string    `json:"operation_id"`
	RunID           string    `json:"run_id"`
	RequestSHA256   string    `json:"request_sha256"`
	ChallengeSHA256 string    `json:"challenge_sha256"`
	TokenSHA256     string    `json:"token_sha256"`
	SnapshotSHA256  string    `json:"snapshot_sha256"`
	VerifiedAt      time.Time `json:"verified_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	IntentSHA256    string    `json:"intent_sha256,omitempty"`
}

type challengeOptions struct {
	owner  uint32
	anchor string
	// Only tests can inject a failure. Production never receives these options.
	after func(string) error
}

type challengeDir struct {
	fd, parent int
	name       string
	initial    unix.Stat_t
	final      bool
}

type challengeFile struct {
	name    string
	initial unix.Stat_t
}

// ConsumeProbeChallenge requires a preexisting root-owned 0700 directory and
// root-protected ancestors. The caller supplies the independently approved full
// policy; only an opaque Permit actually returned by AuthorizeProbe is accepted.
// There is no cleanup/adopt/retry API: every existing intent or result blocks.
func ConsumeProbeChallenge(ctx context.Context, rootDir string, approvedPolicy Policy, permit *Permit) (ChallengeConsumptionReceipt, error) {
	if os.Geteuid() != 0 {
		return ChallengeConsumptionReceipt{}, ErrChallengeStore
	}
	return consumeProbeChallenge(ctx, rootDir, approvedPolicy, permit, challengeOptions{owner: 0, anchor: "/"})
}

func challengeValid(ctx context.Context, policy Policy, p *Permit) error {
	if ctx == nil || ctx.Err() != nil || p == nil || !p.verified {
		return ErrChallengeBinding
	}
	now := time.Now()
	if !p.expiresAt.After(now) {
		return ErrChallengeExpiry
	}
	if policy.Validate(now) != nil || p.policyHash != policy.digest() || p.sourceSHA != policy.SourceSHA || p.operation != policy.OperationID || p.run != policy.RunID+"-"+policy.RunAttempt || p.request != policy.RequestSHA256 || p.challengeHash != policy.ChallengeSHA256 || !sha64.MatchString(p.tokenHash) || !sha64.MatchString(p.snapshotHash) || p.workflows < 21 || p.verifiedAt.IsZero() || p.verifiedAt.After(now) || !p.expiresAt.After(p.verifiedAt) || p.expiresAt.After(policy.ExpiresAt) || p.expiresAt.Sub(p.verifiedAt) > 600*time.Second {
		return ErrChallengeBinding
	}
	return nil
}

func consumeProbeChallenge(ctx context.Context, rootDir string, policy Policy, p *Permit, opts challengeOptions) (receipt ChallengeConsumptionReceipt, err error) {
	if err = challengeValid(ctx, policy, p); err != nil {
		return receipt, err
	}
	intentName := p.challengeHash + ".intent.json"
	resultName := p.challengeHash + ".result.json"
	dirs, err := openChallengeDirsForReservation(rootDir, opts, []string{intentName, resultName})
	if err != nil {
		return receipt, err
	}
	defer func() {
		for i := len(dirs) - 1; i >= 0; i-- {
			if unix.Close(dirs[i].fd) != nil {
				receipt = ChallengeConsumptionReceipt{}
				err = ErrChallengeUnknown
			}
		}
	}()
	fd := dirs[len(dirs)-1].fd
	for _, name := range []string{intentName, resultName} {
		var st unix.Stat_t
		e := unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW)
		if e == nil {
			return receipt, ErrChallengeUsed
		}
		if !errors.Is(e, unix.ENOENT) {
			return receipt, ErrChallengeStore
		}
	}
	if checkChallengeDirs(dirs, opts.owner) != nil {
		// A simultaneous consumer may have durably reserved this same key since
		// our first existence check. It remains consumed, never retryable.
		for _, name := range []string{intentName, resultName} {
			var st unix.Stat_t
			if unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW) == nil {
				return receipt, ErrChallengeUsed
			}
		}
		return receipt, ErrChallengeStore
	}
	s := challengeState{Protocol: "probe-challenge-consumption/v1", Kind: "intent", PolicySHA256: p.policyHash, SourceSHA: p.sourceSHA, OperationID: p.operation, RunID: p.run, RequestSHA256: p.request, ChallengeSHA256: p.challengeHash, TokenSHA256: p.tokenHash, SnapshotSHA256: p.snapshotHash, VerifiedAt: p.verifiedAt.UTC(), ExpiresAt: p.expiresAt.UTC()}
	intent, e := json.Marshal(s)
	if e != nil {
		return receipt, ErrChallengeBinding
	}
	intFile, e := writeChallengeFile(fd, intentName, intent, opts, "intent_created")
	if e != nil {
		return receipt, e
	}
	if updateChallengeDirAfterOwnFile(dirs, intFile, opts.owner) != nil || unix.Fsync(fd) != nil || checkChallengeDirs(dirs, opts.owner) != nil || challengeHook(opts, "intent_durable") != nil {
		return receipt, ErrChallengeUnknown
	}
	// File work consumes real validity time. An expired permit leaves its durable
	// intent blocking; it is never replaced by a fresh token or adopted on retry.
	if e = challengeValid(ctx, policy, p); e != nil {
		return receipt, e
	}
	s.Kind = "consumed"
	s.IntentSHA256 = digest(intent)
	result, e := json.Marshal(s)
	if e != nil {
		return receipt, ErrChallengeUnknown
	}
	resFile, e := writeChallengeFile(fd, resultName, result, opts, "result_created")
	if e != nil {
		return receipt, e
	}
	if updateChallengeDirAfterOwnFile(dirs, resFile, opts.owner) != nil || unix.Fsync(fd) != nil || challengeHook(opts, "result_durable") != nil || checkChallengeDirs(dirs, opts.owner) != nil {
		return receipt, ErrChallengeUnknown
	}
	if readChallengeFile(fd, intFile, intent, opts.owner) != nil || readChallengeFile(fd, resFile, result, opts.owner) != nil || checkChallengeDirs(dirs, opts.owner) != nil {
		return receipt, ErrChallengeUnknown
	}
	if e = challengeValid(ctx, policy, p); e != nil {
		return receipt, e
	}
	return ChallengeConsumptionReceipt{Protocol: s.Protocol, PolicySHA256: s.PolicySHA256, SourceSHA: s.SourceSHA, OperationID: s.OperationID, RunID: s.RunID, RequestSHA256: s.RequestSHA256, ChallengeSHA256: s.ChallengeSHA256, TokenSHA256: s.TokenSHA256, VerifiedAt: s.VerifiedAt, ExpiresAt: s.ExpiresAt, IntentSHA256: digest(intent), ResultSHA256: digest(result), ProbeChallengeDurable: true}, nil
}

func challengeHook(opts challengeOptions, stage string) error {
	if opts.after != nil {
		return opts.after(stage)
	}
	return nil
}

func challengeSameInode(a, b unix.Stat_t) bool { return a.Dev == b.Dev && a.Ino == b.Ino }

func challengeDirValid(st unix.Stat_t, owner uint32, final bool) bool {
	return st.Mode&unix.S_IFMT == unix.S_IFDIR && st.Uid == owner && st.Nlink > 0 && st.Mode&0022 == 0 && (!final || st.Mode&07777 == 0700)
}

func challengePathsValid(path, anchor string) bool {
	return protectedPath.MatchString(path) && (anchor == "/" || protectedPath.MatchString(anchor)) && filepath.Clean(path) == path && filepath.Clean(anchor) == anchor && filepath.IsAbs(path) && filepath.IsAbs(anchor) && (path == anchor || anchor == "/" || strings.HasPrefix(path, anchor+string(os.PathSeparator)))
}

func openChallengeDirs(path string, opts challengeOptions) ([]challengeDir, error) {
	return openChallengeDirsForReservation(path, opts, nil)
}

// A reservation can change APFS's directory-entry link count while these FDs
// are being opened. Detecting an existing exact key only selects a rejection
// category: it never accepts the changed directory, reads/adopts state or grants
// execution. Callers without an exact reservation retain the strict open check.
func openChallengeDirsForReservation(path string, opts challengeOptions, keys []string) (dirs []challengeDir, err error) {
	if !challengePathsValid(path, opts.anchor) {
		return nil, ErrChallengeStore
	}
	defer func() {
		if err != nil {
			for _, d := range dirs {
				// Reject remains fixed even if releasing a failed-open FD fails.
				_ = unix.Close(d.fd)
			}
			dirs = nil
		}
	}()
	fd, e := unix.Open(opts.anchor, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return dirs, ErrChallengeStore
	}
	dirs = append(dirs, challengeDir{fd: fd, parent: -1, name: opts.anchor, final: path == opts.anchor})
	if unix.Fstat(fd, &dirs[0].initial) != nil || !challengeDirValid(dirs[0].initial, opts.owner, dirs[0].final) {
		return dirs, ErrChallengeStore
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(path, opts.anchor), "/")
	if rel != "" {
		components := strings.Split(rel, "/")
		for i, name := range components {
			parent := dirs[len(dirs)-1].fd
			child, e := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
			if e != nil {
				return dirs, ErrChallengeStore
			}
			d := challengeDir{fd: child, parent: parent, name: name, final: i == len(components)-1}
			dirs = append(dirs, d)
			last := &dirs[len(dirs)-1]
			if unix.Fstat(child, &last.initial) != nil || !challengeDirValid(last.initial, opts.owner, last.final) {
				return dirs, ErrChallengeStore
			}
		}
	}
	if challengeHook(opts, "directories_opened") != nil {
		return dirs, ErrChallengeStore
	}
	if checkChallengeDirs(dirs, opts.owner) != nil {
		for _, key := range keys {
			var st unix.Stat_t
			if unix.Fstatat(dirs[len(dirs)-1].fd, key, &st, unix.AT_SYMLINK_NOFOLLOW) == nil {
				return dirs, ErrChallengeUsed
			}
		}
		return dirs, ErrChallengeStore
	}
	return dirs, nil
}

func checkChallengeDirs(dirs []challengeDir, owner uint32) error {
	for _, d := range dirs {
		var actual, linked unix.Stat_t
		if unix.Fstat(d.fd, &actual) != nil || !challengeSameInode(d.initial, actual) || actual.Nlink != d.initial.Nlink || !challengeDirValid(actual, owner, d.final) {
			return ErrChallengeStore
		}
		var err error
		if d.parent < 0 {
			err = unix.Lstat(d.name, &linked)
		} else {
			err = unix.Fstatat(d.parent, d.name, &linked, unix.AT_SYMLINK_NOFOLLOW)
		}
		if err != nil || !challengeSameInode(actual, linked) || !challengeDirValid(linked, owner, d.final) || linked.Nlink != d.initial.Nlink {
			return ErrChallengeStore
		}
	}
	return nil
}

// APFS counts regular directory entries in st_nlink; Linux filesystems usually
// do not. Only this known O_EXCL-created, inode-verified regular file may explain
// an unchanged or exactly +1 terminal-directory count. Ancestors never change.
func updateChallengeDirAfterOwnFile(dirs []challengeDir, saved challengeFile, owner uint32) error {
	last := &dirs[len(dirs)-1]
	var current, file unix.Stat_t
	if unix.Fstat(last.fd, &current) != nil || !challengeSameInode(last.initial, current) || !challengeDirValid(current, owner, true) || (current.Nlink != last.initial.Nlink && current.Nlink != last.initial.Nlink+1) || unix.Fstatat(last.fd, saved.name, &file, unix.AT_SYMLINK_NOFOLLOW) != nil || !challengeSameInode(saved.initial, file) || !challengeFileValid(file, owner) {
		return ErrChallengeStore
	}
	last.initial.Nlink = current.Nlink
	return checkChallengeDirs(dirs, owner)
}

func challengeFileValid(st unix.Stat_t, owner uint32) bool {
	return st.Mode&unix.S_IFMT == unix.S_IFREG && st.Uid == owner && st.Nlink == 1 && st.Mode&07777 == 0600
}

func writeChallengeFile(dir int, name string, data []byte, opts challengeOptions, stage string) (saved challengeFile, err error) {
	if len(data) == 0 || len(data) > 4096 {
		return saved, ErrChallengeBinding
	}
	fd, e := unix.Openat(dir, name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if errors.Is(e, unix.EEXIST) {
		return saved, ErrChallengeUsed
	}
	if e != nil {
		return saved, ErrChallengeUnknown
	}
	f := os.NewFile(uintptr(fd), "retirement-challenge-state")
	defer func() {
		if f.Close() != nil {
			err = ErrChallengeUnknown
		}
	}()
	saved.name = name
	if unix.Fstat(fd, &saved.initial) != nil || !challengeFileValid(saved.initial, opts.owner) || challengeHook(opts, stage) != nil {
		return saved, ErrChallengeUnknown
	}
	if n, e := f.Write(data); e != nil || n != len(data) || f.Sync() != nil {
		return saved, ErrChallengeUnknown
	}
	var after, linked unix.Stat_t
	if unix.Fstat(fd, &after) != nil || !challengeFileValid(after, opts.owner) || !challengeSameInode(saved.initial, after) || unix.Fstatat(dir, name, &linked, unix.AT_SYMLINK_NOFOLLOW) != nil || !challengeSameInode(after, linked) || !challengeFileValid(linked, opts.owner) || after.Size != int64(len(data)) {
		return saved, ErrChallengeUnknown
	}
	return saved, nil
}

func readChallengeFile(dir int, expected challengeFile, data []byte, owner uint32) (err error) {
	fd, e := unix.Openat(dir, expected.name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrChallengeUnknown
	}
	f := os.NewFile(uintptr(fd), "retirement-challenge-state")
	defer func() {
		if f.Close() != nil {
			err = ErrChallengeUnknown
		}
	}()
	var before, after, linked unix.Stat_t
	if unix.Fstat(fd, &before) != nil || !challengeFileValid(before, owner) || !challengeSameInode(before, expected.initial) || before.Size != int64(len(data)) {
		return ErrChallengeUnknown
	}
	actual, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil || len(actual) > 4096 || !bytes.Equal(actual, data) || unix.Fstat(fd, &after) != nil || after.Size != before.Size || !challengeSameInode(before, after) || !challengeFileValid(after, owner) || unix.Fstatat(dir, expected.name, &linked, unix.AT_SYMLINK_NOFOLLOW) != nil || linked.Size != after.Size || !challengeSameInode(after, linked) || !challengeFileValid(linked, owner) {
		return ErrChallengeUnknown
	}
	return nil
}
