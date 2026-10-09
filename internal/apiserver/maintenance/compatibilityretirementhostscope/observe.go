// Package compatibilityretirementhostscope observes the real root host on the
// caller's existing channel. An observation is never an installation lease,
// authentication verdict, historical-rerun fence or database write permission.
package compatibilityretirementhostscope

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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/pkg/version"
	"golang.org/x/sys/unix"
)

const Protocol = "qs-retirement-root-writer-scope/v1"
const Budget = 120 * time.Second
const fileLimit = 2 << 20
const totalLimit = 32 << 20
const itemLimit = 32768

var ErrRoot = errors.New("host_scope_actual_linux_root_required")
var ErrBinding = errors.New("host_scope_binding_rejected")
var ErrIncomplete = errors.New("host_scope_observation_incomplete")
var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var runPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}-[1-9][0-9]{0,3}$`)
var unitPattern = regexp.MustCompile(`^[A-Za-z0-9_.@:-]{1,200}\.(service|timer|socket|path|target)$`)

// Binding identifies the actual caller and approved observation request. It
// contains no filesystem paths, command arguments or claimed completeness.
// The existing authenticated root session, not these fields, supplies authority.
type Binding struct {
	SourceSHA      string `json:"source_sha"`
	OperationID    string `json:"operation_id"`
	RunID          string `json:"run_id"`
	HostRole       string `json:"host_role"`
	RequestSHA256  string `json:"request_sha256"`
	ApprovalSHA256 string `json:"approval_sha256"`
}

func (b Binding) Valid() bool {
	return shaPattern.MatchString(b.SourceSHA) && runPattern.MatchString(b.OperationID) && runPattern.MatchString(b.RunID) && (b.HostRole == "server_a" || b.HostRole == "server_d") && hashPattern.MatchString(b.RequestSHA256) && hashPattern.MatchString(b.ApprovalSHA256)
}

type File struct {
	Role           string `json:"role"`
	PathSHA256     string `json:"path_sha256"`
	ContentSHA256  string `json:"content_sha256"`
	Bytes          int64  `json:"bytes"`
	UID            uint32 `json:"uid"`
	GID            uint32 `json:"gid"`
	Links          uint64 `json:"links"`
	IdentitySHA256 string `json:"identity_sha256"`
	Mode           uint32 `json:"mode"`
	SourceStable   bool   `json:"source_stable"`
}
type Process struct {
	PID                  int    `json:"pid"`
	ParentPID            int    `json:"parent_pid"`
	UID                  uint32 `json:"uid"`
	StartTicks           uint64 `json:"start_ticks"`
	Role                 string `json:"role"`
	ExecutablePathSHA256 string `json:"executable_path_sha256"`
	ExecutableSHA256     string `json:"executable_sha256"`
	CgroupSHA256         string `json:"cgroup_sha256"`
	NamespaceSHA256      string `json:"namespace_sha256"`
}
type Container struct {
	ID               string `json:"id"`
	ImageID          string `json:"image_id"`
	PID              int    `json:"pid"`
	LabelsSHA256     string `json:"labels_sha256"`
	EntrypointSHA256 string `json:"entrypoint_sha256"`
	CommandSHA256    string `json:"command_sha256"`
	RestartSHA256    string `json:"restart_sha256"`
	MountsSHA256     string `json:"mounts_sha256"`
}
type Entry struct {
	Kind           string `json:"kind"`
	IdentitySHA256 string `json:"identity_sha256"`
	SubjectSHA256  string `json:"subject_sha256"`
	UID            uint32 `json:"uid"`
	SourceSHA256   string `json:"source_sha256"`
	DetailsSHA256  string `json:"details_sha256"`
}
type Scope struct {
	Name                string   `json:"name"`
	EnumerationComplete bool     `json:"enumeration_complete"`
	RecheckEqual        bool     `json:"recheck_equal"`
	Items               int      `json:"items"`
	CatalogSHA256       string   `json:"catalog_sha256"`
	Unknown             []string `json:"unknown"`
}
type Observation struct {
	Protocol            string          `json:"protocol"`
	Binding             Binding         `json:"binding"`
	ObservedUID         uint32          `json:"observed_uid"`
	MachineIDSHA256     string          `json:"machine_id_sha256"`
	BootIDSHA256        string          `json:"boot_id_sha256"`
	NamespaceSHA256     string          `json:"namespace_sha256"`
	ObservedAt          string          `json:"observed_at"`
	ElapsedMillis       int64           `json:"elapsed_millis"`
	ObservationComplete bool            `json:"observation_complete"`
	WriterScopeComplete bool            `json:"writer_scope_complete"`
	Files               []File          `json:"files"`
	Processes           []Process       `json:"processes"`
	Containers          []Container     `json:"containers"`
	Entries             []Entry         `json:"entries"`
	Scopes              []Scope         `json:"scopes"`
	Unknown             []string        `json:"unknown"`
	Capabilities        map[string]bool `json:"capabilities"`
}

// SHA256 is the exact JSON digest of the body-free observation. It does not
// deserialize a proof or certify an external writer scope.
func (o Observation) CatalogSHA256() string {
	return jsonHash(struct {
		Machine, Boot, Namespace string
		Files                    []File
		Processes                []Process
		Entries                  []Entry
	}{o.MachineIDSHA256, o.BootIDSHA256, o.NamespaceSHA256, o.Files, o.Processes, o.Entries})
}
func (o Observation) SHA256() string {
	b, e := json.Marshal(o)
	if e != nil {
		return ""
	}
	return sum(b)
}
func sum(b []byte) string   { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func jsonHash(v any) string { b, _ := json.Marshal(v); return sum(b) }
func capabilities() map[string]bool {
	return map[string]bool{"management_channel_authenticated": false, "production_installed": false, "sshd_reloaded": false, "writer_fence_proven": false, "historical_rerun_denied": false, "execution_authority": false, "cas_ready": false, "drop_ready": false}
}

// Observe uses actual UID/EUID, Linux proc and a fixed command/read set. There is
// no exported alternate root, fake command runner or JSON-to-authority path.
func Observe(ctx context.Context, b Binding) (Observation, error) {
	if runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 {
		return Observation{}, ErrRoot
	}
	if ctx == nil || !b.Valid() || !shaPattern.MatchString(version.Get().GitCommit) || b.SourceSHA != version.Get().GitCommit {
		return Observation{}, ErrBinding
	}
	bounded, cancel := context.WithTimeout(ctx, Budget)
	defer cancel()
	o := observer{root: "/", uid: 0, ctx: bounded, files: map[string]witness{}, directories: map[string][]string{}, binaries: map[string]string{}, commandHashes: map[string]string{}, absent: map[string]bool{}, links: map[string]string{}, argvHashes: map[int]string{}}
	o.command = o.nativeCommand
	return o.observe(b)
}

type witness struct {
	fact File
	info os.FileInfo
	path string
}
type observer struct {
	root          string
	uid           uint32
	ctx           context.Context
	bytes         int64
	binaryBytes   int64
	binaries      map[string]string
	absent        map[string]bool
	links         map[string]string
	argvHashes    map[int]string
	commandHashes map[string]string
	files         map[string]witness
	directories   map[string][]string
	command       func(string, ...string) ([]byte, error)
	out           Observation
	accounts      []account
}
type account struct {
	name, home, shell string
	uid               uint32
}

func (o *observer) physical(p string) (string, error) {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n") || strings.Contains(p, "//") {
		return "", ErrBinding
	}
	if o.root == "/" {
		return p, nil
	}
	return filepath.Join(o.root, strings.TrimPrefix(p, "/")), nil
}
func same(a, b os.FileInfo) bool {
	if a == nil || b == nil || a.Size() != b.Size() || a.ModTime() != b.ModTime() || a.Mode() != b.Mode() {
		return false
	}
	x, ok := a.Sys().(*syscall.Stat_t)
	y, other := b.Sys().(*syscall.Stat_t)
	if !ok || !other || x.Dev != y.Dev || x.Ino != y.Ino || x.Uid != y.Uid || x.Gid != y.Gid || x.Nlink != y.Nlink || x.Mode != y.Mode {
		return false
	}
	return reflect.DeepEqual(changeTime(x), changeTime(y))
}
func changeTime(s *syscall.Stat_t) any {
	v := reflect.ValueOf(s).Elem()
	for _, n := range []string{"Ctim", "Ctimespec"} {
		if f := v.FieldByName(n); f.IsValid() {
			return f.Interface()
		}
	}
	return nil
}

func uidOf(info os.FileInfo) (uint32, bool) {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return s.Uid, true
}
func (o *observer) gap(scope, category string) {
	for i := range o.out.Scopes {
		if o.out.Scopes[i].Name == scope {
			o.out.Scopes[i].Unknown = appendUnique(o.out.Scopes[i].Unknown, category)
			return
		}
	}
}
func appendUnique(a []string, s string) []string {
	for _, v := range a {
		if v == s {
			return a
		}
	}
	return append(a, s)
}
func (o *observer) scope(name string) *Scope {
	for i := range o.out.Scopes {
		if o.out.Scopes[i].Name == name {
			return &o.out.Scopes[i]
		}
	}
	panic("closed scope")
}
func (o *observer) read(path, role string, cap int64) ([]byte, error) {
	if o.ctx.Err() != nil {
		return nil, ErrIncomplete
	}
	p, e := o.physical(path)
	if e != nil {
		return nil, e
	}
	name := filepath.Base(path)
	if name == "shadow" || name == "gshadow" || name == "environ" || name == ".env" || strings.HasSuffix(name, ".env") || strings.HasPrefix(name, "id_") || (strings.HasPrefix(path, "/etc/ssh/ssh_host_") && !strings.HasSuffix(path, ".pub")) {
		return nil, ErrBinding
	}
	f, check, closeParents, e := openObservedFile(p, false)
	if e != nil {
		return nil, ErrIncomplete
	}
	defer o.closeParents(closeParents)
	defer o.closeObserved(f)
	before, e := f.Stat()
	if e != nil || !before.Mode().IsRegular() {
		return nil, ErrIncomplete
	}
	if s, ok := before.Sys().(*syscall.Stat_t); !ok || s.Nlink != 1 {
		return nil, ErrIncomplete
	}
	b, e := io.ReadAll(io.LimitReader(f, cap+1))
	after, x := f.Stat()
	named, y := f.Stat()
	if e != nil || x != nil || y != nil || int64(len(b)) > cap || (role != "observer_binary" && o.bytes+int64(len(b)) > totalLimit) || (role == "observer_binary" && o.binaryBytes+int64(len(b)) > 512<<20) || !same(before, after) || !same(before, named) || check() != nil || o.ctx.Err() != nil {
		return nil, ErrIncomplete
	}
	if role == "observer_binary" {
		o.binaryBytes += int64(len(b))
	} else {
		o.bytes += int64(len(b))
	}
	uid, ok := uidOf(before)
	if !ok {
		return nil, ErrIncomplete
	}
	fact := File{Role: role, PathSHA256: sum([]byte(path)), ContentSHA256: sum(b), Bytes: int64(len(b)), UID: uid, Mode: uint32(before.Sys().(*syscall.Stat_t).Mode), GID: before.Sys().(*syscall.Stat_t).Gid, Links: uint64(before.Sys().(*syscall.Stat_t).Nlink), IdentitySHA256: jsonHash([]any{before.Sys().(*syscall.Stat_t).Dev, before.Sys().(*syscall.Stat_t).Ino}), SourceStable: true}
	if w, ok := o.files[path]; ok && (!same(w.info, before) || w.fact.ContentSHA256 != fact.ContentSHA256) {
		return nil, ErrIncomplete
	}
	o.files[path] = witness{fact: fact, info: before, path: p}
	return b, nil
}
func (o *observer) list(path string) ([]string, error) {
	p, e := o.physical(path)
	if e != nil || o.ctx.Err() != nil {
		return nil, ErrIncomplete
	}
	before, e := os.Lstat(p)
	if e != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, ErrIncomplete
	}
	f, check, closeParents, e := openObservedFile(p, true)
	if e != nil {
		return nil, ErrIncomplete
	}
	defer o.closeParents(closeParents)
	defer o.closeObserved(f)
	items, e := f.Readdirnames(itemLimit + 1)
	if e != io.EOF && e != nil {
		return nil, ErrIncomplete
	}
	if extra, end := f.Readdirnames(1); len(extra) != 0 || end != io.EOF {
		return nil, ErrIncomplete
	}
	if len(items) > itemLimit {
		return nil, ErrIncomplete
	}
	after, e := f.Stat()
	named, x := os.Lstat(p)
	if e != nil || x != nil || !same(before, after) || !same(before, named) || check() != nil {
		return nil, ErrIncomplete
	}
	sort.Strings(items)
	if prior, ok := o.directories[path]; ok && !reflect.DeepEqual(prior, items) {
		return nil, ErrIncomplete
	}
	o.directories[path] = items
	return items, nil
}
func protectedExecutable(path string) (os.FileInfo, error) {
	for d := path; ; d = filepath.Dir(d) {
		i, e := os.Lstat(d)
		if e != nil || i.Mode()&os.ModeSymlink != 0 || i.Mode().Perm()&022 != 0 {
			return nil, ErrIncomplete
		}
		u, ok := uidOf(i)
		if !ok || u != 0 {
			return nil, ErrIncomplete
		}
		if d == path && !i.Mode().IsRegular() {
			return nil, ErrIncomplete
		}
		if d == "/" {
			return i, nil
		}
	}
}
func (o *observer) nativeCommand(kind string, args ...string) ([]byte, error) {
	var path string
	var tail []string
	switch kind {
	case "nss_passwd":
		path = "/usr/bin/getent"
		tail = []string{"passwd"}
	case "nss_group":
		path = "/usr/bin/getent"
		tail = []string{"group"}
	case "sessions":
		path = "/usr/bin/loginctl"
		tail = []string{"list-sessions", "--no-legend", "--no-pager"}
	case "units":
		path = "/usr/bin/systemctl"
		tail = []string{"list-units", "--all", "--type=service,timer,socket,path,target", "--no-legend", "--plain", "--no-pager"}
	case "unit_files":
		path = "/usr/bin/systemctl"
		tail = []string{"list-unit-files", "--type=service,timer,socket,path,target", "--no-legend", "--no-pager"}
	case "docker_list":
		if len(args) != 0 {
			return nil, ErrBinding
		}
		path = "/usr/bin/docker"
		tail = []string{"--host", "unix:///run/docker.sock", "ps", "-aq", "--no-trunc"}
	case "docker_inspect":
		if len(args) < 1 || len(args) > 512 {
			return nil, ErrBinding
		}
		for _, id := range args {
			if !hashPattern.MatchString(id) {
				return nil, ErrBinding
			}
		}
		path = "/usr/bin/docker"
		tail = append([]string{"--host", "unix:///run/docker.sock", "inspect", "--format", `{"id":{{json .Id}},"image":{{json .Image}},"pid":{{json .State.Pid}},"labels":{{json .Config.Labels}},"entrypoint":{{json .Config.Entrypoint}},"command":{{json .Config.Cmd}},"restart":{{json .HostConfig.RestartPolicy}},"mounts":{{json .Mounts}}}`}, args...)
	case "unit":
		if len(args) < 1 || len(args) > 512 {
			return nil, ErrBinding
		}
		for _, arg := range args {
			if !unitPattern.MatchString(arg) {
				return nil, ErrBinding
			}
		}
		path = "/usr/bin/systemctl"
		tail = append([]string{"show", "--no-pager", "--property=Id,ActiveState,SubState,MainPID,FragmentPath,DropInPaths,User,Group,Transient,Triggers,TriggeredBy"}, args...)
	default:
		return nil, ErrBinding
	}
	if kind != "unit" && kind != "docker_inspect" && len(args) != 0 {
		return nil, ErrBinding
	}
	if strings.HasPrefix(kind, "docker_") {
		info, e := os.Lstat("/run/docker.sock")
		if e != nil || info.Mode()&os.ModeSocket == 0 || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrIncomplete
		}
		uid, ok := uidOf(info)
		if !ok || uid != 0 {
			return nil, ErrIncomplete
		}
		for _, parent := range []string{"/run", "/"} {
			if _, e := protectedExecutableDirectory(parent); e != nil {
				return nil, ErrIncomplete
			}
		}
	}
	if _, e := protectedExecutable(path); e != nil {
		return nil, e
	}
	// Hash each fixed executable once. Every invocation checks the actual
	// named identity, then the final epoch hashes its complete bytes again.
	if _, ok := o.files[path]; !ok {
		if _, e := o.read(path, "observer_binary", 128<<20); e != nil {
			return nil, e
		}
	}
	before := o.files[path].info
	ctx, cancel := context.WithTimeout(o.ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, tail...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat"}
	var out cappedBuffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if cmd.Run() != nil || ctx.Err() != nil {
		return nil, ErrIncomplete
	}
	after, e := os.Lstat(path)
	if e != nil || !same(before, after) {
		return nil, ErrIncomplete
	}
	return out.Bytes(), nil
}

type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > fileLimit {
		return 0, ErrIncomplete
	}
	return b.Buffer.Write(p)
}
func (o *observer) entry(kind string, uid uint32, subject, identity, source, details string) {
	o.out.Entries = append(o.out.Entries, Entry{Kind: kind, UID: uid, SubjectSHA256: sum([]byte(subject)), IdentitySHA256: sum([]byte(identity)), SourceSHA256: source, DetailsSHA256: sum([]byte(details))})
}
func (o *observer) observe(b Binding) (Observation, error) {
	started := time.Now()
	o.out = Observation{Protocol: Protocol, Binding: b, ObservedUID: o.uid, ObservedAt: started.UTC().Format(time.RFC3339Nano), Files: []File{}, Processes: []Process{}, Entries: []Entry{}, Scopes: []Scope{}, Unknown: []string{}, Capabilities: capabilities()}
	for _, name := range []string{"ssh_configuration_sources", "nss_accounts_and_key_sources", "existing_sessions_and_processes", "local_activation_sources"} {
		o.out.Scopes = append(o.out.Scopes, Scope{Name: name, Unknown: []string{}})
	}
	for _, p := range []struct {
		path, role string
		dst        *string
	}{{"/etc/machine-id", "machine_id", &o.out.MachineIDSHA256}, {"/proc/sys/kernel/random/boot_id", "boot_id", &o.out.BootIDSHA256}} {
		raw, e := o.read(p.path, p.role, fileLimit)
		if e != nil {
			o.out.Unknown = appendUnique(o.out.Unknown, "host_identity_read_unknown")
		} else {
			*p.dst = sum(raw)
		}
	}
	if ns, e := o.namespace("/proc/self/ns"); e == nil {
		o.out.NamespaceSHA256 = ns
	} else {
		o.out.Unknown = appendUnique(o.out.Unknown, "host_namespace_read_unknown")
	}
	o.observeAccounts()
	o.observeProcesses()
	o.observeSSH()
	o.observeActivation()
	o.observeContainers()
	// Every readable source is reread and every directory is enumerated again.
	// Errors, disappearance or changed bytes retain the partial catalog, never EOF.
	for path, w := range o.files {
		raw, e := o.read(path, w.fact.Role, max(fileLimit, w.fact.Bytes))
		if e != nil || sum(raw) != w.fact.ContentSHA256 {
			o.out.Unknown = appendUnique(o.out.Unknown, "source_end_recheck_changed_or_unread")
		}
	}
	o.recheckProcesses()
	for key, hash := range o.commandHashes {
		parts := strings.Split(key, "\x00")
		raw, e := o.command(parts[0], parts[1:]...)
		if e != nil || sum(raw) != hash {
			o.out.Unknown = appendUnique(o.out.Unknown, "native_command_end_recheck_changed_or_unread")
		}
	}
	for path := range o.absent {
		p, e := o.physical(path)
		if e != nil {
			o.out.Unknown = appendUnique(o.out.Unknown, "absent_source_end_recheck_unknown")
			continue
		}
		if _, e := os.Lstat(p); !os.IsNotExist(e) {
			o.out.Unknown = appendUnique(o.out.Unknown, "absent_source_end_recheck_changed_or_unread")
		}
	}
	for path, link := range o.links {
		p, e := o.physical(path)
		actual, x := os.Readlink(p)
		if e != nil || x != nil || actual != link {
			o.out.Unknown = appendUnique(o.out.Unknown, "symlink_source_end_recheck_changed_or_unread")
		}
	}
	for pid, hash := range o.argvHashes {
		raw, e := o.procRead(fmt.Sprintf("/proc/%d/cmdline", pid))
		if e != nil || sum(raw) != hash {
			o.out.Unknown = appendUnique(o.out.Unknown, "sshd_argv_end_recheck_changed_or_unread")
		}
	}
	for path, prior := range o.directories {
		current, e := o.list(path)
		if e != nil || !reflect.DeepEqual(prior, current) {
			o.out.Unknown = appendUnique(o.out.Unknown, "directory_end_recheck_changed_or_unread")
		}
	}
	sort.Slice(o.out.Processes, func(i, j int) bool { return o.out.Processes[i].PID < o.out.Processes[j].PID })
	sort.Slice(o.out.Entries, func(i, j int) bool { return jsonHash(o.out.Entries[i]) < jsonHash(o.out.Entries[j]) })
	for _, w := range o.files {
		o.out.Files = append(o.out.Files, w.fact)
	}
	sort.Slice(o.out.Files, func(i, j int) bool { return o.out.Files[i].PathSHA256 < o.out.Files[j].PathSHA256 })
	for i := range o.out.Scopes {
		s := &o.out.Scopes[i]
		sort.Strings(s.Unknown)
		s.RecheckEqual = len(o.out.Unknown) == 0
		s.CatalogSHA256 = jsonHash(struct {
			Name    string
			Catalog string
		}{s.Name, o.out.CatalogSHA256()})
		s.EnumerationComplete = s.EnumerationComplete && s.RecheckEqual
	}
	o.out.Unknown = appendUnique(o.out.Unknown, "external_database_and_qs_ai_writers_not_observed")
	o.out.Unknown = appendUnique(o.out.Unknown, "writer_admission_and_historical_platform_fence_not_installed")
	sort.Strings(o.out.Unknown)
	o.out.ElapsedMillis = time.Since(started).Milliseconds()
	o.out.ObservationComplete = o.ctx.Err() == nil && o.out.MachineIDSHA256 != "" && o.out.BootIDSHA256 != "" && o.out.NamespaceSHA256 != ""
	for _, s := range o.out.Scopes {
		o.out.ObservationComplete = o.out.ObservationComplete && s.EnumerationComplete
	}
	// WriterScopeComplete and all capability bits intentionally remain false.
	if !o.out.ObservationComplete {
		return o.out, ErrIncomplete
	}
	return o.out, nil
}
func (o *observer) namespace(path string) (string, error) {
	p, e := o.physical(path)
	if e != nil {
		return "", e
	}
	var v []string
	for _, n := range []string{"mnt", "pid", "user", "net"} {
		s, e := os.Readlink(filepath.Join(p, n))
		if e != nil || !regexp.MustCompile(`^[a-z]+:\[[0-9]+\]$`).MatchString(s) {
			return "", ErrIncomplete
		}
		v = append(v, n+":"+s)
	}
	return jsonHash(v), nil
}
func accountLines(raw []byte) ([]account, error) {
	var rows []account
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, ":")
		if len(f) != 7 || !regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`).MatchString(f[0]) || seen[f[0]] {
			return nil, ErrIncomplete
		}
		u, e := strconv.ParseUint(f[2], 10, 32)
		if e != nil {
			return nil, ErrIncomplete
		}
		seen[f[0]] = true
		rows = append(rows, account{name: f[0], uid: uint32(u), home: f[5], shell: f[6]})
	}
	return rows, nil
}
func accountFingerprint(rows []account) string {
	v := make([][]any, 0, len(rows))
	for _, r := range rows {
		v = append(v, []any{r.name, r.uid, r.home, r.shell})
	}
	return jsonHash(v)
}
func (o *observer) observeAccounts() {
	scope := "nss_accounts_and_key_sources"
	finiteComplete := true
	// Finite read/schema failures differ from the retained semantic gaps below.
	o.scope(scope).EnumerationComplete = false
	local, e := o.read("/etc/passwd", "local_accounts", fileLimit)
	if e != nil {
		o.gap(scope, "local_accounts_unread")
		return
	}
	accounts, e := accountLines(local)
	if e != nil {
		o.gap(scope, "local_accounts_schema_unknown")
		return
	}
	o.accounts = accounts
	for _, a := range accounts {
		o.entry("local_account", a.uid, a.name, a.name, sum(local), a.home+"\x00"+a.shell)
	}
	nss, e := o.read("/etc/nsswitch.conf", "nss_sources", fileLimit)
	if e != nil {
		o.gap(scope, "nss_sources_unread")
		return
	}
	for _, line := range strings.Split(string(nss), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		f := strings.SplitN(line, ":", 2)
		if len(f) != 2 || (f[0] != "passwd" && f[0] != "group" && f[0] != "shadow") {
			continue
		}
		o.entry("nss_source", 0, f[0], f[0], sum(nss), strings.TrimSpace(f[1]))
		for _, token := range strings.Fields(f[1]) {
			if token != "files" {
				o.gap(scope, "external_or_conditional_nss_backend_not_exhaustively_proven")
			}
		}
	}
	for _, kind := range []string{"nss_passwd", "nss_group"} {
		raw, e := o.observedCommand(kind)
		if e != nil {
			o.gap(scope, "native_nss_enumeration_unread")
			finiteComplete = false
			continue
		}
		o.entry("native_nss_enumeration", 0, kind, kind, sum(raw), "finite_command_eof")
		if kind == "nss_passwd" {
			remote, e := accountLines(raw)
			if e != nil {
				o.gap(scope, "native_nss_accounts_schema_unknown")
				finiteComplete = false
			} else if accountFingerprint(remote) != accountFingerprint(accounts) {
				o.gap(scope, "native_nss_differs_from_local_accounts")
			}
		}
	}
	if _, e = o.read("/etc/group", "local_groups", fileLimit); e != nil {
		o.gap(scope, "local_groups_unread")
		finiteComplete = false
	}
	for _, a := range accounts {
		for _, name := range []string{".profile", ".bash_profile", ".bashrc", ".zshrc", ".zprofile", ".zshenv", ".ssh/rc", ".ssh/authorized_keys"} {
			if !filepath.IsAbs(a.home) || filepath.Clean(a.home) != a.home {
				o.gap(scope, "account_home_source_unsupported")
				finiteComplete = false
				continue
			}
			p := filepath.Join(a.home, name)
			physical, _ := o.physical(p)
			if _, e := os.Lstat(physical); os.IsNotExist(e) {
				o.absent[p] = true
				continue
			}
			raw, e := o.read(p, "account_startup_or_public_key_source", fileLimit)
			if e != nil {
				o.gap(scope, "account_startup_or_public_key_unread")
				finiteComplete = false
				continue
			}
			o.entry("account_startup_or_public_key", a.uid, a.name, p, sum(raw), name)
			if name != ".ssh/authorized_keys" {
				o.gap(scope, "account_startup_indirect_execution_not_proven")
			}
		}
	}
	for _, p := range []string{"/etc/pam.conf", "/etc/pam.d/sshd", "/etc/pam.d/common-auth", "/etc/pam.d/common-account", "/etc/pam.d/common-session"} {
		physical, _ := o.physical(p)
		if _, e := os.Lstat(physical); os.IsNotExist(e) {
			o.absent[p] = true
			continue
		}
		raw, e := o.read(p, "pam_authentication_source", fileLimit)
		if e != nil {
			o.gap(scope, "pam_authentication_source_unread")
			finiteComplete = false
		} else {
			o.entry("pam_authentication_source", 0, p, p, sum(raw), "body_never_persisted")
		}
	}
	o.gap(scope, "pam_and_dynamic_authentication_modules_not_exhaustively_proven")
	o.gap(scope, "public_key_options_and_certificate_semantics_not_proven")
	o.scope(scope).Items = len(accounts)
	o.scope(scope).EnumerationComplete = finiteComplete
}
func parseProcStat(raw []byte) (pid, ppid int, start uint64, e error) {
	s := string(raw)
	n := strings.IndexByte(s, '(')
	end := strings.LastIndex(s, ") ")
	if n < 1 || end < n {
		return 0, 0, 0, ErrIncomplete
	}
	pid, e = strconv.Atoi(strings.TrimSpace(s[:n]))
	f := strings.Fields(s[end+2:])
	if e != nil || len(f) < 20 {
		return 0, 0, 0, ErrIncomplete
	}
	ppid, e = strconv.Atoi(f[1])
	if e != nil {
		return
	}
	start, e = strconv.ParseUint(f[19], 10, 64)
	return
}
func parseUID(raw []byte) (uint32, error) {
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "Uid:") {
			f := strings.Fields(line)
			if len(f) != 5 {
				return 0, ErrIncomplete
			}
			n, e := strconv.ParseUint(f[1], 10, 32)
			return uint32(n), e
		}
	}
	return 0, ErrIncomplete
}
func (o *observer) procRead(path string) ([]byte, error) {
	p, e := o.physical(path)
	if e != nil {
		return nil, e
	}
	f, e := os.OpenFile(p, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, e
	}
	defer o.closeObserved(f)
	b, e := io.ReadAll(io.LimitReader(f, 64<<10))
	if e != nil || len(b) == 64<<10 || o.ctx.Err() != nil {
		return nil, ErrIncomplete
	}
	return b, nil
}
func (o *observer) observeProcesses() {
	scope := "existing_sessions_and_processes"
	names, e := o.list("/proc")
	if e != nil {
		o.gap(scope, "proc_roster_unread")
		return
	}
	for _, name := range names {
		pid, e := strconv.Atoi(name)
		if e != nil || pid < 1 {
			continue
		}
		base := "/proc/" + name
		raw, e := o.procRead(base + "/stat")
		if e != nil {
			o.gap(scope, "process_disappeared_or_stat_unread")
			continue
		}
		p, ppid, start, e := parseProcStat(raw)
		if e != nil || p != pid {
			o.gap(scope, "process_stat_schema_unknown")
			continue
		}
		status, e := o.procRead(base + "/status")
		if e != nil {
			o.gap(scope, "process_status_unread")
			continue
		}
		uid, e := parseUID(status)
		if e != nil {
			o.gap(scope, "process_uid_schema_unknown")
			continue
		}
		physical, _ := o.physical(base + "/exe")
		exe, e := os.Readlink(physical)
		if e != nil {
			if bytes.Contains(status, []byte("Kthread:\t1")) {
				o.out.Processes = append(o.out.Processes, Process{PID: pid, ParentPID: ppid, UID: uid, StartTicks: start, Role: "kernel_thread"})
				continue
			}
			o.gap(scope, "process_executable_unread")
			continue
		}
		role := "unclassified_process"
		bn := filepath.Base(exe)
		switch bn {
		case "sshd":
			role = "sshd"
		case "Runner.Listener", "Runner.Worker":
			role = "local_actions_runner"
		case "systemd":
			role = "systemd_manager"
		}
		cgroup, e := o.procRead(base + "/cgroup")
		if e != nil {
			o.gap(scope, "process_cgroup_unread")
		}
		ns, e := o.namespace(base + "/ns")
		if e != nil {
			o.gap(scope, "process_namespace_unread")
		}
		after, e := o.procRead(base + "/stat")
		ap, app, ast, ae := parseProcStat(after)
		if e != nil || ae != nil || ap != p || app != ppid || ast != start {
			o.gap(scope, "process_changed_during_read")
			continue
		}
		fact := Process{PID: pid, ParentPID: ppid, UID: uid, StartTicks: start, Role: role, ExecutablePathSHA256: sum([]byte(exe)), CgroupSHA256: sum(cgroup), NamespaceSHA256: ns}
		fact.ExecutableSHA256, e = o.hashProcessExecutable(base+"/exe", exe)
		if e != nil {
			o.gap(scope, "process_executable_hash_unread")
		}
		o.out.Processes = append(o.out.Processes, fact)
		if role == "local_actions_runner" {
			o.gap("local_activation_sources", "local_runner_workflow_admission_not_fenced")
		}
	}
	sessions, e := o.observedCommand("sessions")
	if e != nil {
		o.gap(scope, "login_session_listing_unread")
	} else {
		for _, line := range strings.Split(strings.TrimSpace(string(sessions)), "\n") {
			f := strings.Fields(line)
			if len(f) < 2 {
				if line != "" {
					o.gap(scope, "login_session_schema_unknown")
				}
				continue
			}
			uid, e := strconv.ParseUint(f[1], 10, 32)
			if e != nil {
				o.gap(scope, "login_session_schema_unknown")
				continue
			}
			o.entry("login_session", uint32(uid), f[1], f[0], sum(sessions), "existing_session_observed_not_drained")
		}
	}
	o.gap(scope, "existing_sessions_not_drained_or_admission_fenced")
	o.scope(scope).Items = len(o.out.Processes)
	o.gap(scope, "unclassified_processes_require_independent_writer_catalog")
	o.scope(scope).EnumerationComplete = !hasUnknown(o.scope(scope).Unknown, "process_") && !hasUnknown(o.scope(scope).Unknown, "proc_") && !hasUnknown(o.scope(scope).Unknown, "login_session_")
}
func (o *observer) observeSSH() {
	scope := "ssh_configuration_sources"
	visited := map[string]bool{}
	o.readSSHConfig("/etc/ssh/sshd_config", visited, 0)
	// Actual daemon argv is examined only in memory. It is never copied or logged.
	count := 0
	for _, p := range o.out.Processes {
		if p.Role != "sshd" {
			continue
		}
		count++
		raw, e := o.procRead(fmt.Sprintf("/proc/%d/cmdline", p.PID))
		if e != nil {
			o.gap(scope, "sshd_original_argv_unread")
			continue
		}
		args := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		o.argvHashes[p.PID] = sum(raw)
		if len(args) == 0 || strings.Contains(args[0], "sshd:") {
			o.gap(scope, "source_sshd_original_config_from_process_title_unproven")
		}
		o.entry("sshd_actual_argv", p.UID, strconv.Itoa(p.PID), strconv.Itoa(p.PID), sum(raw), "in_memory_only")
		for i, arg := range args {
			if arg == "-f" && i+1 < len(args) {
				o.readSSHConfig(args[i+1], visited, 0)
			} else if strings.HasPrefix(arg, "-f") && len(arg) > 2 {
				o.readSSHConfig(arg[2:], visited, 0)
			} else if arg == "-o" || strings.HasPrefix(arg, "-o") {
				o.gap(scope, "sshd_command_line_override_semantics_unproven")
			}
		}
	}
	if count == 0 {
		o.gap(scope, "sshd_daemon_not_observed")
	}
	o.gap(scope, "sshd_loaded_configuration_snapshot_unproven")
	o.scope(scope).Items = len(visited)
	o.scope(scope).EnumerationComplete = len(visited) > 0 && !hasUnknown(o.scope(scope).Unknown, "source_")
}
func hasUnknown(v []string, prefix string) bool {
	for _, s := range v {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
func sshFields(line string) ([]string, error) {
	var out []string
	var b strings.Builder
	quote := byte(0)
	escape := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if escape {
			b.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				b.WriteByte(c)
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '#' {
			break
		}
		if c == ' ' || c == '\t' {
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		} else {
			b.WriteByte(c)
		}
	}
	if quote != 0 || escape {
		return nil, ErrIncomplete
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out, nil
}
func (o *observer) readSSHConfig(path string, visited map[string]bool, depth int) {
	scope := "ssh_configuration_sources"
	if !strings.HasPrefix(path, "/etc/ssh/") || filepath.Clean(path) != path || depth > 8 || len(visited) >= 512 {
		o.gap(scope, "source_sshd_include_scope_or_budget_unknown")
		return
	}
	if visited[path] {
		o.gap(scope, "source_sshd_include_cycle_or_duplicate")
		return
	}
	visited[path] = true
	raw, e := o.read(path, "sshd_configuration", fileLimit)
	if e != nil {
		o.gap(scope, "source_sshd_configuration_unread")
		return
	}
	for n, line := range strings.Split(string(raw), "\n") {
		f, e := sshFields(line)
		if e != nil {
			o.gap(scope, "source_sshd_syntax_unknown")
			continue
		}
		if len(f) == 0 {
			continue
		}
		key := strings.ToLower(f[0])
		tail := strings.Join(f[1:], " ")
		if key == "match" {
			o.entry("sshd_match_domain", 0, path, fmt.Sprintf("%s:%d", path, n), sum(raw), tail)
			o.gap(scope, "all_match_authentication_domains_not_exhaustively_proven")
		}
		if key == "include" {
			for _, pattern := range f[1:] {
				if !filepath.IsAbs(pattern) {
					pattern = filepath.Join("/etc/ssh", pattern)
				}
				if !strings.HasPrefix(pattern, "/etc/ssh/") || filepath.Clean(pattern) != pattern || strings.ContainsAny(filepath.Dir(pattern), "*?[") {
					o.gap(scope, "source_sshd_include_path_unsupported")
					continue
				}
				names, e := o.list(filepath.Dir(pattern))
				if e != nil {
					o.gap(scope, "source_sshd_include_directory_unread")
					continue
				}
				for _, name := range names {
					matched, e := filepath.Match(filepath.Base(pattern), name)
					if e != nil {
						o.gap(scope, "source_sshd_include_pattern_unknown")
						break
					}
					if matched {
						o.readSSHConfig(filepath.Join(filepath.Dir(pattern), name), visited, depth+1)
					}
				}
			}
		}
		if key == "authorizedkeyscommand" || key == "authorizedprincipalscommand" {
			o.entry("dynamic_ssh_authorization_source", 0, path, fmt.Sprintf("%s:%d", path, n), sum(raw), tail)
			if tail != "none" {
				o.gap("nss_accounts_and_key_sources", "dynamic_key_or_principal_provider_not_exhaustively_proven")
			}
		}
		if key == "trustedusercakeys" || key == "authorizedprincipalsfile" || key == "authorizedkeysfile" {
			o.entry("ssh_public_authorization_source", 0, path, fmt.Sprintf("%s:%d", path, n), sum(raw), tail)
			for _, source := range f[1:] {
				if source == "none" {
					continue
				}
				if strings.Contains(source, "%") {
					o.gap("nss_accounts_and_key_sources", "ssh_key_path_expansion_requires_effective_subject")
					continue
				}
				if filepath.IsAbs(source) && strings.HasPrefix(source, "/etc/ssh/") {
					if _, e = o.read(source, "ssh_public_authorization", fileLimit); e != nil {
						o.gap("nss_accounts_and_key_sources", "ssh_public_authorization_unread")
					}
				} else {
					o.gap("nss_accounts_and_key_sources", "ssh_authorization_path_scope_unknown")
				}
			}
		}
	}
}
func (o *observer) observeActivation() {
	scope := "local_activation_sources"
	for _, root := range []string{"/etc/cron.d", "/etc/cron.hourly", "/etc/cron.daily", "/etc/cron.weekly", "/etc/cron.monthly", "/var/spool/cron", "/var/spool/cron/crontabs", "/etc/systemd/system", "/etc/systemd/user", "/usr/lib/systemd/system", "/usr/lib/systemd/user", "/run/systemd/system", "/run/systemd/transient", "/run/systemd/user"} {
		o.walkActivation(root, 0)
	}
	for _, p := range []string{"/etc/crontab", "/etc/anacrontab", "/etc/profile", "/etc/bash.bashrc", "/etc/zsh/zshrc", "/etc/zsh/zprofile", "/etc/zsh/zshenv", "/etc/ssh/sshrc"} {
		physical, _ := o.physical(p)
		if _, e := os.Lstat(physical); os.IsNotExist(e) {
			o.absent[p] = true
			continue
		}
		raw, e := o.read(p, "local_activation_source", fileLimit)
		if e != nil {
			o.gap(scope, "activation_source_unread")
		} else {
			o.entry("fixed_activation_source", 0, p, p, sum(raw), "body_never_persisted")
		}
	}
	names := map[string]bool{}
	for _, kind := range []string{"units", "unit_files"} {
		raw, e := o.observedCommand(kind)
		if e != nil {
			o.gap(scope, "native_systemd_listing_unread")
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			f := strings.Fields(line)
			if len(f) == 0 {
				continue
			}
			if !unitPattern.MatchString(f[0]) {
				o.gap(scope, "native_systemd_listing_schema_unknown")
				continue
			}
			names[f[0]] = true
		}
		o.entry("native_systemd_listing", 0, kind, kind, sum(raw), "finite_command_eof")
	}
	if len(names) > 512 {
		o.gap(scope, "native_systemd_unit_budget_exceeded")
	} else {
		ordered := make([]string, 0, len(names))
		for n := range names {
			ordered = append(ordered, n)
		}
		sort.Strings(ordered)
		if len(ordered) > 0 {
			raw, e := o.observedCommand("unit", ordered...)
			if e != nil {
				o.gap(scope, "native_systemd_properties_unread")
			} else {
				o.entry("native_systemd_unit_batch", 0, "system", "complete_listed_units", sum(raw), "fixed_properties_no_environment_or_command")
			}
		}

	}
	for _, a := range o.accounts {
		if !filepath.IsAbs(a.home) || filepath.Clean(a.home) != a.home {
			continue
		}
		for _, suffix := range []string{".config/systemd/user", ".local/share/systemd/user"} {
			o.walkActivation(filepath.Join(a.home, suffix), 0)
		}
		o.walkActivation(fmt.Sprintf("/run/user/%d/systemd", a.uid), 0)
	}
	o.gap(scope, "indirect_activation_scripts_and_arbitrary_commands_unproven")
	o.gap(scope, "user_manager_runtime_socket_activation_not_exhaustively_proven")
	o.scope(scope).Items = len(names)
	o.scope(scope).EnumerationComplete = !hasUnknown(o.scope(scope).Unknown, "source_") && !hasUnknown(o.scope(scope).Unknown, "activation_") && !hasUnknown(o.scope(scope).Unknown, "native_")
}
func (o *observer) walkActivation(path string, depth int) {
	scope := "local_activation_sources"
	if depth > 6 || len(o.files) > 4096 {
		o.gap(scope, "source_activation_tree_budget_exceeded")
		return
	}
	physical, _ := o.physical(path)
	info, e := os.Lstat(physical)
	if os.IsNotExist(e) {
		o.absent[path] = true
		return
	}
	if e != nil {
		o.gap(scope, "source_activation_tree_unread")
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, e := os.Readlink(physical)
		if e != nil {
			o.gap(scope, "source_activation_symlink_unread")
			return
		}
		o.links[path] = link
		o.entry("activation_symlink", 0, path, path, "", link)
		o.gap(scope, "source_activation_symlink_target_not_followed")
		return
	}
	if !info.IsDir() {
		raw, e := o.read(path, "local_activation_source", fileLimit)
		if e != nil {
			o.gap(scope, "activation_source_unread")
		} else {
			o.entry("activation_file", 0, path, path, sum(raw), "body_never_persisted")
		}
		return
	}
	names, e := o.list(path)
	if e != nil {
		o.gap(scope, "source_activation_directory_unread")
		return
	}
	for _, name := range names {
		o.walkActivation(filepath.Join(path, name), depth+1)
	}
}

// Only a numeric PID supplied by the real /proc enumeration reaches this
// deliberate kernel exe-link exception. No approved DTO can name a link/path.
func (o *observer) hashProcessExecutable(path, link string) (string, error) {
	p, e := o.physical(path)
	if e != nil {
		return "", e
	}
	f, e := os.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if e != nil {
		return "", ErrIncomplete
	}
	defer o.closeObserved(f)
	before, e := f.Stat()
	if e != nil || !before.Mode().IsRegular() || before.Size() < 1 || before.Size() > 128<<20 {
		return "", ErrIncomplete
	}
	st, ok := before.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ErrIncomplete
	}
	key := fmt.Sprintf("%d:%d:%d:%d:%s", st.Dev, st.Ino, before.Size(), before.ModTime().UnixNano(), jsonHash(changeTime(st)))
	if hash, ok := o.binaries[key]; ok {
		after, x := f.Stat()
		current, y := os.Readlink(p)
		if x != nil || y != nil || !same(before, after) || current != link {
			return "", ErrIncomplete
		}
		return hash, nil
	}
	if o.binaryBytes+before.Size() > 512<<20 {
		return "", ErrIncomplete
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, (128<<20)+1))
	after, x := f.Stat()
	current, y := os.Readlink(p)
	if e != nil || x != nil || y != nil || !same(before, after) || current != link || n != before.Size() || o.ctx.Err() != nil {
		return "", ErrIncomplete
	}
	o.binaryBytes += n
	hash := hex.EncodeToString(h.Sum(nil))
	o.binaries[key] = hash
	return hash, nil
}
func (o *observer) observedCommand(kind string, args ...string) ([]byte, error) {
	raw, e := o.command(kind, args...)
	if e == nil {
		o.commandHashes[strings.Join(append([]string{kind}, args...), "\x00")] = sum(raw)
	}
	return raw, e
}
func (o *observer) recheckProcesses() {
	for _, p := range o.out.Processes {
		base := fmt.Sprintf("/proc/%d", p.PID)
		raw, e := o.procRead(base + "/stat")
		pid, parent, start, x := parseProcStat(raw)
		status, y := o.procRead(base + "/status")
		uid, z := parseUID(status)
		if e != nil || x != nil || y != nil || z != nil || pid != p.PID || parent != p.ParentPID || start != p.StartTicks || uid != p.UID {
			o.out.Unknown = appendUnique(o.out.Unknown, "process_end_recheck_changed_or_unread")
			continue
		}
		if p.Role == "kernel_thread" {
			continue
		}
		physical, _ := o.physical(base + "/exe")
		exe, e := os.Readlink(physical)
		hash, x := o.hashProcessExecutable(base+"/exe", exe)
		ns, y := o.namespace(base + "/ns")
		cg, z := o.procRead(base + "/cgroup")
		if e != nil || x != nil || y != nil || z != nil || sum([]byte(exe)) != p.ExecutablePathSHA256 || hash != p.ExecutableSHA256 || ns != p.NamespaceSHA256 || sum(cg) != p.CgroupSHA256 {
			o.out.Unknown = appendUnique(o.out.Unknown, "process_end_recheck_changed_or_unread")
		}
	}
}

// All pathname components are opened on actual held dirfds. Intermediate
// symlinks cannot widen the fixed source set, including mutable user homes.
func openObservedFile(path string, directory bool) (file *os.File, check func() error, closeParents func() error, result error) {
	type edge struct {
		parent int
		name   string
		child  int
		stat   unix.Stat_t
	}
	var edges []edge
	var parents []int
	cleanup := func() (result error) {
		for i := len(parents) - 1; i >= 0; i-- {
			if e := unix.Close(parents[i]); result == nil && e != nil {
				result = e
			}
		}
		return result
	}
	root, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, nil, nil, e
	}
	parents = append(parents, root)
	parent := root
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, part := range parts[:len(parts)-1] {
		if part == "" || part == "." || part == ".." {
			_ = cleanup()
			return nil, nil, nil, ErrBinding
		}
		child, e := unix.Openat(parent, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if e != nil {
			_ = cleanup()
			return nil, nil, nil, e
		}
		parents = append(parents, child)
		var st unix.Stat_t
		if unix.Fstat(child, &st) != nil {
			_ = cleanup()
			return nil, nil, nil, ErrIncomplete
		}
		edges = append(edges, edge{parent, part, child, st})
		parent = child
	}
	flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if directory {
		flags |= unix.O_DIRECTORY
	}
	fd, e := unix.Openat(parent, parts[len(parts)-1], flags, 0)
	if e != nil {
		_ = cleanup()
		return nil, nil, nil, e
	}
	file = os.NewFile(uintptr(fd), "fixed-host-observation")
	check = func() error {
		for _, edge := range edges {
			var held, named unix.Stat_t
			if unix.Fstat(edge.child, &held) != nil || unix.Fstatat(edge.parent, edge.name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || held.Dev != edge.stat.Dev || held.Ino != edge.stat.Ino || held.Mode != edge.stat.Mode || held.Uid != edge.stat.Uid || held.Gid != edge.stat.Gid || named.Dev != held.Dev || named.Ino != held.Ino || named.Mode != held.Mode || named.Uid != held.Uid || named.Gid != held.Gid {
				return ErrIncomplete
			}
		}
		info, e := file.Stat()
		var named unix.Stat_t
		if e != nil || unix.Fstatat(parent, parts[len(parts)-1], &named, unix.AT_SYMLINK_NOFOLLOW) != nil {
			return ErrIncomplete
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || uint64(named.Dev) != uint64(st.Dev) || uint64(named.Ino) != uint64(st.Ino) || uint32(named.Mode) != uint32(st.Mode) || named.Uid != st.Uid || named.Gid != st.Gid {
			return ErrIncomplete
		}
		return nil
	}
	return file, check, cleanup, nil
}

func protectedExecutableDirectory(path string) (os.FileInfo, error) {
	i, e := os.Lstat(path)
	if e != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 || i.Mode().Perm()&022 != 0 {
		return nil, ErrIncomplete
	}
	uid, ok := uidOf(i)
	if !ok || uid != 0 {
		return nil, ErrIncomplete
	}
	return i, nil
}
func (o *observer) observeContainers() {
	scope := "local_activation_sources"
	raw, e := o.observedCommand("docker_list")
	if e != nil {
		o.gap(scope, "native_docker_roster_unread")
		o.scope(scope).EnumerationComplete = false
		return
	}
	ids := strings.Fields(string(raw))
	if len(ids) > 512 {
		o.gap(scope, "native_docker_roster_budget_exceeded")
		o.scope(scope).EnumerationComplete = false
		return
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !hashPattern.MatchString(id) || seen[id] {
			o.gap(scope, "native_docker_roster_schema_unknown")
			o.scope(scope).EnumerationComplete = false
			return
		}
		seen[id] = true
	}
	sort.Strings(ids)
	o.out.Containers = []Container{}
	if len(ids) > 0 {
		raw, e = o.observedCommand("docker_inspect", ids...)
		if e != nil {
			o.gap(scope, "native_docker_selected_inspect_unread")
			o.scope(scope).EnumerationComplete = false
			return
		}
		lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
		if len(lines) != len(ids) {
			o.gap(scope, "native_docker_selected_inspect_schema_unknown")
			o.scope(scope).EnumerationComplete = false
			return
		}
		for _, line := range lines {
			var v struct {
				ID         string            `json:"id"`
				Image      string            `json:"image"`
				PID        int               `json:"pid"`
				Labels     map[string]string `json:"labels"`
				Entrypoint []string          `json:"entrypoint"`
				Command    []string          `json:"command"`
				Restart    json.RawMessage   `json:"restart"`
				Mounts     json.RawMessage   `json:"mounts"`
			}
			d := json.NewDecoder(strings.NewReader(line))
			d.DisallowUnknownFields()
			if d.Decode(&v) != nil || d.Decode(new(any)) != io.EOF || !seen[v.ID] || !hashPattern.MatchString(strings.TrimPrefix(v.Image, "sha256:")) || !strings.HasPrefix(v.Image, "sha256:") || v.PID < 0 {
				o.gap(scope, "native_docker_selected_inspect_schema_unknown")
				o.scope(scope).EnumerationComplete = false
				return
			}
			delete(seen, v.ID)
			fact := Container{ID: v.ID, ImageID: v.Image, PID: v.PID, LabelsSHA256: jsonHash(v.Labels), EntrypointSHA256: jsonHash(v.Entrypoint), CommandSHA256: jsonHash(v.Command), RestartSHA256: jsonHash(v.Restart), MountsSHA256: jsonHash(v.Mounts)}
			o.out.Containers = append(o.out.Containers, fact)
			o.entry("actual_container_writer_candidate", 0, v.ID, v.ID, jsonHash(fact), "candidate_requires_independent_QS_descriptor_approval")
		}
	}
	if len(seen) != 0 {
		o.gap(scope, "native_docker_selected_inspect_schema_unknown")
		o.scope(scope).EnumerationComplete = false
	}
	sort.Slice(o.out.Containers, func(i, j int) bool { return o.out.Containers[i].ID < o.out.Containers[j].ID })
	o.gap(scope, "docker_socket_and_other_container_writer_admission_not_fenced")
}

func (o *observer) closeObserved(f *os.File) {
	if f.Close() != nil {
		o.out.Unknown = appendUnique(o.out.Unknown, "observation_handle_close_failed")
	}
}
func (o *observer) closeParents(close func() error) {
	if close() != nil {
		o.out.Unknown = appendUnique(o.out.Unknown, "observation_handle_close_failed")
	}
}
