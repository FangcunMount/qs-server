// Package compatibilityretirementstop stops only approved, actually observed QS
// containers. Its opaque Lease is a service-stop observation, never a whole-system
// writer fence or DROP permission. The host owns database handles and deadlines.
package compatibilityretirementstop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	"github.com/FangcunMount/qs-server/pkg/version"
	"golang.org/x/sys/unix"
)

var (
	ErrRollbackAPI = errors.New("retirement_stop_bound_rollback_api_required")
	ErrBinding     = errors.New("retirement_stop_binding_rejected")
	ErrState       = errors.New("retirement_stop_state_changed")
	ErrCommand     = errors.New("retirement_stop_command_failed")
	ErrForced      = errors.New("retirement_stop_graceful_exit_unproven")
	ErrJournal     = errors.New("retirement_stop_journal_rejected")
	ErrRemote      = errors.New("retirement_stop_remote_host_required")
)
var hash64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
var sha40 = regexp.MustCompile(`^[0-9a-f]{40}$`)
var opID = regexp.MustCompile(`^[1-9][0-9]{0,19}-[1-9][0-9]{0,3}$`)

const maxBytes = 4 << 20

// Descriptor is an independently approved inventory, not imported stop proof.
// The actual host identity, container catalog and every listed field are checked
// before any signal. Server B is IAM and is deliberately not a valid role.
type Descriptor struct {
	RemoteDescriptorSHA256 string      `json:"remote_descriptor_sha256,omitempty"`
	BudgetTrustSHA256      string      `json:"budget_trust_sha256,omitempty"`
	Version                int         `json:"version"`
	SourceSHA              string      `json:"source_sha"`
	ToolSourceSHA          string      `json:"tool_source_sha"`
	OriginalRunID          string      `json:"original_run_id"`
	OperationID            string      `json:"operation_id"`
	ManifestSHA256         string      `json:"manifest_sha256"`
	HostRole               string      `json:"host_role"`
	MachineIDSHA256        string      `json:"machine_id_sha256"`
	DockerPath             string      `json:"docker_path"`
	DockerSHA256           string      `json:"docker_sha256"`
	Containers             []Container `json:"containers"`
}
type Container struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Image          string   `json:"image"`
	Component      string   `json:"component"`
	Project        string   `json:"project"`
	Service        string   `json:"service"`
	Entrypoint     []string `json:"entrypoint"`
	Command        []string `json:"command"`
	Running        bool     `json:"running"`
	StartedAt      string   `json:"started_at"`
	RestartPolicy  string   `json:"restart_policy"`
	RestartMaximum int      `json:"restart_maximum"`
}
type Approval struct {
	self       *Approval
	descriptor Descriptor
	rawHash    string
	path       string
	toolMu     sync.Mutex
	toolInfo   os.FileInfo
	materials  *RootRemoteMaterials
}

func (*Approval) MarshalJSON() ([]byte, error) { return nil, ErrBinding }
func (*Approval) String() string               { return "opaque root-approved QS service inventory; no writer fence" }

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func rootProtected(path string, directory bool) error {
	if filepath.Clean(path) != path || !filepath.IsAbs(path) {
		return ErrBinding
	}
	for current := path; ; current = filepath.Dir(current) {
		info, e := os.Lstat(current)
		if e != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
			return ErrBinding
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 {
			return ErrBinding
		}
		if current == path && (directory && !info.IsDir() || !directory && (!info.Mode().IsRegular() || st.Nlink != 1)) {
			return ErrBinding
		}
		if current == "/" {
			break
		}
	}
	return nil
}
func readProtected(path string) ([]byte, error) {
	if rootProtected(path, false) != nil {
		return nil, ErrBinding
	}
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, ErrBinding
	}
	f := os.NewFile(uintptr(fd), "approved-stop-input")
	b, e := io.ReadAll(io.LimitReader(f, maxBytes+1))
	ce := f.Close()
	if e != nil || ce != nil || len(b) > maxBytes {
		return nil, ErrBinding
	}
	return b, nil
}
func hashProtectedExecutable(path string) (string, error) {
	if rootProtected(path, false) != nil {
		return "", ErrBinding
	}
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return "", ErrBinding
	}
	f := os.NewFile(uintptr(fd), "approved-stop-executable")
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, (256<<20)+1))
	ce := f.Close()
	if e != nil || ce != nil || n > 256<<20 {
		return "", ErrBinding
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func exactJSON(b []byte, dst any) error {
	// A duplicate key can change the approved meaning even when unknown fields
	// are forbidden, so reject duplicates recursively before decoding.
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var walk func() error
	walk = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		q, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch q {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				s, ok := k.(string)
				if !ok || seen[s] {
					return ErrBinding
				}
				seen[s] = true
				if e = walk(); e != nil {
					return e
				}
			}
			t, e = d.Token()
			if e != nil || t != json.Delim('}') {
				return ErrBinding
			}
		case '[':
			for d.More() {
				if e = walk(); e != nil {
					return e
				}
			}
			t, e = d.Token()
			if e != nil || t != json.Delim(']') {
				return ErrBinding
			}
		default:
			return ErrBinding
		}
		return nil
	}
	if walk() != nil {
		return ErrBinding
	}
	if _, e := d.Token(); !errors.Is(e, io.EOF) {
		return ErrBinding
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil {
		return ErrBinding
	}
	if d.Decode(new(any)) != io.EOF {
		return ErrBinding
	}
	return nil
}
func validContainer(c Container, role string) bool {
	if !hash64.MatchString(c.ID) || !strings.HasPrefix(c.Image, "sha256:") || !hash64.MatchString(strings.TrimPrefix(c.Image, "sha256:")) || len(c.StartedAt) == 0 || c.RestartPolicy != "unless-stopped" || c.RestartMaximum != 0 {
		return false
	}
	switch c.Component {
	case "qs-apiserver":
		return role == "server-a" && c.Name == "/qs-apiserver" && c.Service == "qs-apiserver" && reflect.DeepEqual(c.Entrypoint, []string{"/app/qs-apiserver"}) && reflect.DeepEqual(c.Command, []string{"--config=/app/configs/apiserver.prod.yaml"})
	case "qs-collection-server":
		return role == "server-a" && c.Project == "qs-collection" && c.Service == "server" && reflect.DeepEqual(c.Entrypoint, []string{"/app/collection-server"}) && reflect.DeepEqual(c.Command, []string{"--config=/app/configs/collection-server.prod.yaml"})
	case "qs-worker":
		return role == "server-d" && c.Project == "qs-worker" && c.Service == "runtime" && reflect.DeepEqual(c.Entrypoint, []string{"/app/qs-worker"}) && reflect.DeepEqual(c.Command, []string{"--config=/app/configs/worker.prod.yaml"})
	}
	return false
}
func validDescriptor(d Descriptor) bool {
	if d.RemoteDescriptorSHA256 != "" && (d.HostRole != "server-a" || !hash64.MatchString(d.RemoteDescriptorSHA256)) || d.BudgetTrustSHA256 != "" && (d.HostRole != "server-d" || !hash64.MatchString(d.BudgetTrustSHA256)) {
		return false
	}
	if d.Version != 1 || !sha40.MatchString(d.SourceSHA) || !sha40.MatchString(d.ToolSourceSHA) || !opID.MatchString(d.OriginalRunID) || !opID.MatchString(d.OperationID) || !hash64.MatchString(d.ManifestSHA256) || !hash64.MatchString(d.MachineIDSHA256) || !hash64.MatchString(d.DockerSHA256) || (d.DockerPath != "/usr/bin/docker" && d.DockerPath != "/usr/local/bin/docker") || len(d.Containers) < 1 || len(d.Containers) > 32 {
		return false
	}
	seen := map[string]bool{}
	api, collection, worker := 0, 0, 0
	for _, c := range d.Containers {
		if seen[c.ID] || !validContainer(c, d.HostRole) {
			return false
		}
		seen[c.ID] = true
		switch c.Component {
		case "qs-apiserver":
			api++
		case "qs-collection-server":
			collection++
		case "qs-worker":
			worker++
		}
	}
	return d.HostRole == "server-a" && api == 1 && collection >= 1 && worker == 0 || d.HostRole == "server-d" && api == 0 && collection == 0 && worker >= 1
}
func ReadApprovedDescriptor(path, approvedSHA256 string) (*Approval, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || !hash64.MatchString(approvedSHA256) {
		return nil, ErrBinding
	}
	b, e := readProtected(path)
	if e != nil || digest(b) != approvedSHA256 {
		return nil, ErrBinding
	}
	var d Descriptor
	if exactJSON(b, &d) != nil || !validDescriptor(d) {
		return nil, ErrBinding
	}
	a := &Approval{descriptor: d, rawHash: approvedSHA256, path: path}
	a.self = a
	return a, nil
}
func (a *Approval) validate() error {
	if a == nil || a.self != a || version.GitCommit != a.descriptor.ToolSourceSHA || runtime.GOOS != "linux" || os.Geteuid() != 0 || !validDescriptor(a.descriptor) {
		return ErrBinding
	}
	machine, e := os.ReadFile("/etc/machine-id")
	if e != nil || digest(bytes.TrimSpace(machine)) != a.descriptor.MachineIDSHA256 {
		return ErrRemote
	}
	policy, e := readProtected(a.path)
	if e != nil || digest(policy) != a.rawHash {
		return ErrBinding
	}
	a.toolMu.Lock()
	info, e := os.Lstat(a.descriptor.DockerPath)
	if e != nil || rootProtected(a.descriptor.DockerPath, false) != nil {
		a.toolMu.Unlock()
		return ErrBinding
	}
	if a.toolInfo == nil {
		h, e := hashProtectedExecutable(a.descriptor.DockerPath)
		after, ae := os.Lstat(a.descriptor.DockerPath)
		if e != nil || ae != nil || h != a.descriptor.DockerSHA256 || !os.SameFile(info, after) || info.Size() != after.Size() || info.ModTime() != after.ModTime() {
			a.toolMu.Unlock()
			return ErrBinding
		}
		a.toolInfo = after
	} else if !os.SameFile(info, a.toolInfo) || info.Size() != a.toolInfo.Size() || info.ModTime() != a.toolInfo.ModTime() {
		a.toolMu.Unlock()
		return ErrBinding
	}
	a.toolMu.Unlock()
	socket, e := os.Lstat("/run/docker.sock")
	if e != nil || socket.Mode()&os.ModeSocket == 0 || socket.Mode()&os.ModeSymlink != 0 {
		return ErrBinding
	}
	st, ok := socket.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 {
		return ErrBinding
	}
	return nil
}

type boundedOutput struct {
	b        bytes.Buffer
	overflow bool
}

func (v *boundedOutput) Write(b []byte) (int, error) {
	n := len(b)
	remain := maxBytes - v.b.Len()
	if remain > 0 {
		if remain > n {
			remain = n
		}
		_, _ = v.b.Write(b[:remain])
	}
	if remain < n {
		v.overflow = true
	}
	return n, nil
}
func (a *Approval) docker(ctx context.Context, args ...string) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || a.validate() != nil {
		return nil, ErrBinding
	}
	cmd := exec.CommandContext(ctx, a.descriptor.DockerPath, append([]string{"--host", "unix:///run/docker.sock"}, args...)...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	var out, errOut boundedOutput
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	e := cmd.Run()
	if e != nil || out.overflow || errOut.overflow || ctx.Err() != nil {
		return nil, ErrCommand
	}
	return bytes.Clone(out.b.Bytes()), nil
}

const inspectFormat = `{"id":{{json .Id}},"name":{{json .Name}},"image":{{json .Image}},"component":{{json (index .Config.Labels "prometheus.component")}},"project":{{json (index .Config.Labels "com.docker.compose.project")}},"service":{{json (index .Config.Labels "com.docker.compose.service")}},"entrypoint":{{json .Config.Entrypoint}},"command":{{json .Config.Cmd}},"running":{{json .State.Running}},"started_at":{{json .State.StartedAt}},"restart_policy":{{json .HostConfig.RestartPolicy.Name}},"restart_maximum":{{json .HostConfig.RestartPolicy.MaximumRetryCount}},"status":{{json .State.Status}},"exit_code":{{json .State.ExitCode}},"oom_killed":{{json .State.OOMKilled}},"dead":{{json .State.Dead}},"paused":{{json .State.Paused}},"restarting":{{json .State.Restarting}},"pid":{{json .State.Pid}}}`

type actualContainer struct {
	Container
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	OOMKilled  bool   `json:"oom_killed"`
	Dead       bool   `json:"dead"`
	Paused     bool   `json:"paused"`
	Restarting bool   `json:"restarting"`
	PID        int    `json:"pid"`
}

func (a *Approval) inspect(ctx context.Context, id string) (actualContainer, error) {
	var v actualContainer
	if !hash64.MatchString(id) {
		return v, ErrBinding
	}
	b, e := a.docker(ctx, "inspect", "--format", inspectFormat, id)
	if e != nil || exactJSON(bytes.TrimSpace(b), &v) != nil || v.ID != id {
		return v, ErrCommand
	}
	return v, nil
}

// DatabasePrincipal is a credential-free selection from the actual original
// container environment. It is not an ownership approval or writer fence.
type DatabasePrincipal struct {
	ContainerID       string `json:"container_id"`
	Component         string `json:"component"`
	EnvironmentSHA256 string `json:"environment_sha256"`
	SQLUser           string `json:"mysql_user"`
	SQLDatabase       string `json:"mysql_database"`
	MongoUser         string `json:"mongodb_user"`
	MongoDatabase     string `json:"mongodb_database"`
}

func (l *Lease) ObserveDatabasePrincipals(ctx context.Context) ([]DatabasePrincipal, error) {
	if l == nil || l.self != l || l.Check(ctx) != nil {
		return nil, ErrBinding
	}
	out := []DatabasePrincipal{}
	for _, want := range l.baseline {
		prefix := ""
		switch want.Component {
		case "qs-apiserver":
			prefix = "QS_APISERVER_"
		case "qs-worker":
			prefix = "QS_WORKER_"
		case "qs-collection-server":
			continue // Collection uses the original API, not a DB credential.
		default:
			return nil, ErrBinding
		}
		// The deployed fixed config argument cannot override these env fields via
		// command-line DB flags. Missing actual env values never use YAML guesses.
		if len(want.Command) != 1 || !strings.HasPrefix(want.Command[0], "--config=") {
			return nil, ErrBinding
		}
		before, e := l.approval.inspect(ctx, want.ID)
		if e != nil || !sameIdentity(before, want) || before.Running || before.PID != 0 {
			return nil, ErrBinding
		}
		raw, e := l.approval.docker(ctx, "inspect", "--format", "{{json .Config.Env}}", want.ID)
		var env []string
		if e != nil || json.Unmarshal(bytes.TrimSpace(raw), &env) != nil || len(raw) > 32768 {
			return nil, ErrBinding
		}
		v, e := databasePrincipalEnvironment(want, prefix, env)
		if e != nil {
			return nil, e
		}
		again, e := l.approval.docker(ctx, "inspect", "--format", "{{json .Config.Env}}", want.ID)
		after, ae := l.approval.inspect(ctx, want.ID)
		if e != nil || ae != nil || !bytes.Equal(raw, again) || !reflect.DeepEqual(before, after) {
			return nil, ErrState
		}
		out = append(out, v)
	}
	if len(out) == 0 || l.Check(ctx) != nil {
		return nil, ErrBinding
	}
	return out, nil
}

func databasePrincipalEnvironment(want Container, prefix string, env []string) (DatabasePrincipal, error) {
	v := DatabasePrincipal{ContainerID: want.ID, Component: want.Component, EnvironmentSHA256: digest([]byte(strings.Join(env, "\x00")))}
	values := map[string]string{}
	for _, item := range env {
		k, value, ok := strings.Cut(item, "=")
		if !ok || strings.ContainsAny(item, "\x00\r\n") {
			return v, ErrBinding
		}
		if _, exists := values[k]; exists {
			return v, ErrBinding
		}
		values[k] = value
	}
	v.SQLUser, v.SQLDatabase = values[prefix+"MYSQL_USERNAME"], values[prefix+"MYSQL_DATABASE"]
	v.MongoUser, v.MongoDatabase = values[prefix+"MONGODB_USERNAME"], values[prefix+"MONGODB_DATABASE"]
	if v.SQLUser == "" || v.SQLDatabase == "" || v.MongoUser == "" || v.MongoDatabase == "" || values[prefix+"MONGODB_URL"] != "" {
		return v, ErrBinding
	}
	return v, nil
}
func relevant(v actualContainer) bool {
	return v.Component == "qs-apiserver" || v.Component == "qs-collection-server" || v.Component == "qs-worker" || v.Name == "/qs-apiserver" || v.Name == "/qs-worker" || v.Name == "/qs-collection-server" || v.Project == "qs-collection" || v.Project == "qs-worker"
}
func (a *Approval) catalog(ctx context.Context) ([]actualContainer, error) {
	b, e := a.docker(ctx, "ps", "--all", "--quiet", "--no-trunc")
	if e != nil {
		return nil, e
	}
	ids := strings.Fields(string(b))
	if len(ids) > 512 {
		return nil, ErrBinding
	}
	seen := map[string]bool{}
	out := []actualContainer{}
	for _, id := range ids {
		if seen[id] {
			return nil, ErrState
		}
		seen[id] = true
		v, e := a.inspect(ctx, id)
		if e != nil {
			return nil, e
		}
		if relevant(v) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func sameIdentity(got actualContainer, want Container) bool {
	copy := got.Container
	copy.Running = want.Running
	copy.StartedAt = want.StartedAt
	return reflect.DeepEqual(copy, want)
}

// Lease is returned even after a failed stop once the immutable baseline exists.
// That lets the host restore the exact original running set under its recovery
// context. An interrupted/failed stop can never satisfy Check.
type Lease struct {
	self                                *Lease
	mu                                  sync.Mutex
	approval                            *Approval
	dir                                 string
	dirFD                               int
	baseline                            []Container
	stopped                             map[string]bool
	restored                            map[string]string
	failed                              bool
	closed                              bool
	window                              serviceWindow
	controlledIssued, controlledResumed bool
	runtimeObservation                  *DependentRuntimeObservation
}

func (*Lease) MarshalJSON() ([]byte, error) { return nil, ErrBinding }
func (*Lease) String() string               { return "opaque actual QS service stop lease; no global writer fence" }
func (l *Lease) record(name string, v any) (result error) {
	defer func() {
		if result != nil {
			l.approval.materials.markUnknown()
		}
	}()
	b, e := json.Marshal(v)
	if e != nil {
		return ErrJournal
	}
	b = append(b, '\n')
	fd, e := unix.Openat(l.dirFD, name, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return ErrJournal
	}
	f := os.NewFile(uintptr(fd), "service-stop-journal")
	n, e := f.Write(b)
	if e == nil && n != len(b) {
		e = ErrJournal
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil || syscall.Fsync(l.dirFD) != nil {
		return ErrJournal
	}
	return l.approval.materials.registerWritten("service-journal/"+name, b)
}
func openJournal(dir string) (int, error) {
	if rootProtected(dir, true) != nil {
		return -1, ErrJournal
	}
	info, e := os.Lstat(dir)
	if e != nil || info.Mode().Perm() != 0700 {
		return -1, ErrJournal
	}
	fd, e := syscall.Open(dir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return -1, ErrJournal
	}
	if syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		_ = syscall.Close(fd)
		return -1, ErrJournal
	}
	return fd, nil
}

// WindowBinding exposes only the root-approved expected original batch binding.
// It is not a stopped-service, whole-writer, restoration or DROP capability.
// The real window is checked independently before every service operation.
func (a *Approval) WindowBinding(ctx context.Context) (fence.WindowBinding, error) {
	if ctx == nil || ctx.Err() != nil || a.validate() != nil {
		return fence.WindowBinding{}, ErrBinding
	}
	return fence.WindowBinding{TargetSHA256: fence.MaintenanceWindowTargetSHA256(), SourceSHA: a.descriptor.SourceSHA, OperationID: a.descriptor.OperationID, ManifestSHA256: a.descriptor.ManifestSHA256, OriginalRunID: a.descriptor.OriginalRunID}, nil
}

type serviceWindow interface {
	Diagnostic(context.Context) (fence.WindowBudgetReceipt, error)
	ForwardContext(context.Context) (context.Context, context.CancelFunc, error)
	RecoveryContext(context.Context) (context.Context, context.CancelFunc, error)
}

func checkWindow(ctx context.Context, a *Approval, w serviceWindow) error {
	if a == nil || a.self != a || w == nil || ctx == nil || ctx.Err() != nil {
		return ErrBinding
	}
	d, e := w.Diagnostic(ctx)
	want := fence.WindowBinding{TargetSHA256: fence.MaintenanceWindowTargetSHA256(), SourceSHA: a.descriptor.SourceSHA, OperationID: a.descriptor.OperationID, ManifestSHA256: a.descriptor.ManifestSHA256, OriginalRunID: a.descriptor.OriginalRunID}
	if e != nil || d.Binding != want || !d.BudgetOnly || !d.DirectoryLeaseHeld || d.RemainingMilliseconds <= 0 {
		return ErrBinding
	}
	return nil
}
func StopAndDrain(ctx context.Context, a *Approval, journalDir string, window *fence.MaintenanceWindow) (*Lease, error) {
	return stopAndDrainWithBudget(ctx, a, journalDir, window)
}

func stopAndDrainWithBudget(ctx context.Context, a *Approval, journalDir string, window serviceWindow) (*Lease, error) {
	if ctx == nil || ctx.Err() != nil || a.validate() != nil {
		return nil, ErrBinding
	}
	if checkWindow(ctx, a, window) != nil {
		return nil, ErrBinding
	}
	bounded, cancel, e := window.ForwardContext(ctx)
	if e != nil {
		return nil, ErrBinding
	}
	defer cancel()
	ctx = bounded
	actual, e := a.catalog(ctx)
	if e != nil || len(actual) != len(a.descriptor.Containers) {
		return nil, ErrState
	}
	expected := append([]Container(nil), a.descriptor.Containers...)
	sort.Slice(expected, func(i, j int) bool { return expected[i].ID < expected[j].ID })
	for i, v := range actual {
		if !reflect.DeepEqual(v.Container, expected[i]) || v.Paused || v.Restarting || v.Dead || v.OOMKilled || v.Running && v.PID <= 0 {
			return nil, ErrState
		}
	}
	fd, e := openJournal(journalDir)
	if e != nil {
		return nil, e
	}
	l := &Lease{approval: a, window: window, dir: journalDir, dirFD: fd, baseline: expected, stopped: map[string]bool{}, restored: map[string]string{}}
	l.self = l
	if l.record("baseline.json", struct {
		ApprovalSHA256 string     `json:"approval_sha256"`
		Descriptor     Descriptor `json:"descriptor"`
	}{a.rawHash, a.descriptor}) != nil {
		_ = l.Close()
		return nil, ErrJournal
	}
	// Collection ingress drains first. Parent launches A and D in parallel so
	// worker handlers and apiserver GracefulStop retain their dependencies.
	ordered := append([]Container(nil), expected...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Component == "qs-collection-server" && ordered[j].Component != "qs-collection-server"
	})
	for _, v := range ordered {
		if !v.Running {
			continue
		}
		if e = l.stop(ctx, v); e != nil {
			l.failed = true
			return l, e
		}
	}
	if e = l.check(ctx); e != nil {
		l.failed = true
		return l, e
	}
	return l, nil
}

// OpenRecovery reads an existing protected baseline and checks actual host/container
// identity. It can restore service state but always has failed=true, so imported
// journal data can never reconstruct a fresh StopAndDrain / DROP qualification.
func OpenRecovery(ctx context.Context, a *Approval, journalDir string, window *fence.MaintenanceWindow) (*Lease, error) {
	return openRecoveryWithBudget(ctx, a, journalDir, window)
}

func openRecoveryWithBudget(ctx context.Context, a *Approval, journalDir string, window serviceWindow) (*Lease, error) {
	if ctx == nil || ctx.Err() != nil || a.validate() != nil {
		return nil, ErrBinding
	}
	if checkWindow(ctx, a, window) != nil {
		return nil, ErrBinding
	}
	fd, e := openJournal(journalDir)
	if e != nil {
		return nil, e
	}
	l := &Lease{approval: a, window: window, dir: journalDir, dirFD: fd, failed: true, stopped: map[string]bool{}, restored: map[string]string{}}
	l.self = l
	fail := func() (*Lease, error) { _ = l.Close(); return nil, ErrJournal }
	b, e := readProtected(filepath.Join(journalDir, "baseline.json"))
	if e != nil {
		return fail()
	}
	var baseline struct {
		ApprovalSHA256 string     `json:"approval_sha256"`
		Descriptor     Descriptor `json:"descriptor"`
	}
	if exactJSON(b, &baseline) != nil || baseline.ApprovalSHA256 != a.rawHash || !reflect.DeepEqual(baseline.Descriptor, a.descriptor) {
		return fail()
	}
	l.baseline = append([]Container(nil), baseline.Descriptor.Containers...)
	sort.Slice(l.baseline, func(i, j int) bool { return l.baseline[i].ID < l.baseline[j].ID })
	for _, v := range l.baseline {
		name := filepath.Join(journalDir, v.ID+".restore-result.json")
		if _, e = os.Lstat(name); os.IsNotExist(e) {
			continue
		}
		b, e = readProtected(name)
		if e != nil {
			return fail()
		}
		var result struct {
			ID        string `json:"id"`
			StartedAt string `json:"started_at"`
		}
		if exactJSON(b, &result) != nil || result.ID != v.ID || result.StartedAt == "" {
			return fail()
		}
		l.restored[v.ID] = result.StartedAt
	}
	actual, e := a.catalog(ctx)
	if e != nil || len(actual) != len(l.baseline) {
		return fail()
	}
	for i, v := range actual {
		if !sameIdentity(v, l.baseline[i]) || v.Paused || v.Restarting || v.Dead {
			return fail()
		}
	}
	return l, nil
}

func (l *Lease) stop(ctx context.Context, v Container) error {
	actual, e := l.approval.inspect(ctx, v.ID)
	if e != nil || !reflect.DeepEqual(actual.Container, v) {
		return ErrState
	}
	seconds := 510
	if v.Component == "qs-collection-server" {
		seconds = 60
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) < time.Duration(seconds+15)*time.Second {
		return ErrBinding
	}
	if l.record(v.ID+".stop-intent.json", struct {
		ID      string `json:"id"`
		Timeout int    `json:"timeout_seconds"`
	}{v.ID, seconds}) != nil {
		return ErrJournal
	}
	_, e = l.approval.docker(ctx, "stop", "--time", fmt.Sprint(seconds), v.ID)
	if e != nil {
		_ = l.record(v.ID+".stop-unknown.json", struct {
			Result string `json:"result"`
		}{"command_result_unknown"})
		return e
	}
	actual, e = l.approval.inspect(ctx, v.ID)
	if e != nil || !sameIdentity(actual, v) || actual.Running || actual.PID != 0 || actual.Restarting || actual.Paused || actual.Dead || actual.OOMKilled || actual.ExitCode != 0 {
		return ErrForced
	}
	if l.record(v.ID+".stop-result.json", struct {
		ID       string `json:"id"`
		ExitCode int    `json:"exit_code"`
	}{v.ID, actual.ExitCode}) != nil {
		return ErrJournal
	}
	l.stopped[v.ID] = true
	return nil
}
func (l *Lease) check(ctx context.Context) error {
	if l == nil || checkWindow(ctx, l.approval, l.window) != nil {
		return ErrBinding
	}
	if l == nil || l.self != l || l.closed || l.failed || ctx == nil || ctx.Err() != nil {
		return ErrState
	}
	actual, e := l.approval.catalog(ctx)
	if e != nil || len(actual) != len(l.baseline) {
		return ErrState
	}
	for i, v := range actual {
		original := l.baseline[i]
		if !sameIdentity(v, original) || v.StartedAt != original.StartedAt || v.Running || v.PID != 0 || v.Paused || v.Restarting || v.Dead || v.OOMKilled || original.Running && (!l.stopped[v.ID] || v.ExitCode != 0) {
			return ErrState
		}
	}
	return nil
}

// Read-only checks use only the epoch already held by the original caller.
// This never begins recovery merely because a request chooses an action name.
func observedServiceContext(ctx context.Context, w serviceWindow) (context.Context, context.CancelFunc, error) {
	if w == nil || ctx == nil || ctx.Err() != nil {
		return nil, nil, ErrBinding
	}
	d, e := w.Diagnostic(ctx)
	if e != nil || !d.DirectoryLeaseHeld || d.RemainingMilliseconds <= 0 {
		return nil, nil, ErrBinding
	}
	if d.RecoverySHA256 != "" {
		return w.RecoveryContext(ctx)
	}
	return w.ForwardContext(ctx)
}

func (l *Lease) Check(ctx context.Context) error {
	if l == nil || l.self != l {
		return ErrBinding
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	bounded, cancel, e := observedServiceContext(ctx, l.window)
	if e != nil {
		return ErrBinding
	}
	defer cancel()
	return l.check(bounded)
}

// Restore starts only the original running CID/Image set and never recreates,
// removes, deploys, or starts an originally stopped container. A replaced CID,
// changed image/config, unexpected QS container, or concurrent restart blocks.
func (l *Lease) Restore(ctx context.Context) error {
	if e := l.restoreScope(ctx, true, true); e != nil {
		return e
	}
	// Production A starts automatic migrations. Never docker start its old API
	// during recovery. An unchanged already-running original API needs no action;
	// otherwise the host must deploy its separately bound no-migration rollback.
	if l.approval.descriptor.HostRole != "server-a" {
		return nil
	}
	for _, original := range l.baseline {
		if original.Component != "qs-apiserver" {
			continue
		}
		v, e := l.approval.inspect(ctx, original.ID)
		if e != nil || !sameIdentity(v, original) || v.Running != original.Running || v.StartedAt != original.StartedAt || v.Dead || v.Restarting || v.Paused || original.Running && v.PID <= 0 {
			return ErrRollbackAPI
		}
		// A still-running original API can have an in-flight daemon Stop after
		// a disconnected caller. Its actual protected journal must be settled;
		// unchanged Running/StartedAt alone is not successful recovery evidence.
		if e := l.noPendingStop(v); e != nil {
			return e
		}
	}
	return nil
}

// RestoreDependents restores only original Collection / Worker instances. The
// caller must verify the separately deployed B or rollback API and its startup
// binding before reopening traffic; this method never adopts an API replacement
// as whole-service or whole-writer proof. It permits the known API identity to be
// replaced while preserving every dependent's original CID/Image/config/state.
func (l *Lease) RestoreDependents(ctx context.Context) error { return l.restoreScope(ctx, true, true) }

// ResumeDependents is the normal B-success handoff. It stays in the original
// forward window and does not switch/cancel the caller's forward context. The
// caller must have actually accepted B before reopening the external ingress.
func (l *Lease) ResumeDependents(ctx context.Context) error { return l.restoreScope(ctx, true, false) }

// Restore / RestoreDependents require the original parent context, never an
// already-derived forward context: beginning RecoveryContext cancels forwards.
func validDependentScopeAPI(v Container) bool {
	c := v
	if reflect.DeepEqual(c.Command, []string{"--config=/app/configs/apiserver.prod.yaml", "--migration.enabled=false"}) ||
		reflect.DeepEqual(c.Command, []string{"--config=/app/configs/apiserver.prod.yaml", "--migration.enabled=true"}) {
		c.Command = []string{"--config=/app/configs/apiserver.prod.yaml"}
	}
	return validContainer(c, "server-a")
}

// A disconnected Docker stop can outlive its CLI. A still-running original
// process with an issued intent and no completed result must never be declared
// restored: the pending daemon stop could later take it down. The parent must
// observe its actual stopped/PID-zero state before explicit recovery continues.
func (l *Lease) noPendingStop(v actualContainer) error {
	intentPath := filepath.Join(l.dir, v.ID+".stop-intent.json")
	resultPath := filepath.Join(l.dir, v.ID+".stop-result.json")
	_, intentErr := os.Lstat(intentPath)
	_, resultErr := os.Lstat(resultPath)
	if os.IsNotExist(intentErr) {
		if !os.IsNotExist(resultErr) {
			return ErrJournal
		}
		return nil
	}
	if intentErr != nil {
		return ErrJournal
	}
	rawIntent, e := readProtected(intentPath)
	if e != nil {
		return ErrJournal
	}
	if os.IsNotExist(resultErr) {
		return validateStopCompletion(rawIntent, nil, true, v)
	}
	if resultErr != nil {
		return ErrJournal
	}
	rawResult, e := readProtected(resultPath)
	if e != nil {
		return ErrJournal
	}
	return validateStopCompletion(rawIntent, rawResult, false, v)
}
func validateStopCompletion(rawIntent, rawResult []byte, resultMissing bool, v actualContainer) error {
	var intent struct {
		ID      string `json:"id"`
		Timeout int    `json:"timeout_seconds"`
	}
	seconds := 510
	if v.Component == "qs-collection-server" {
		seconds = 60
	}
	if exactJSON(rawIntent, &intent) != nil || intent.ID != v.ID || intent.Timeout != seconds {
		return ErrJournal
	}
	if resultMissing {
		if v.Running || v.PID != 0 {
			return ErrState
		}
		return nil
	}
	var result struct {
		ID       string `json:"id"`
		ExitCode int    `json:"exit_code"`
	}
	if exactJSON(rawResult, &result) != nil || result.ID != v.ID || result.ExitCode != 0 {
		return ErrJournal
	}
	return nil
}

func (l *Lease) restoreScope(ctx context.Context, dependentsOnly bool, recovery bool) error {
	if l == nil || l.self != l {
		return ErrBinding
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || ctx == nil || ctx.Err() != nil || checkWindow(ctx, l.approval, l.window) != nil {
		return ErrBinding
	}
	var bounded context.Context
	var cancel context.CancelFunc
	var e error
	if recovery {
		bounded, cancel, e = l.window.RecoveryContext(ctx)
	} else {
		bounded, cancel, e = l.window.ForwardContext(ctx)
	}
	if e != nil {
		return ErrBinding
	}
	defer cancel()
	ctx = bounded
	actual, e := l.approval.catalog(ctx)
	baseline := l.baseline
	if dependentsOnly {
		baseline = nil
		for _, v := range l.baseline {
			if v.Component != "qs-apiserver" {
				baseline = append(baseline, v)
			}
		}
		kept := []actualContainer{}
		apis := 0
		for _, v := range actual {
			if v.Component == "qs-apiserver" {
				apis++
				if !validDependentScopeAPI(v.Container) {
					return ErrState
				}
			} else {
				kept = append(kept, v)
			}
		}
		if apis > 1 {
			return ErrState
		}
		actual = kept
	}
	if e != nil || len(actual) != len(baseline) {
		return ErrState
	}
	for i, v := range actual {
		if e := l.noPendingStop(v); e != nil {
			return e
		}
		if !sameIdentity(v, baseline[i]) || v.Paused || v.Restarting || v.Dead {
			return ErrState
		}
		if v.Running && !baseline[i].Running {
			return ErrState
		}
		if v.Running && v.StartedAt != baseline[i].StartedAt && l.restored[v.ID] != v.StartedAt {
			return ErrState
		}
	}
	for _, original := range baseline {
		if !original.Running {
			continue
		}
		v, e := l.approval.inspect(ctx, original.ID)
		if e != nil || !sameIdentity(v, original) {
			return ErrState
		}
		if v.Running {
			continue
		}
		if l.record(v.ID+".restore-intent.json", struct {
			ID string `json:"id"`
		}{v.ID}) != nil {
			return ErrJournal
		}
		if _, e = l.approval.docker(ctx, "start", v.ID); e != nil {
			return e
		}
		v, e = l.approval.inspect(ctx, v.ID)
		if e != nil || !sameIdentity(v, original) || !v.Running || v.PID <= 0 || v.Restarting || v.Dead || v.OOMKilled {
			return ErrState
		}
		if l.record(v.ID+".restore-result.json", struct {
			ID        string `json:"id"`
			StartedAt string `json:"started_at"`
		}{v.ID, v.StartedAt}) != nil {
			return ErrJournal
		}
		l.restored[v.ID] = v.StartedAt
	}
	return nil
}
func (l *Lease) Close() error {
	if l == nil || l.self != l {
		return ErrBinding
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if l.dirFD < 0 {
		return ErrJournal
	}
	e := syscall.Close(l.dirFD)
	l.dirFD = -1
	if e != nil {
		l.approval.materials.markUnknown()
		return ErrJournal
	}
	return nil
}

// Diagnostic describes actual observation scope without granting permission.
type Diagnostic struct {
	HostRole               string    `json:"host_role"`
	Containers             int       `json:"containers"`
	Stopped                int       `json:"stopped"`
	OriginalRunning        int       `json:"original_running"`
	Failed                 bool      `json:"failed"`
	WholeWriterFenceProven bool      `json:"whole_writer_fence_proven"`
	RequiredHosts          []string  `json:"required_hosts"`
	ObservedAt             time.Time `json:"observed_at"`
}

func (l *Lease) Diagnostic(ctx context.Context) (Diagnostic, error) {
	if l == nil || l.self != l {
		return Diagnostic{}, ErrBinding
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if e := l.check(ctx); e != nil {
		return Diagnostic{}, e
	}
	d := Diagnostic{HostRole: l.approval.descriptor.HostRole, Containers: len(l.baseline), Stopped: len(l.stopped), RequiredHosts: []string{"server-a", "server-d"}, ObservedAt: time.Now().UTC()}
	for _, v := range l.baseline {
		if v.Running {
			d.OriginalRunning++
		}
	}
	return d, nil
}
