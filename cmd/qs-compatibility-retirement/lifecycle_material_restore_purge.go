package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type lifecyclePurgeVolume struct {
	Name, Driver, Mountpoint, CreatedAt, Scope string
	Labels, Options                            map[string]string
	inode                                      os.FileInfo
}
type lifecycleEnginePurge struct {
	engine                    *lifecycleOwnedEngine
	binding                   lifecycleMaterialBinding
	identity                  lifecyclePurgeEngineIdentityValue
	volumes                   []lifecyclePurgeVolume
	started, removed, unknown bool
}

func lifecycleEngineWiresTerminal(e *lifecycleOwnedEngine) bool {
	if e == nil {
		return false
	}
	e.wireMu.Lock()
	defer e.wireMu.Unlock()
	if !e.wireClosed {
		return false
	}
	for _, w := range e.wires {
		if w == nil || w.done == nil {
			return false
		}
		select {
		case <-w.done:
		default:
			return false
		}
		w.mu.Lock()
		closed := w.closed
		w.mu.Unlock()
		if !closed {
			return false
		}
	}
	return true
}

type lifecyclePurgeEngineIdentityValue struct {
	ID, Name, ImageID, Kind, Tool, Archive, Namespace, Owner, Docker string
	Volumes                                                          []string
	Labels, ContainerLabels                                          map[string]string
}

func lifecyclePurgeEngineIdentity(e *lifecycleOwnedEngine) lifecyclePurgeEngineIdentityValue {
	// Do not copy the mutex, native wires, contexts or connections.
	v := lifecyclePurgeEngineIdentityValue{ID: e.ID, Name: e.Name, ImageID: e.ImageID, Kind: e.Kind, Tool: e.Tool, Archive: e.Archive, Namespace: e.Namespace, Owner: e.Owner, Docker: e.Docker, Volumes: append([]string(nil), e.Volumes...), Labels: map[string]string{}, ContainerLabels: map[string]string{}}
	for k, x := range e.Labels {
		v.Labels[k] = x
	}
	for k, x := range e.ContainerLabels {
		v.ContainerLabels[k] = x
	}
	return v
}
func readLifecyclePurgeVolume(ctx context.Context, e *lifecycleOwnedEngine, name string) (lifecyclePurgeVolume, error) {
	var v lifecyclePurgeVolume
	raw, err := lifecycleDocker(ctx, e.Docker, "volume", "inspect", name)
	var values []lifecyclePurgeVolume
	if err != nil || rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &values) != nil || len(values) != 1 {
		return v, lifecycleError("lifecycle_restore_volume_identity_unknown")
	}
	v = values[0]
	if v.Name != name || v.Driver != "local" || v.Scope != "local" || v.CreatedAt == "" || len(v.Options) != 0 || !reflect.DeepEqual(v.Labels, e.Labels) || !filepath.IsAbs(v.Mountpoint) || filepath.Clean(v.Mountpoint) != v.Mountpoint {
		return lifecyclePurgeVolume{}, lifecycleError("lifecycle_restore_volume_identity_unknown")
	}
	inode, err := os.Lstat(v.Mountpoint)
	if err != nil || !inode.IsDir() || inode.Mode()&os.ModeSymlink != 0 {
		return lifecyclePurgeVolume{}, lifecycleError("lifecycle_restore_volume_identity_unknown")
	}
	v.inode = inode
	return v, nil
}

// This only registers the actual same-process owned engine, after restore wire
// termination and direct inspections. It does not adopt an intent JSON, grant
// acceptance, or build an engine from an expected name/CID. Native execution is
// not available without the fixed-root socket and the still-required host fence.
func registerLifecycleEnginePurge(ctx context.Context, b lifecycleMaterialBinding, e *lifecycleOwnedEngine) (*lifecycleEnginePurge, error) {
	if e == nil {
		return nil, lifecycleError("lifecycle_restore_owned_material_binding_rejected")
	}
	owner, decodeErr := hex.DecodeString(e.Owner)
	volumeCount := 1
	if e.Kind == "mongodb" {
		volumeCount = 2
	}
	if decodeErr != nil || len(owner) != 16 || e.Kind != "mysql" && e.Kind != "mongodb" || len(e.Volumes) != volumeCount || e.Name != "qs-retirement-restore-"+e.Owner {
		return nil, lifecycleError("lifecycle_restore_owned_material_binding_rejected")
	}
	expectedLabels := map[string]string{"codex.task": "qs-compatibility-retirement", "codex.owner": e.Owner, "qs.retirement.operation": b.operation, "qs.retirement.run": b.actualRun}
	if !reflect.DeepEqual(e.Labels, expectedLabels) {
		return nil, lifecycleError("lifecycle_restore_owned_material_binding_rejected")
	}
	if !b.valid() || e == nil || !hashRE.MatchString(e.ID) || !lifecycleEngineWiresTerminal(e) || e.Labels["qs.retirement.operation"] != b.operation || e.Labels["qs.retirement.run"] != b.actualRun || e.Labels["codex.task"] != "qs-compatibility-retirement" || e.Labels["codex.owner"] != e.Owner || len(e.Owner) != 32 || e.check(ctx, true, true) != nil {
		return nil, lifecycleError("lifecycle_restore_owned_material_binding_rejected")
	}
	p := &lifecycleEnginePurge{engine: e, binding: b, identity: lifecyclePurgeEngineIdentity(e)}
	for _, name := range e.Volumes {
		v, err := readLifecyclePurgeVolume(ctx, e, name)
		if err != nil {
			return nil, err
		}
		p.volumes = append(p.volumes, v)
	}
	return p, nil
}
func lifecycleSameVolume(a, b lifecyclePurgeVolume) bool {
	x, y := a.inode, b.inode
	a.inode, b.inode = nil, nil
	ax, ao := infoStat(x)
	bx, bo := infoStat(y)
	return reflect.DeepEqual(a, b) && ao && bo && ax.Dev == bx.Dev && ax.Ino == bx.Ino && ax.Uid == bx.Uid && ax.Gid == bx.Gid && x.Mode() == y.Mode()
}
func (p *lifecycleEnginePurge) check(ctx context.Context) error {
	if p == nil || p.engine == nil || p.unknown || p.started || !p.binding.valid() || !reflect.DeepEqual(p.identity, lifecyclePurgeEngineIdentity(p.engine)) || !lifecycleEngineWiresTerminal(p.engine) || len(p.volumes) != len(p.engine.Volumes) {
		return lifecycleError("lifecycle_restore_owned_material_binding_rejected")
	}
	if err := p.engine.check(ctx, true, true); err != nil {
		return err
	}
	for _, expected := range p.volumes {
		actual, err := readLifecyclePurgeVolume(ctx, p.engine, expected.Name)
		if err != nil || !lifecycleSameVolume(expected, actual) {
			return lifecycleError("lifecycle_restore_volume_identity_changed")
		}
	}
	return nil
}
func (p *lifecycleEnginePurge) purge(ctx context.Context) error {
	if err := p.check(ctx); err != nil {
		return err
	}
	p.started = true
	// Native daemon calls are exact CID/name, never force, recursive filesystem
	// deletion, image pruning or ownership inferred from a naming prefix.
	if _, err := lifecycleDocker(ctx, p.engine.Docker, "stop", "--time", "10", p.identity.ID); err != nil {
		p.unknown = true
		return err
	}
	if err := p.engine.check(ctx, false, true); err != nil {
		p.unknown = true
		return err
	}
	for _, expected := range p.volumes {
		actual, err := readLifecyclePurgeVolume(ctx, p.engine, expected.Name)
		if err != nil || !lifecycleSameVolume(expected, actual) {
			p.unknown = true
			return lifecycleError("lifecycle_restore_volume_identity_changed")
		}
	}
	if _, err := lifecycleDocker(ctx, p.engine.Docker, "rm", p.identity.ID); err != nil {
		p.unknown = true
		return err
	}
	raw, err := lifecycleDocker(ctx, p.engine.Docker, "ps", "--all", "--no-trunc", "--filter", "id="+p.identity.ID, "--format", "{{.ID}}")
	if err != nil || strings.TrimSpace(string(raw)) != "" {
		p.unknown = true
		return lifecycleError("lifecycle_restore_remove_result_unknown")
	}
	for _, expected := range p.volumes {
		actual, err := readLifecyclePurgeVolume(ctx, p.engine, expected.Name)
		if err != nil || !lifecycleSameVolume(expected, actual) {
			p.unknown = true
			return lifecycleError("lifecycle_restore_volume_identity_changed")
		}
		if _, err = lifecycleDocker(ctx, p.engine.Docker, "volume", "rm", expected.Name); err != nil {
			p.unknown = true
			return err
		}
	}
	p.removed = true
	if err = p.verifyZero(ctx); err != nil {
		p.unknown = true
		return err
	}
	return nil
}
func (p *lifecycleEnginePurge) verifyZero(ctx context.Context) error {
	if p == nil || p.unknown || !p.removed || !lifecycleEngineWiresTerminal(p.engine) || !reflect.DeepEqual(p.identity, lifecyclePurgeEngineIdentity(p.engine)) {
		return lifecycleError("lifecycle_restore_material_zero_unproven")
	}
	queries := [][]string{{"ps", "--all", "--no-trunc", "--filter", "id=" + p.identity.ID, "--format", "{{.ID}}"}}
	for _, v := range p.volumes {
		queries = append(queries, []string{"volume", "ls", "--filter", "name=^" + v.Name + "$", "--format", "{{.Name}}"})
	}
	for _, kind := range []string{"container", "volume"} {
		args := []string{"ps", "--all", "--no-trunc"}
		format := "{{.ID}}"
		if kind == "volume" {
			args = []string{"volume", "ls"}
			format = "{{.Name}}"
		}
		args = append(args, "--filter", "label=codex.owner="+p.identity.Owner, "--filter", "label=qs.retirement.operation="+p.binding.operation, "--filter", "label=qs.retirement.run="+p.binding.actualRun, "--format", format)
		queries = append(queries, args)
	}
	for _, args := range queries {
		raw, err := lifecycleDocker(ctx, p.engine.Docker, args...)
		if err != nil || strings.TrimSpace(string(raw)) != "" {
			return lifecycleError("lifecycle_restore_material_remaining_or_unknown")
		}
	}
	for _, v := range p.volumes {
		if _, err := os.Lstat(v.Mountpoint); !os.IsNotExist(err) {
			return lifecycleError("lifecycle_restore_material_remaining_or_unknown")
		}
	}
	return nil
}

func lifecycleMaterialNativeSet(raw []byte, expected []string) bool {
	actual := strings.Fields(string(raw))
	if len(actual) != len(expected) {
		return false
	}
	seen := map[string]bool{}
	allowed := map[string]bool{}
	for _, s := range expected {
		if s == "" || allowed[s] {
			return false
		}
		allowed[s] = true
	}
	for _, s := range actual {
		if !allowed[s] || seen[s] {
			return false
		}
		seen[s] = true
	}
	return true
}

// All actual resources carrying this batch binding must be exactly registered.
// An unexpected resource blocks cleanup; it is never adopted or removed. Owner
// scans separately detect an unexpected conflicting operation/run label.
func checkLifecycleRestoreMaterialSet(ctx context.Context, engine []*lifecycleEnginePurge, zero bool) error {
	if len(engine) == 0 {
		return nil
	} // Filesystem-only kernel tests grant no host capability.
	if engine[0] == nil || engine[0].engine == nil {
		return lifecycleError("lifecycle_restore_material_scope_unknown")
	}
	b := engine[0].binding
	docker := engine[0].identity.Docker
	ids, volumes := []string{}, []string{}
	for _, e := range engine {
		if e == nil || e.engine == nil || e.binding != b || e.identity.Docker != docker {
			return lifecycleError("lifecycle_restore_material_scope_unknown")
		}
		if !zero {
			ids = append(ids, e.identity.ID)
			for _, v := range e.volumes {
				volumes = append(volumes, v.Name)
			}
		}
		for _, kind := range []string{"container", "volume"} {
			args := []string{"ps", "--all", "--no-trunc"}
			format := "{{.ID}}"
			expected := []string{}
			if !zero {
				expected = []string{e.identity.ID}
			}
			if kind == "volume" {
				args = []string{"volume", "ls"}
				format = "{{.Name}}"
				expected = nil
				if !zero {
					for _, v := range e.volumes {
						expected = append(expected, v.Name)
					}
				}
			}
			raw, err := lifecycleDocker(ctx, docker, append(args, "--filter", "label=codex.owner="+e.identity.Owner, "--format", format)...)
			if err != nil || !lifecycleMaterialNativeSet(raw, expected) {
				return lifecycleError("lifecycle_restore_material_remaining_or_foreign")
			}
		}
	}
	for _, kind := range []string{"container", "volume"} {
		args := []string{"ps", "--all", "--no-trunc"}
		format := "{{.ID}}"
		expected := ids
		if kind == "volume" {
			args = []string{"volume", "ls"}
			format = "{{.Name}}"
			expected = volumes
		}
		args = append(args, "--filter", "label=qs.retirement.operation="+b.operation, "--filter", "label=qs.retirement.run="+b.actualRun, "--format", format)
		raw, err := lifecycleDocker(ctx, docker, args...)
		if err != nil || !lifecycleMaterialNativeSet(raw, expected) {
			return lifecycleError("lifecycle_restore_material_remaining_or_foreign")
		}
	}
	return nil
}
