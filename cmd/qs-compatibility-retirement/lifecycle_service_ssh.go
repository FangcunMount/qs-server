package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// This is the already-authorized SSH management route and its expected file
// identities, not a whole-writer proof or a new login/credential platform.
type lifecycleServiceSSHChannel struct {
	FormatVersion          int    `json:"format_version"`
	Kind                   string `json:"kind"`
	ToolSourceSHA          string `json:"tool_source_sha"`
	OriginalSourceSHA      string `json:"original_source_sha"`
	OperationID            string `json:"operation_id"`
	ManifestSHA256         string `json:"manifest_sha256"`
	OriginalRunID          string `json:"original_run_id"`
	ActualRunID            string `json:"actual_run_id"`
	SSHExecutableSHA256    string `json:"ssh_executable_sha256"`
	Host                   string `json:"host"`
	Port                   int    `json:"port"`
	User                   string `json:"user"`
	IdentitySHA256         string `json:"identity_sha256"`
	KnownHostsSHA256       string `json:"known_hosts_sha256"`
	HostKeyFingerprint     string `json:"host_key_fingerprint"`
	RemoteRequestSHA256    string `json:"remote_request_sha256"`
	RemoteDescriptorSHA256 string `json:"remote_descriptor_sha256"`
}

var lifecycleSSHHostRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)
var lifecycleSSHUserRE = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

func (v lifecycleServiceSSHChannel) bindingMatches(r lifecycleRequest) bool {
	return v.FormatVersion == 1 && ((v.Kind == "qs_existing_pinned_service_ssh_channel" && v.ActualRunID == r.ActualRunID) ||
		(v.Kind == "qs_existing_pinned_service_ssh_channel_template" && v.ActualRunID == "")) &&
		v.ToolSourceSHA == sourceSHA && v.ToolSourceSHA == r.ToolSourceSHA && v.OriginalSourceSHA == r.OriginalSourceSHA &&
		v.OperationID == r.OperationID && v.ManifestSHA256 == r.ManifestSHA256 && v.OriginalRunID == r.Recovery.OriginalRunID &&
		lifecycleSSHHostRE.MatchString(v.Host) && !strings.Contains(v.Host, "..") && lifecycleSSHUserRE.MatchString(v.User) &&
		v.Port >= 1 && v.Port <= 65535 && hashRE.MatchString(v.SSHExecutableSHA256) && hashRE.MatchString(v.IdentitySHA256) &&
		hashRE.MatchString(v.KnownHostsSHA256) && hashRE.MatchString(v.RemoteRequestSHA256) && hashRE.MatchString(v.RemoteDescriptorSHA256)
}

func validateLifecycleServiceChannelAssignedRun(raw []byte, channel lifecycleServiceSSHChannel) error {
	var fields map[string]json.RawMessage
	if rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(channel)) != nil ||
		json.Unmarshal(raw, &fields) != nil || len(fields) != reflect.TypeOf(channel).NumField() {
		return lifecycleError("lifecycle_service_channel_rejected")
	}
	var run any
	if json.Unmarshal(fields["actual_run_id"], &run) != nil || run != channel.ActualRunID {
		return lifecycleError("lifecycle_service_channel_rejected")
	}
	return nil
}

func readLifecycleRootFile(path, expected string, executable bool) ([]byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !hashRE.MatchString(expected) || os.Getuid() != 0 || os.Geteuid() != 0 {
		return nil, lifecycleError("lifecycle_service_channel_rejected")
	}
	for p := path; ; p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		s, ok := infoStat(st)
		if e != nil || !ok || s.Uid != 0 || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&022 != 0 ||
			(p == path && (!st.Mode().IsRegular() || s.Nlink != 1 || !executable && st.Mode().Perm() != 0600 || executable && st.Mode().Perm()&0111 == 0)) ||
			(p != path && !st.IsDir()) {
			return nil, lifecycleError("lifecycle_service_channel_rejected")
		}
		if p == "/" {
			break
		}
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, lifecycleError("lifecycle_service_channel_rejected")
	}
	before, be := f.Stat()
	maximum := int64(1 << 20)
	if executable {
		maximum = 256 << 20
	}
	if be != nil || before == nil || before.Size() < 1 || before.Size() > maximum {
		_ = f.Close()
		return nil, lifecycleError("lifecycle_service_channel_rejected")
	}
	raw, re := io.ReadAll(io.LimitReader(f, maximum+1))
	after, ae := f.Stat()
	ce := f.Close()
	if re != nil || ae != nil || ce != nil || int64(len(raw)) > maximum || !sameLifecycleFile(before, after) || digestRaw(raw) != expected {
		return nil, lifecycleError("lifecycle_service_channel_rejected")
	}
	return raw, nil
}

func lifecycleSSHKnownHostMatches(raw []byte, channel lifecycleServiceSSHChannel) bool {
	// Require one exact, unambiguous target/key in the batch's already pinned
	// known_hosts projection. Wildcards, aliases, certificates and @ markers are
	// not substituted for a verified server key. OpenSSH checks the real peer.
	parts := strings.Fields(strings.TrimSpace(string(raw)))
	host := channel.Host
	if channel.Port != 22 {
		host = "[" + host + "]:" + strconv.Itoa(channel.Port)
	}
	if len(parts) != 3 || parts[0] != host || parts[1] != "ssh-ed25519" && parts[1] != "ssh-rsa" && parts[1] != "ecdsa-sha2-nistp256" {
		return false
	}
	blob, e := base64.StdEncoding.DecodeString(parts[2])
	if e != nil || len(blob) == 0 || len(blob) > 16384 {
		return false
	}
	h := sha256.Sum256(blob)
	return channel.HostKeyFingerprint == "SHA256:"+base64.RawStdEncoding.EncodeToString(h[:])
}

func lifecycleServiceSSHArgs(r lifecycleRequest, channel lifecycleServiceSSHChannel, recovery bool) []string {
	root := lifecycleServicesRoot(r.OperationID, "server-a")
	remote := lifecycleServicesRoot(r.OperationID, "server-d")
	mode := "host-services-d"
	if recovery {
		mode += "-recovery"
	}
	requestName := "service-session.json"
	if channel.Kind == "qs_existing_pinned_service_ssh_channel_template" {
		mode += "-template"
		requestName = "service-session-template.json"
	}
	// Every remote command word is fixed or separately restricted to numeric
	// runs/hex digests. There is no user shell fragment, caller-selected program,
	// environment assignment, target name or arbitrary service action here.
	command := "/usr/bin/sudo -n -- " + filepath.Join(remote, "qs-compatibility-retirement") +
		" --mode " + mode + " --request " + filepath.Join(remote, requestName) +
		" --request-hash " + channel.RemoteRequestSHA256 + " --operation-id " + r.OperationID + " --run-id " + r.ActualRunID
	return []string{"-F", "/dev/null", "-T", "-p", strconv.Itoa(channel.Port), "-l", channel.User,
		"-i", filepath.Join(root, "ssh", "identity"), "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + filepath.Join(root, "ssh", "known_hosts"),
		"-o", "GlobalKnownHostsFile=/dev/null", "-o", "ControlMaster=no", "-o", "ControlPath=none",
		"-o", "ClearAllForwardings=yes", "-o", "PermitLocalCommand=no", "-o", "RequestTTY=no",
		"-o", "ConnectTimeout=15", "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=2", "--", channel.Host, command}
}

// Own the actual SSH process and its pipes. Saved diagnostics are never used to
// construct a live controller. Wait executes once; uncertain remote termination
// or an abnormal child result is an error and leaves the native journals intact.
type lifecycleOwnedServiceSSH struct {
	cmd       *exec.Cmd
	in, out   *os.File
	done      chan struct{}
	waitErr   error
	closeOnce sync.Once
	closeErr  error
	reaped    bool
}

func (v *lifecycleOwnedServiceSSH) requireLive() error {
	if v == nil || v.cmd == nil || v.cmd.Process == nil || v.done == nil || v.in == nil || v.out == nil {
		return lifecycleError("lifecycle_original_live_recovery_session_unknown")
	}
	select {
	case <-v.done:
		return lifecycleError("lifecycle_original_live_recovery_session_unknown")
	default:
		return nil // The next native challenge/action still verifies the live peer.
	}
}

func startLifecycleServiceSSH(ctx context.Context, r lifecycleRequest, path, hash string, recovery bool) (*lifecycleOwnedServiceSSH, lifecycleServiceSSHChannel, error) {
	var channel lifecycleServiceSSHChannel
	if ctx == nil || ctx.Err() != nil || !runRE.MatchString(r.OperationID) || !runRE.MatchString(r.ActualRunID) ||
		path != filepath.Join(lifecycleServicesRoot(r.OperationID, "server-a"), "ssh-channel.json") ||
		readLifecyclePrivate(path, hash, &channel) != nil || !channel.bindingMatches(r) {
		return nil, channel, lifecycleError("lifecycle_service_channel_rejected")
	}
	raw, err := readLifecycleRootFile(path, hash, false)
	if err != nil {
		return nil, channel, err
	}
	if validateLifecycleServiceChannelAssignedRun(raw, channel) != nil {
		return nil, channel, lifecycleError("lifecycle_service_channel_rejected")
	}
	root := lifecycleServicesRoot(r.OperationID, "server-a")
	if _, err := readLifecycleRootFile("/usr/bin/ssh", channel.SSHExecutableSHA256, true); err != nil {
		return nil, channel, err
	}
	key, err := readLifecycleRootFile(filepath.Join(root, "ssh", "identity"), channel.IdentitySHA256, false)
	if err != nil {
		return nil, channel, err
	}
	for i := range key {
		key[i] = 0
	}
	known, err := readLifecycleRootFile(filepath.Join(root, "ssh", "known_hosts"), channel.KnownHostsSHA256, false)
	if err != nil || !lifecycleSSHKnownHostMatches(known, channel) {
		return nil, channel, lifecycleError("lifecycle_service_channel_rejected")
	}
	cmd := exec.Command("/usr/bin/ssh", lifecycleServiceSSHArgs(r, channel, recovery)...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	cmd.Stderr = io.Discard // Authentication errors/hosts/remote output are private.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child, err := startLifecycleOwnedServiceChild(cmd)
	return child, channel, err
}

// Explicit os.Pipe handles are owned by this caller, not exec.Cmd's internal
// parentIOPipes. Wait may observe the D process exit after its terminal reply,
// but must not close the unread reply before RemoteController.Do consumes it.
func startLifecycleOwnedServiceChild(cmd *exec.Cmd) (*lifecycleOwnedServiceSSH, error) {
	if cmd == nil || cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid || cmd.Stdin != nil || cmd.Stdout != nil {
		return nil, lifecycleError("lifecycle_service_channel_failed")
	}
	childIn, writer, err := os.Pipe()
	if err != nil {
		return nil, lifecycleError("lifecycle_service_channel_failed")
	}
	reader, childOut, err := os.Pipe()
	if err != nil {
		_ = childIn.Close()
		_ = writer.Close()
		return nil, lifecycleError("lifecycle_service_channel_failed")
	}
	cmd.Stdin, cmd.Stdout = childIn, childOut
	if err = cmd.Start(); err != nil {
		_ = childIn.Close()
		_ = childOut.Close()
		_ = reader.Close()
		_ = writer.Close()
		return nil, lifecycleError("lifecycle_service_channel_failed")
	}
	child := &lifecycleOwnedServiceSSH{cmd: cmd, in: reader, out: writer, done: make(chan struct{})}
	inClose, outClose := childIn.Close(), childOut.Close()
	go func() { child.waitErr = cmd.Wait(); close(child.done) }()
	if inClose != nil || outClose != nil {
		if cleanupErr := child.closeForRecovery(); cleanupErr != nil {
			return nil, cleanupErr
		}
		return nil, lifecycleError("lifecycle_service_channel_failed")
	}
	return child, nil
}

func (v *lifecycleOwnedServiceSSH) Close() error {
	if v == nil || v.cmd == nil || v.cmd.Process == nil {
		return lifecycleError("lifecycle_service_channel_failed")
	}
	v.closeOnce.Do(func() { v.closeErr = v.close() })
	return v.closeErr
}
func (v *lifecycleOwnedServiceSSH) close() error {
	if v == nil || v.cmd == nil || v.cmd.Process == nil {
		return lifecycleError("lifecycle_service_channel_failed")
	}
	_ = v.out.Close()
	// Successful released D sessions exit normally on EOF. Otherwise terminate
	// only this owned local process group and reap it. This does not assert the
	// remote service state or permit a new stop/DDL/recovery attempt.
	select {
	case <-v.done:
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(-v.cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-v.done:
		case <-time.After(time.Second):
			_ = syscall.Kill(-v.cmd.Process.Pid, syscall.SIGKILL)
			select {
			case <-v.done:
			case <-time.After(5 * time.Second):
				return lifecycleError("lifecycle_service_channel_child_exit_unknown")
			}
		}
	}
	_ = v.in.Close()
	if !errors.Is(syscall.Kill(-v.cmd.Process.Pid, 0), syscall.ESRCH) {
		return lifecycleError("lifecycle_service_channel_child_exit_unknown")
	}
	v.reaped = true
	if v.waitErr != nil {
		return lifecycleError("lifecycle_service_channel_failed")
	}
	return nil
}

func (v *lifecycleOwnedServiceSSH) closeForRecovery() error {
	err := v.Close()
	// A killed/failed SSH client is never called a successful remote action.
	// Once its actual local group is gone and Wait completed, the separate
	// original-window recovery path may re-observe D's physical journal/state.
	if v != nil && v.reaped {
		return nil
	}
	return err
}
