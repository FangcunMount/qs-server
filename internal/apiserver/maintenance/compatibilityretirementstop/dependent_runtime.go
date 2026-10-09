package compatibilityretirementstop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"time"

	redisobs "github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

// Only fixed metadata leaves the actual original owner. ReadySHA256 represents
// a real read, never broker/consumer or business-flow acceptance.
type DependentRuntimeInstance struct {
	ContainerID   string `json:"container_id"`
	ImageID       string `json:"image_id"`
	ProgramSHA256 string `json:"program_sha256"`
	StateSHA256   string `json:"state_sha256"`
	ReadySHA256   string `json:"ready_sha256"`
}
type DependentRuntimeSnapshot struct {
	Instances []DependentRuntimeInstance `json:"instances"`
}
type DependentRuntimeObservation struct {
	self     *DependentRuntimeObservation
	lease    *Lease
	snapshot DependentRuntimeSnapshot
	seal     string
}

func (*DependentRuntimeObservation) MarshalJSON() ([]byte, error) { return nil, ErrBinding }
func (o *DependentRuntimeObservation) Snapshot() (DependentRuntimeSnapshot, error) {
	if o == nil || o.self != o || o.lease == nil || o.seal != runtimeDigest(o.snapshot) {
		return DependentRuntimeSnapshot{}, ErrBinding
	}
	return DependentRuntimeSnapshot{Instances: append([]DependentRuntimeInstance(nil), o.snapshot.Instances...)}, nil
}
func runtimeDigest(v any) string {
	raw, e := json.Marshal(v)
	if e != nil {
		return ""
	}
	return digest(raw)
}

// ControlledResumeDependents preserves the original forward lease and every
// actual restored timestamp. It does not release the session. The fixed host
// must have compared its native non-target baseline and retained its complete
// external writer fence before calling this service-only primitive.
func (l *Lease) ControlledResumeDependents(ctx context.Context) error {
	if l == nil || l.self != l {
		return ErrBinding
	}
	l.mu.Lock()
	if l.closed || l.failed || l.controlledIssued || l.approval == nil || !runtimeReplyFits(l.baseline) {
		l.mu.Unlock()
		return ErrBinding
	}
	l.controlledIssued = true
	l.mu.Unlock()
	if e := l.restoreScope(ctx, true, false); e != nil {
		return e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.failed || ctx == nil || ctx.Err() != nil {
		return ErrState
	}
	l.controlledResumed = true
	return nil
}

func runtimeReplyFits(baseline []Container) bool {
	var instances []DependentRuntimeInstance
	for _, v := range baseline {
		if v.Component != "qs-apiserver" && v.Running {
			instances = append(instances, DependentRuntimeInstance{strings.Repeat("a", 64), "sha256:" + strings.Repeat("a", 64), strings.Repeat("a", 64), strings.Repeat("a", 64), strings.Repeat("a", 64)})
		}
	}
	// Keep the original 4095-byte session limit. Allow conservative room for the
	// existing full batch/source/window envelope instead of expanding its bound.
	b, e := json.Marshal(DependentRuntimeSnapshot{instances})
	return e == nil && len(instances) > 0 && len(b)+1200 < 4095
}

func (l *Lease) ObserveRunningDependents(ctx context.Context) (*DependentRuntimeObservation, error) {
	return l.observeRunningDependents(ctx, "")
}

// WithInlineAPI binds A's sole observed API to the actual original native inline
// owner. The fixed host is responsible for producing that ID; it is never read
// from the lifecycle request. D uses ObserveRunningDependents without an API.
func (l *Lease) ObserveRunningDependentsWithInlineAPI(ctx context.Context, nativeInlineAPIID string) (*DependentRuntimeObservation, error) {
	if !hash64.MatchString(nativeInlineAPIID) {
		return nil, ErrBinding
	}
	return l.observeRunningDependents(ctx, nativeInlineAPIID)
}
func (l *Lease) observeRunningDependents(ctx context.Context, nativeInlineAPIID string) (*DependentRuntimeObservation, error) {
	if l == nil || l.self != l {
		return nil, ErrBinding
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.failed || !l.controlledResumed || ctx == nil || ctx.Err() != nil || checkWindow(ctx, l.approval, l.window) != nil {
		return nil, ErrBinding
	}
	q, c, e := l.window.ForwardContext(ctx)
	if e != nil {
		return nil, ErrBinding
	}
	defer c()
	before, e := l.approval.catalog(q)
	if e != nil {
		return nil, e
	}
	if l.approval.descriptor.HostRole == "server-a" {
		if !hash64.MatchString(nativeInlineAPIID) {
			return nil, ErrBinding
		}
		for _, v := range before {
			if v.Component == "qs-apiserver" && v.ID != nativeInlineAPIID {
				return nil, ErrState
			}
		}
	} else if nativeInlineAPIID != "" {
		return nil, ErrBinding
	}
	selected, e := runningDependents(l.baseline, before, l.restored, l.approval.descriptor.HostRole)
	if e != nil {
		return nil, e
	}
	snapshot := DependentRuntimeSnapshot{Instances: []DependentRuntimeInstance{}}
	for _, v := range selected {
		if e = l.noPendingStop(v); e != nil {
			return nil, e
		}
		if !v.Running {
			continue
		} // Originally stopped instances remain stopped.
		program, ready, e := l.approval.readRuntime(q, v)
		if e != nil {
			return nil, e
		}
		snapshot.Instances = append(snapshot.Instances, DependentRuntimeInstance{v.ID, v.Image, program, runtimeDigest(v), ready})
	}
	after, e := l.approval.catalog(q)
	if e != nil || !reflect.DeepEqual(before, after) || q.Err() != nil {
		return nil, ErrState
	}
	if !runtimeReplyFits(l.baseline) {
		return nil, ErrBinding
	}
	o := &DependentRuntimeObservation{lease: l, snapshot: snapshot, seal: runtimeDigest(snapshot)}
	o.self = o
	l.runtimeObservation = o
	return o, nil
}

func runningDependents(baseline []Container, actual []actualContainer, restored map[string]string, role string) ([]actualContainer, error) {
	if role != "server-a" && role != "server-d" {
		return nil, ErrBinding
	}
	originals := []Container{}
	observed := []actualContainer{}
	apis := 0
	for _, v := range baseline {
		if v.Component != "qs-apiserver" {
			originals = append(originals, v)
		}
	}
	for _, v := range actual {
		if v.Component == "qs-apiserver" {
			apis++
			if role != "server-a" || !validDependentScopeAPI(v.Container) {
				return nil, ErrState
			}
		} else {
			observed = append(observed, v)
		}
	}
	if len(observed) != len(originals) || role == "server-a" && apis != 1 || role == "server-d" && apis != 0 {
		return nil, ErrState
	}
	for i, v := range observed {
		old := originals[i]
		if !sameIdentity(v, old) || v.Paused || v.Restarting || v.Dead || v.OOMKilled || v.Running != old.Running || v.Running && (v.PID <= 0 || restored[v.ID] == "" || v.StartedAt != restored[v.ID]) || !v.Running && (v.PID != 0 || v.StartedAt != old.StartedAt) {
			return nil, ErrState
		}
	}
	return observed, nil
}

const runtimeInspectionFormat = `{"id":{{json .Id}},"image":{{json .Image}},"path":{{json .Path}},"mount_destinations":[{{range $i,$m := .Mounts}}{{if $i}},{{end}}{{json $m.Destination}}{{end}}]}`

func (a *Approval) readRuntime(ctx context.Context, v actualContainer) (string, string, error) {
	program, config, component, port, endpoint := "", "", "", 0, ""
	switch v.Component {
	case "qs-worker":
		program, config, component, port, endpoint = "/app/qs-worker", "/app/configs/worker.prod.yaml", "worker", 9092, "/readyz"
	case "qs-collection-server":
		program, config, component, port, endpoint = "/app/collection-server", "/app/configs/collection-server.prod.yaml", "collection-server", 8080, "/serve-readyz"
	default:
		return "", "", ErrBinding
	}
	raw, e := a.docker(ctx, "inspect", "--format", runtimeInspectionFormat, v.ID)
	var metadata struct {
		ID     string   `json:"id"`
		Image  string   `json:"image"`
		Path   string   `json:"path"`
		Mounts []string `json:"mount_destinations"`
	}
	if e != nil || exactJSON(bytes.TrimSpace(raw), &metadata) != nil {
		return "", "", ErrState
	}
	if metadata.ID != v.ID || metadata.Image != v.Image || metadata.Path != program || !runtimeMountsSafe(metadata.Mounts, program) {
		return "", "", ErrState
	}
	raw, e = a.docker(ctx, "image", "inspect", "--format", `{"image_id":{{json .Id}},"entrypoint":{{json .Config.Entrypoint}}}`, v.Image)
	var imageMetadata struct {
		ID         string   `json:"image_id"`
		Entrypoint []string `json:"entrypoint"`
	}
	if e != nil || exactJSON(bytes.TrimSpace(raw), &imageMetadata) != nil || imageMetadata.ID != v.Image || !reflect.DeepEqual(imageMetadata.Entrypoint, []string{program}) {
		return "", "", ErrState
	}
	raw, e = a.docker(ctx, "diff", v.ID)
	if e != nil || runtimeDiffSafe(string(raw), program) != nil {
		return "", "", ErrState
	}
	raw, e = a.docker(ctx, "exec", v.ID, "sha256sum", "/proc/1/exe", program)
	if e != nil {
		return "", "", e
	}
	hash, e := runtimeProgramHash(string(raw), program)
	if e != nil {
		return "", "", e
	}
	raw, e = a.docker(ctx, "exec", v.ID, program, "--version=true", "--config="+config)
	if e != nil || len(raw) > 64<<10 || runtimeVersionValid(string(raw), a.descriptor.SourceSHA, runtime.GOARCH) != nil {
		return "", "", ErrState
	}
	raw, e = a.docker(ctx, "exec", v.ID, "wget", "-qO-", "-T", "5", fmt.Sprintf("http://127.0.0.1:%d%s", port, endpoint))
	if e != nil || len(raw) > 64<<10 || runtimeReadinessValid(raw, component, v.StartedAt) != nil {
		return "", "", ErrState
	}
	return hash, digest(raw), nil
}

func runtimeMountsSafe(mounts []string, program string) bool {
	seen := map[string]bool{}
	for _, m := range mounts {
		if !strings.HasPrefix(m, "/") || strings.ContainsAny(m, "\x00\r\n") || seen[m] {
			return false
		}
		seen[m] = true
		p := path.Clean(m)
		if p == program || strings.HasPrefix(program, strings.TrimSuffix(p, "/")+"/") || p == "/proc" || strings.HasPrefix(p, "/proc/") {
			return false
		}
	}
	return true
}
func runtimeDiffSafe(raw, program string) error {
	for _, line := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		if line == "" && raw == "" {
			continue
		}
		if len(line) < 4 || !strings.ContainsRune("ACD", rune(line[0])) || line[1] != ' ' || line[2] != '/' || strings.ContainsAny(line, "\x00\r") {
			return ErrState
		}
		p := strings.TrimSuffix(line[2:], "/")
		if p == "" {
			p = "/"
		}
		if p == program || (line[0] == 'A' || line[0] == 'D') && strings.HasPrefix(program, strings.TrimSuffix(p, "/")+"/") {
			return ErrState
		}
	}
	return nil
}
func runtimeProgramHash(raw, program string) (string, error) {
	lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
	if len(lines) != 2 || len(lines[0]) != 64+2+len("/proc/1/exe") || len(lines[1]) != 64+2+len(program) || !hash64.MatchString(lines[0][:64]) || lines[0][64:] != "  /proc/1/exe" || lines[1][64:] != "  "+program || lines[0][:64] != lines[1][:64] {
		return "", ErrState
	}
	return lines[0][:64], nil
}

var versionLine = regexp.MustCompile(`^[ \t]*([a-zA-Z]+):[ \t]*([^\x00-\x1f\x7f]*)$`)

func runtimeVersionValid(raw, source, arch string) error {
	keys := []string{"gitVersion", "gitCommit", "gitTreeState", "buildDate", "goVersion", "compiler", "platform"}
	lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
	blocks, commits := 0, 0
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "gitCommit:") {
			commits++
		}
		if !strings.HasPrefix(strings.TrimSpace(line), "gitVersion:") {
			continue
		}
		if i+len(keys) > len(lines) {
			return ErrState
		}
		values := map[string]string{}
		for j, key := range keys {
			m := versionLine.FindStringSubmatch(lines[i+j])
			if len(m) != 3 || m[1] != key {
				return ErrState
			}
			values[key] = strings.TrimSpace(m[2])
		}
		if values["gitCommit"] != source || !sha40.MatchString(source) || values["gitTreeState"] != "clean" || values["platform"] != "linux/"+arch {
			return ErrState
		}
		blocks++
	}
	if blocks != 1 || commits != 1 {
		return ErrState
	}
	return nil
}
func runtimeReadinessValid(raw []byte, component, started string) error {
	var r struct {
		Status    string                   `json:"status"`
		Component string                   `json:"component"`
		Redis     redisobs.RuntimeSnapshot `json:"redis"`
	}
	if component == "collection-server" {
		var envelope struct {
			Code    *int   `json:"code"`
			Message string `json:"message"`
			Data    struct {
				Status          string                   `json:"status"`
				Service         string                   `json:"service"`
				Version         string                   `json:"version"`
				ServeReady      bool                     `json:"serve_ready"`
				DependencyReady bool                     `json:"dependency_ready"`
				Redis           redisobs.RuntimeSnapshot `json:"redis"`
				Synchronized    bool                     `json:"resilience_control_synchronized"`
			} `json:"data"`
		}
		if exactJSON(raw, &envelope) != nil || envelope.Code == nil || *envelope.Code != 0 || envelope.Message != "success" || !envelope.Data.ServeReady || !envelope.Data.DependencyReady || !envelope.Data.Synchronized {
			return ErrState
		}
		r.Status, r.Component, r.Redis = envelope.Data.Status, envelope.Data.Service, envelope.Data.Redis
	} else if component != "worker" || exactJSON(raw, &r) != nil {
		return ErrState
	}
	if r.Status != "ready" || r.Component != component || r.Redis.Component != component || len(r.Redis.Families) == 0 || !r.Redis.Summary.Ready || r.Redis.Summary != redisobs.SummarizeFamilies(r.Redis.Families) {
		return ErrState
	}
	start, e := time.Parse(time.RFC3339Nano, started)
	if e != nil || r.Redis.GeneratedAt.Before(start) || r.Redis.GeneratedAt.After(time.Now().Add(5*time.Second)) {
		return ErrState
	}
	for _, family := range r.Redis.Families {
		if family.Component != component || !family.Configured {
			return ErrState
		}
	}
	return nil
}
