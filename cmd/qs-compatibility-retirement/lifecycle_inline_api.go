package main

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	"github.com/FangcunMount/qs-server/pkg/version"
)

// Expected independently approved image/program/config facts are in the existing
// immutable request template. No completion, stop, fence or recovery flag exists.
type lifecycleAPIDeploymentControl struct {
	BImageID                  string `json:"b_image_id"`
	BProgramSHA256            string `json:"b_program_sha256"`
	RollbackSourceSHA         string `json:"rollback_source_sha"`
	RollbackImageID           string `json:"rollback_image_id"`
	RollbackProgramSHA256     string `json:"rollback_program_sha256"`
	OriginalRuntimeSpecSHA256 string `json:"original_runtime_spec_sha256"`
}

func (v *lifecycleAPIDeploymentControl) valid() bool {
	return v != nil && shaRE.MatchString(v.RollbackSourceSHA) && hashRE.MatchString(v.BProgramSHA256) &&
		hashRE.MatchString(v.RollbackProgramSHA256) && hashRE.MatchString(v.OriginalRuntimeSpecSHA256) &&
		lifecycleAPIImageID(v.BImageID) && lifecycleAPIImageID(v.RollbackImageID)
}
func lifecycleAPIImageID(v string) bool {
	return strings.HasPrefix(v, "sha256:") && hashRE.MatchString(strings.TrimPrefix(v, "sha256:"))
}

type lifecycleRecoveryReadback struct {
	direct  *backup.TargetRecoveryVerification
	resumed *backup.TargetLifecycleResult
}

func (v *lifecycleRecoveryReadback) verify(ctx context.Context, b backup.TargetRecoveryBorrowed, r lifecycleRequest, w *fence.MaintenanceWindow) error {
	if v == nil || (v.direct == nil) == (v.resumed == nil) {
		return lifecycleError("lifecycle_actual_no_migration_rollback_missing")
	}
	expected := r.Recovery
	expected.ActualRunID = r.ActualRunID
	if v.direct != nil {
		return v.direct.VerifyHostRecoveredTargets(ctx, b, expected, w)
	}
	return v.resumed.VerifyHostRecoveredTargets(ctx, b, expected, w)
}
func lifecycleInlineMigrationBindingMatches(r lifecycleRequest, o migration.CompatibilityPairMigrationObservation, w *fence.MaintenanceWindow, ctx context.Context) bool {
	if w == nil {
		return false
	}
	d, e := w.Diagnostic(ctx)
	b := o.Binding
	return e == nil && d.Binding == lifecycleWindowBinding(r) && d.DirectoryLeaseHeld && d.RemainingMilliseconds > 0 &&
		b.ApprovedSourceSHA == r.ToolSourceSHA && b.OriginalSourceSHA == r.OriginalSourceSHA && b.OperationID == r.OperationID &&
		b.OriginalRunID == r.Recovery.OriginalRunID && b.ActualRunID == r.ActualRunID && b.ManifestSHA256 == r.ManifestSHA256 &&
		b.ArchiveSHA256 == r.Recovery.ArchiveSHA256 && b.RequestSHA256 == digest(r.Recovery) && b.WindowStartSHA256 == d.StartSHA256 &&
		b.ResourcesSHA256 == migration.CompatibilityMigrationResourcesSHA256() && o.SQLBefore == 99 && o.SQLAfter == 100 &&
		o.MongoBefore == 38 && o.MongoAfter == 39 && o.SQLChanged && o.MongoChanged
}

// This uses only the same fixed local Docker Engine Unix socket. Expected JSON
// never replaces the same-process migration/recovery producer or native Window.
type lifecycleAPIEngine struct {
	client    *http.Client
	transport *http.Transport
}

func openLifecycleAPIEngine() (*lifecycleAPIEngine, error) {
	if runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 || version.GitCommit != sourceSHA || lifecycleDockerSocket() != nil {
		return nil, lifecycleError("lifecycle_actual_inline_b_deployment_missing")
	}
	t := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		if lifecycleDockerSocket() != nil {
			return nil, lifecycleError("lifecycle_fixed_local_docker_socket_unproven")
		}
		return (&net.Dialer{}).DialContext(ctx, "unix", "/run/docker.sock")
	}, DisableKeepAlives: true}
	return &lifecycleAPIEngine{client: &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, transport: t}, nil
}
func (v *lifecycleAPIEngine) call(ctx context.Context, method, path string, body []byte, expected int) ([]byte, error) {
	if v == nil || ctx == nil || ctx.Err() != nil || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\r\n\x00") || len(body) > 4<<20 {
		return nil, lifecycleError("lifecycle_api_operation_result_unknown")
	}
	q, c := context.WithTimeout(ctx, 45*time.Second)
	defer c()
	req, e := http.NewRequestWithContext(q, method, "http://docker"+path, bytes.NewReader(body))
	if e != nil {
		return nil, lifecycleError("lifecycle_api_operation_result_unknown")
	}
	req.Header.Set("Content-Type", "application/json")
	res, e := v.client.Do(req)
	if e != nil {
		return nil, lifecycleError("lifecycle_api_operation_result_unknown")
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	ce := res.Body.Close()
	if e != nil || ce != nil || len(raw) > 4<<20 || res.StatusCode != expected {
		return nil, lifecycleError("lifecycle_api_operation_result_unknown")
	}
	return raw, nil
}

type lifecycleAPISpec struct {
	Config     map[string]json.RawMessage `json:"Config"`
	HostConfig map[string]json.RawMessage `json:"HostConfig"`
	Networks   map[string]json.RawMessage `json:"Networks"`
}
type lifecycleAPIInspection struct {
	ID         string `json:"Id"`
	Name       string
	Image      string
	Config     map[string]json.RawMessage
	HostConfig map[string]json.RawMessage
	State      struct {
		Running, Paused, Restarting, Dead, OOMKilled bool
		PID, ExitCode                                int
		StartedAt                                    string
		Health                                       struct{ Status string }
	}
	NetworkSettings struct{ Networks map[string]json.RawMessage }
	ExecIDs         json.RawMessage
	Mounts          []json.RawMessage
}

func (v lifecycleAPIInspection) spec() lifecycleAPISpec {
	return lifecycleAPISpec{v.Config, v.HostConfig, v.NetworkSettings.Networks}
}
func lifecycleAPIInspect(ctx context.Context, e *lifecycleAPIEngine, id string) (lifecycleAPIInspection, error) {
	var v lifecycleAPIInspection
	if !hashRE.MatchString(id) {
		return v, lifecycleError("lifecycle_api_binding_rejected")
	}
	raw, err := e.call(ctx, http.MethodGet, "/containers/"+id+"/json", nil, 200)
	if err != nil || rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &v) != nil || v.ID != id || !lifecycleAPIImageID(v.Image) {
		return v, lifecycleError("lifecycle_api_binding_rejected")
	}
	return v, nil
}
func lifecycleAPIStaticMatches(v lifecycleAPIInspection, w stop.Container, expected string) bool {
	var entry, command []string
	var labels map[string]string
	if json.Unmarshal(v.Config["Entrypoint"], &entry) != nil || json.Unmarshal(v.Config["Cmd"], &command) != nil || json.Unmarshal(v.Config["Labels"], &labels) != nil {
		return false
	}
	return v.ID == w.ID && v.Name == w.Name && v.Image == w.Image && digest(v.spec()) == expected &&
		reflect.DeepEqual(entry, w.Entrypoint) && reflect.DeepEqual(command, w.Command) && labels["com.docker.compose.project"] == w.Project && labels["com.docker.compose.service"] == w.Service
}

type lifecycleAPITransition struct {
	self                                      *lifecycleAPITransition
	engine                                    *lifecycleAPIEngine
	docker, dir                               string
	request                                   lifecycleRequest
	original                                  lifecycleAPIInspection
	approved                                  stop.Container
	bProgramVerified, rollbackProgramVerified bool
	bCID, rollbackCID                         string
	removedOriginal                           bool
	unknown                                   bool
	materials                                 *lifecycleMaterialDirectory
	acceptance                                *lifecycleAPIRuntimeObservation
	programProbeIDs                           [2]string
	reopenedRecovery                          bool
}

// Prewarm runs before StartMaintenanceWindow. All probes are never started,
// network-none, exact image/CID/labels/no mounts, with intent before each action.
func prepareLifecycleAPITransition(ctx context.Context, r lifecycleRequest) (v *lifecycleAPITransition, result error) {
	if ctx == nil || ctx.Err() != nil || !r.DeploymentControl.valid() || r.ServiceControl == nil || r.ToolSourceSHA != sourceSHA || r.prepareRoot != lifecycleInvocationBatch(r.OperationID, r.ActualRunID) {
		return nil, lifecycleError("lifecycle_actual_inline_image_approval_missing")
	}
	engine, e := openLifecycleAPIEngine()
	if e != nil {
		return nil, e
	}
	v = &lifecycleAPITransition{engine: engine, request: r, dir: filepath.Join(r.prepareRoot, "api-transition")}
	v.self = v
	defer func() {
		if result != nil {
			engine.transport.CloseIdleConnections()
		}
	}()
	if e = validateLifecycleAPIInvocation(r); e != nil {
		return v, e
	}
	path := filepath.Join(lifecycleServicesRoot(r.OperationID, "server-a"), "approved-services.json")
	approval, e := stop.ReadApprovedDescriptor(path, r.ServiceControl.LocalDescriptorSHA256)
	if e != nil {
		return v, lifecycleError("lifecycle_api_binding_rejected")
	}
	binding, e := approval.WindowBinding(ctx)
	if e != nil || binding != lifecycleWindowBinding(r) {
		return v, lifecycleError("lifecycle_api_binding_rejected")
	}
	var descriptor stop.Descriptor
	if readLifecyclePrivate(path, r.ServiceControl.LocalDescriptorSHA256, &descriptor) != nil || descriptor.RuntimeSourceSHA != r.DeploymentControl.RollbackSourceSHA {
		return v, lifecycleError("lifecycle_api_binding_rejected")
	}
	found := 0
	for _, c := range descriptor.Containers {
		if c.Component == "qs-apiserver" {
			v.approved = c
			found++
		}
	}
	if found != 1 || v.approved.Image != r.DeploymentControl.RollbackImageID {
		return v, lifecycleError("lifecycle_api_binding_rejected")
	}
	v.docker = descriptor.DockerPath
	if _, e = readLifecycleRootFile(v.docker, descriptor.DockerSHA256, true); e != nil {
		return v, e
	}
	original, e := lifecycleAPIInspect(ctx, engine, v.approved.ID)
	if e != nil || !lifecycleAPIStaticMatches(original, v.approved, r.DeploymentControl.OriginalRuntimeSpecSHA256) || original.State.Paused || original.State.Restarting || original.State.Dead || original.State.OOMKilled {
		return v, lifecycleError("lifecycle_api_binding_rejected")
	}
	v.original = original
	if e = os.Mkdir(v.dir, 0700); e != nil {
		return v, lifecycleError("lifecycle_api_existing_or_unknown")
	}
	if e = lifecycleProtectedServiceDirectory(v.dir); e != nil {
		return v, e
	}
	if e = syncLifecycleDirectory(r.prepareRoot); e != nil {
		return v, e
	}
	v.materials, e = openLifecycleMaterialDirectory(v.dir, 0)
	if e != nil {
		return v, e
	}
	raw, e := json.Marshal(original.spec())
	if e != nil {
		return v, lifecycleError("lifecycle_api_binding_rejected")
	}
	if e = v.writeMaterial("original-runtime-spec.private.json", raw); e != nil {
		return v, e
	}
	if e = v.verifyImageProgram(ctx, "b", r.DeploymentControl.BImageID, r.ToolSourceSHA, r.DeploymentControl.BProgramSHA256); e != nil {
		return v, e
	}
	v.bProgramVerified = true
	if e = v.verifyImageProgram(ctx, "rollback", r.DeploymentControl.RollbackImageID, r.DeploymentControl.RollbackSourceSHA, r.DeploymentControl.RollbackProgramSHA256); e != nil {
		return v, e
	}
	v.rollbackProgramVerified = true
	intentRaw, e := readLifecycleAPIRecord(filepath.Join(r.prepareRoot, "native-call.intent.private.json"))
	if e != nil {
		return v, e
	}
	basis := lifecycleAPIRecoveryBasis{FormatVersion: 1, ToolSourceSHA: r.ToolSourceSHA, OriginalSourceSHA: r.OriginalSourceSHA, OperationID: r.OperationID, OriginalRunID: r.Recovery.OriginalRunID, ActualRunID: r.ActualRunID, ManifestSHA256: r.ManifestSHA256, NativeIntentSHA256: digestRaw(intentRaw), DeploymentSHA256: digest(r.DeploymentControl), DescriptorSHA256: r.ServiceControl.LocalDescriptorSHA256, Original: v.approved, OriginalPID: original.State.PID, ProbeIDs: v.programProbeIDs}
	if e = v.record("original-recovery-basis", basis); e != nil {
		return v, e
	}
	return v, nil
}

// Written by the actual original prewarm producer before any service/database
// mutation. It contains no outcome flag and grants no acceptance/purge authority.
type lifecycleAPIRecoveryBasis struct {
	FormatVersion                                                                                                                                     int `json:"format_version"`
	ToolSourceSHA, OriginalSourceSHA, OperationID, OriginalRunID, ActualRunID, ManifestSHA256, NativeIntentSHA256, DeploymentSHA256, DescriptorSHA256 string
	Original                                                                                                                                          stop.Container
	OriginalPID                                                                                                                                       int
	ProbeIDs                                                                                                                                          [2]string
}

func reopenLifecycleAPITransition(ctx context.Context, r lifecycleRequest) (v *lifecycleAPITransition, result error) {
	if ctx == nil || ctx.Err() != nil || r.Resume == nil || !r.DeploymentControl.valid() || r.ServiceControl == nil || r.ToolSourceSHA != sourceSHA || r.Recovery.ActualRunID == r.ActualRunID || validateLifecycleAPIInvocation(r) != nil {
		return nil, lifecycleError("lifecycle_api_original_recovery_basis_missing")
	}
	oldRoot := lifecycleInvocationBatch(r.OperationID, r.Recovery.ActualRunID)
	dir := filepath.Join(oldRoot, "api-transition")
	raw, e := readLifecycleAPIRecord(filepath.Join(dir, "original-recovery-basis.private.json"))
	if e != nil {
		return nil, e
	}
	var b lifecycleAPIRecoveryBasis
	if rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &b) != nil || !bytes.Equal(append(mustLifecycleJSON(b), '\n'), raw) || b.FormatVersion != 1 || b.ToolSourceSHA != r.ToolSourceSHA || b.OriginalSourceSHA != r.OriginalSourceSHA || b.OperationID != r.OperationID || b.OriginalRunID != r.Recovery.OriginalRunID || b.ActualRunID != r.Recovery.ActualRunID || b.ManifestSHA256 != r.ManifestSHA256 || b.DeploymentSHA256 != digest(r.DeploymentControl) || b.DescriptorSHA256 != r.ServiceControl.LocalDescriptorSHA256 {
		return nil, lifecycleError("lifecycle_api_original_recovery_basis_missing")
	}
	intentRaw, e := readLifecycleAPIRecord(filepath.Join(oldRoot, "native-call.intent.private.json"))
	if e != nil || digestRaw(intentRaw) != b.NativeIntentSHA256 {
		return nil, lifecycleError("lifecycle_api_original_recovery_basis_missing")
	}
	intent, e := decodeLifecycleAPIInvocationIntent(intentRaw)
	if e != nil || intent.Stage != "apply" || intent.ToolSourceSHA != sourceSHA || intent.OriginalSourceSHA != r.OriginalSourceSHA || intent.OperationID != r.OperationID || intent.OriginalRunID != r.Recovery.OriginalRunID || intent.ActualRunID != b.ActualRunID || intent.ManifestSHA256 != r.ManifestSHA256 || intent.DropAuthority || intent.BImageID != r.DeploymentControl.BImageID || intent.BProgramSHA256 != r.DeploymentControl.BProgramSHA256 {
		return nil, lifecycleError("lifecycle_api_original_recovery_basis_missing")
	}
	if _, e = readLifecycleRootFile(intent.NativePath, intent.NativeSHA256, true); e != nil {
		return nil, e
	}
	descriptorPath := filepath.Join(lifecycleServicesRoot(r.OperationID, "server-a"), "approved-services.json")
	approval, e := stop.ReadApprovedDescriptor(descriptorPath, r.ServiceControl.LocalDescriptorSHA256)
	if e != nil {
		return nil, e
	}
	binding, e := approval.WindowBinding(ctx)
	if e != nil || binding != lifecycleWindowBinding(r) {
		return nil, lifecycleError("lifecycle_api_binding_rejected")
	}
	var descriptor stop.Descriptor
	if readLifecyclePrivate(descriptorPath, r.ServiceControl.LocalDescriptorSHA256, &descriptor) != nil {
		return nil, lifecycleError("lifecycle_api_binding_rejected")
	}
	matched := 0
	for _, c := range descriptor.Containers {
		if c.Component == "qs-apiserver" {
			if !reflect.DeepEqual(c, b.Original) {
				return nil, lifecycleError("lifecycle_api_binding_rejected")
			}
			matched++
		}
	}
	if matched != 1 {
		return nil, lifecycleError("lifecycle_api_binding_rejected")
	}
	engine, e := openLifecycleAPIEngine()
	if e != nil {
		return nil, e
	}
	v = &lifecycleAPITransition{engine: engine, request: r, dir: dir, docker: descriptor.DockerPath, approved: b.Original, programProbeIDs: b.ProbeIDs, reopenedRecovery: true}
	v.self = v
	defer func() {
		if result != nil {
			engine.transport.CloseIdleConnections()
			if v.materials != nil {
				_ = v.materials.close()
			}
		}
	}()
	specRaw, e := readLifecycleAPIRecord(filepath.Join(dir, "original-runtime-spec.private.json"))
	if e != nil {
		return v, e
	}
	var spec lifecycleAPISpec
	if rejectDuplicateJSON(specRaw) != nil || json.Unmarshal(specRaw, &spec) != nil || digest(spec) != r.DeploymentControl.OriginalRuntimeSpecSHA256 {
		return v, lifecycleError("lifecycle_api_binding_rejected")
	}
	v.original.ID = b.Original.ID
	v.original.Name = b.Original.Name
	v.original.Image = b.Original.Image
	v.original.Config = spec.Config
	v.original.HostConfig = spec.HostConfig
	v.original.NetworkSettings.Networks = spec.Networks
	v.original.State.PID = b.OriginalPID
	v.original.State.Running = b.Original.Running
	v.original.State.StartedAt = b.Original.StartedAt
	if !lifecycleAPIStaticMatches(v.original, b.Original, r.DeploymentControl.OriginalRuntimeSpecSHA256) {
		return v, lifecycleError("lifecycle_api_binding_rejected")
	}
	if _, e = readLifecycleRootFile(v.docker, descriptor.DockerSHA256, true); e != nil {
		return v, e
	}
	for i, kind := range []string{"b", "rollback"} {
		image, src, hash := r.DeploymentControl.BImageID, r.ToolSourceSHA, r.DeploymentControl.BProgramSHA256
		if kind == "rollback" {
			image, src, hash = r.DeploymentControl.RollbackImageID, r.DeploymentControl.RollbackSourceSHA, r.DeploymentControl.RollbackProgramSHA256
		}
		program, e := readLifecycleRootFile(filepath.Join(dir, kind+"-program"), hash, true)
		if e != nil {
			return v, e
		}
		info, e := buildinfo.Read(bytes.NewReader(program))
		if e != nil || !lifecycleCompiledProgramSourceMatches(info, src, runtime.GOARCH) {
			return v, lifecycleError("lifecycle_api_image_program_unproven")
		}
		actualRaw, e := engine.call(ctx, http.MethodGet, "/images/"+image+"/json", nil, 200)
		var actual struct {
			ID               string `json:"Id"`
			Architecture, Os string
			Config           struct{ Labels map[string]string }
		}
		if e != nil || rejectDuplicateJSON(actualRaw) != nil || json.Unmarshal(actualRaw, &actual) != nil || actual.ID != image || actual.Os != "linux" || actual.Architecture != runtime.GOARCH || !lifecycleImageRevisionMatches(kind, actual.Config.Labels, src) {
			return v, lifecycleError("lifecycle_api_image_program_unproven")
		}
		if !hashRE.MatchString(b.ProbeIDs[i]) || lifecycleReadAPIStrings(dir, kind+"-program-create-result", map[string]string{"id": b.ProbeIDs[i]}) != nil || lifecycleReadAPIStrings(dir, kind+"-program-copy-result", map[string]string{"id": b.ProbeIDs[i], "program_sha256": hash}) != nil || lifecycleReadAPIStrings(dir, kind+"-program-remove-result", map[string]string{"id": b.ProbeIDs[i], "state": "absent"}) != nil {
			return v, lifecycleError("lifecycle_api_image_program_unproven")
		}
		if _, e = engine.call(ctx, http.MethodGet, "/containers/"+b.ProbeIDs[i]+"/json", nil, 404); e != nil {
			return v, e
		}
	}
	v.bProgramVerified = true
	v.rollbackProgramVerified = true
	// Reopen only known, settled original B effects. Missing/unknown results are
	// not replayable. The original API's absence is independently read natively.
	removeIntent := filepath.Join(dir, "b-api-remove-intent.private.json")
	if _, e = os.Lstat(removeIntent); e == nil {
		if lifecycleReadAPIStrings(dir, "b-api-remove-intent", map[string]string{"id": b.Original.ID}) != nil || lifecycleReadAPIStrings(dir, "b-api-remove-result", map[string]string{"id": b.Original.ID, "state": "absent"}) != nil {
			return v, lifecycleError("lifecycle_api_operation_result_unknown")
		}
		if _, e = engine.call(ctx, http.MethodGet, "/containers/"+b.Original.ID+"/json", nil, 404); e != nil {
			return v, e
		}
		v.removedOriginal = true
	} else if !os.IsNotExist(e) {
		return v, lifecycleError("lifecycle_api_operation_result_unknown")
	} else {
		current, e := lifecycleAPIInspect(ctx, engine, b.Original.ID)
		if e != nil || !lifecycleAPIStaticMatches(current, b.Original, r.DeploymentControl.OriginalRuntimeSpecSHA256) || current.State.Running || current.State.PID != 0 {
			return v, lifecycleError("lifecycle_api_original_stop_unproven")
		}
	}
	createdPath := filepath.Join(dir, "b-api-create-intent.private.json")
	if _, e = os.Lstat(createdPath); e == nil {
		body, e := readLifecycleAPIRecord(filepath.Join(dir, "b-api-create-body.private.json"))
		if e != nil {
			return v, e
		}
		old := r
		old.ActualRunID = b.ActualRunID
		expected, e := lifecycleAPICreateBody(v.original, r.DeploymentControl.BImageID, false, old)
		if e != nil || !bytes.Equal(body, expected) || lifecycleReadAPIStrings(dir, "b-api-create-intent", map[string]string{"name": strings.TrimPrefix(v.original.Name, "/"), "image": r.DeploymentControl.BImageID, "body_sha256": digestRaw(body), "program_sha256": r.DeploymentControl.BProgramSHA256}) != nil {
			return v, lifecycleError("lifecycle_api_operation_result_unknown")
		}
		resultRaw, e := readLifecycleAPIRecord(filepath.Join(dir, "b-api-create-result.private.json"))
		var result map[string]string
		if e != nil || rejectDuplicateJSON(resultRaw) != nil || json.Unmarshal(resultRaw, &result) != nil || len(result) != 1 || !hashRE.MatchString(result["id"]) {
			return v, lifecycleError("lifecycle_api_operation_result_unknown")
		}
		v.bCID = result["id"]
		startRaw, e := readLifecycleAPIRecord(filepath.Join(dir, "b-api-start-result.private.json"))
		var started map[string]string
		if e != nil || rejectDuplicateJSON(startRaw) != nil || json.Unmarshal(startRaw, &started) != nil || len(started) != 4 || started["id"] != v.bCID || started["image"] != r.DeploymentControl.BImageID || started["program_sha256"] != r.DeploymentControl.BProgramSHA256 || started["started_at"] == "" || lifecycleReadAPIStrings(dir, "b-api-start-intent", map[string]string{"id": v.bCID}) != nil {
			return v, lifecycleError("lifecycle_api_operation_result_unknown")
		}
		current, e := lifecycleAPIInspect(ctx, engine, v.bCID)
		if e != nil || current.State.StartedAt != started["started_at"] || !lifecycleAPIExpectedConfiguration(current, v.original, r.DeploymentControl.BImageID, digestRaw(body), body) || current.State.Paused || current.State.Restarting || current.State.Dead || current.State.OOMKilled {
			return v, lifecycleError("lifecycle_api_runtime_binding_unproven")
		}
	} else if !os.IsNotExist(e) {
		return v, lifecycleError("lifecycle_api_operation_result_unknown")
	}
	// A recovery-created rollback with unfinished effects is not silently replayed.
	for _, name := range []string{"rollback-api-remove-intent", "rollback-api-create-intent", "rollback-api-start-intent"} {
		if _, e = os.Lstat(filepath.Join(dir, name+".private.json")); !os.IsNotExist(e) {
			return v, lifecycleError("lifecycle_api_operation_result_unknown")
		}
	}
	v.materials, e = openLifecycleMaterialDirectory(dir, 0)
	if e != nil {
		return v, e
	}
	return v, nil
}
func mustLifecycleJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }
func lifecycleReadAPIStrings(dir, name string, expected map[string]string) error {
	raw, e := readLifecycleAPIRecord(filepath.Join(dir, name+".private.json"))
	var value map[string]string
	if e != nil || rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &value) != nil || !reflect.DeepEqual(value, expected) {
		return lifecycleError("lifecycle_api_operation_result_unknown")
	}
	return nil
}

func (v *lifecycleAPITransition) record(name string, value any) error {
	raw, e := json.Marshal(value)
	if e != nil {
		return lifecycleError("lifecycle_api_journal_rejected")
	}
	return v.writeMaterial(name+".private.json", append(raw, '\n'))
}

// Register bytes/FD identity only at this original owner's actual successful
// O_EXCL+fsync write. A later walker cannot adopt an unregistered file or secret.
func (v *lifecycleAPITransition) writeMaterial(name string, raw []byte) error {
	if v == nil || v.self != v || v.materials == nil || v.unknown || v.materials.unchanged() != nil || filepath.Base(name) != name || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return lifecycleError("lifecycle_api_material_owner_missing")
	}
	fd, e := unix.Openat(int(v.materials.file.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW, 0600)
	if e != nil {
		v.unknown, v.materials.unknown = true, true
		return lifecycleError("lifecycle_api_material_owner_missing")
	}
	f := os.NewFile(uintptr(fd), filepath.Join(v.dir, name))
	_, writeErr := f.Write(raw)
	syncErr, closeErr := f.Sync(), f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || v.materials.file.Sync() != nil {
		v.unknown, v.materials.unknown = true, true
		return lifecycleError("lifecycle_api_material_owner_missing")
	}
	if e := v.materials.register(name, digestRaw(raw), 0, 0600); e != nil {
		v.unknown, v.materials.unknown = true, true
		return e
	}
	return nil
}

func (v *lifecycleAPITransition) verifyImageProgram(ctx context.Context, kind, image, source, expected string) error {
	index := -1
	switch kind {
	case "b":
		index = 0
	case "rollback":
		index = 1
	}
	if v == nil || v.self != v || index < 0 || v.programProbeIDs[index] != "" {
		return lifecycleError("lifecycle_api_image_program_unproven")
	}
	raw, e := v.engine.call(ctx, http.MethodGet, "/images/"+image+"/json", nil, 200)
	var actual struct {
		ID               string `json:"Id"`
		Architecture, Os string
		Config           struct{ Labels map[string]string }
	}
	if e != nil || rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &actual) != nil || actual.ID != image || actual.Os != "linux" || actual.Architecture != runtime.GOARCH || !lifecycleImageRevisionMatches(kind, actual.Config.Labels, source) {
		return lifecycleError("lifecycle_api_image_program_unproven")
	}
	name := "qs-retirement-program-" + v.request.OperationID + "-" + v.request.ActualRunID + "-" + kind
	labels, e := lifecycleAPIProgramProbeLabels(actual.Config.Labels, map[string]string{"codex.task": "qs-compatibility-retirement", "codex.operation": v.request.OperationID, "codex.run": v.request.ActualRunID, "codex.tool_source": sourceSHA, "codex.kind": "api-program-" + kind})
	if e != nil {
		return e
	}
	body, _ := json.Marshal(map[string]any{"Image": image, "Entrypoint": []string{"/app/qs-apiserver"}, "Labels": labels, "HostConfig": map[string]any{"NetworkMode": "none", "AutoRemove": false}})
	if e = v.record(kind+"-program-create-intent", map[string]any{"name": name, "image": image, "labels": labels, "program_sha256": expected}); e != nil {
		return e
	}
	created, e := v.engine.call(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(name), body, 201)
	var result struct {
		ID       string `json:"Id"`
		Warnings []string
	}
	if e != nil || rejectDuplicateJSON(created) != nil || json.Unmarshal(created, &result) != nil || !hashRE.MatchString(result.ID) || len(result.Warnings) != 0 {
		v.unknown = true
		return lifecycleError("lifecycle_api_image_program_unproven")
	}
	v.programProbeIDs[index] = result.ID
	if e = v.record(kind+"-program-create-result", map[string]string{"id": result.ID}); e != nil {
		v.unknown = true
		return e
	}
	probe, e := lifecycleAPIInspect(ctx, v.engine, result.ID)
	var foundLabels map[string]string
	var mode string
	if e != nil || json.Unmarshal(probe.Config["Labels"], &foundLabels) != nil || json.Unmarshal(probe.HostConfig["NetworkMode"], &mode) != nil || probe.Name != "/"+name || probe.Image != image || probe.State.Running || probe.State.PID != 0 || len(probe.Mounts) != 0 || mode != "none" || !reflect.DeepEqual(foundLabels, labels) {
		v.unknown = true
		return lifecycleError("lifecycle_api_image_program_unproven")
	}
	program := filepath.Join(v.dir, kind+"-program")
	if e = v.record(kind+"-program-copy-intent", map[string]string{"id": result.ID, "program_sha256": expected}); e != nil {
		return e
	}
	if _, e = lifecycleDocker(ctx, v.docker, "cp", result.ID+":/app/qs-apiserver", program); e != nil {
		v.unknown = true
		return e
	}
	programRaw, e := readLifecycleRootFile(program, expected, true)
	if e != nil {
		v.unknown = true
		return e
	}
	compiled, e := buildinfo.Read(bytes.NewReader(programRaw))
	if e != nil || !lifecycleCompiledProgramSourceMatches(compiled, source, runtime.GOARCH) {
		v.unknown = true
		return lifecycleError("lifecycle_api_image_program_unproven")
	}
	if kind == "rollback" {
		if e = v.verifyOriginalRuntimeVersion(ctx, image, source); e != nil {
			v.unknown = true
			return e
		}
	}
	info, e := os.Lstat(program)
	if e != nil || v.materials == nil || v.materials.register(filepath.Base(program), expected, 0, info.Mode().Perm()) != nil {
		v.unknown = true
		return lifecycleError("lifecycle_api_material_owner_missing")
	}
	if v.materials.files[filepath.Base(program)].file.Sync() != nil || syncLifecycleDirectory(v.dir) != nil {
		v.unknown, v.materials.unknown = true, true
		return lifecycleError("lifecycle_api_material_owner_missing")
	}
	if e = v.record(kind+"-program-copy-result", map[string]string{"id": result.ID, "program_sha256": expected}); e != nil {
		return e
	}
	// Exact physical probe is re-observed before removal; no volume/image purge.
	again, e := lifecycleAPIInspect(ctx, v.engine, result.ID)
	if e != nil || !reflect.DeepEqual(probe, again) {
		v.unknown = true
		return lifecycleError("lifecycle_api_image_program_unproven")
	}
	if e = v.record(kind+"-program-remove-intent", map[string]string{"id": result.ID}); e != nil {
		return e
	}
	if _, e = v.engine.call(ctx, http.MethodDelete, "/containers/"+result.ID+"?force=false&v=false", nil, 204); e != nil {
		v.unknown = true
		return e
	}
	if _, e = v.engine.call(ctx, http.MethodGet, "/containers/"+result.ID+"/json", nil, 404); e != nil {
		v.unknown = true
		return e
	}
	return v.record(kind+"-program-remove-result", map[string]string{"id": result.ID, "state": "absent"})
}

// A legacy release image may predate OCI labels. The exact image/program and
// actual compiled source remain required; a present contradictory label fails.
func lifecycleImageRevisionMatches(kind string, labels map[string]string, source string) bool {
	revision, present := labels["org.opencontainers.image.revision"]
	return shaRE.MatchString(source) && (kind == "b" && present && revision == source || kind == "rollback" && (!present || revision == source))
}

func lifecycleCompiledProgramSourceMatches(info *debug.BuildInfo, source, architecture string) bool {
	if info == nil || !shaRE.MatchString(source) || (architecture != "amd64" && architecture != "arm64") {
		return false
	}
	seen, osValue, archValue, flags := map[string]bool{}, "", "", ""
	for _, v := range info.Settings {
		if v.Key != "GOOS" && v.Key != "GOARCH" && v.Key != "-ldflags" {
			continue
		}
		if seen[v.Key] {
			return false
		}
		seen[v.Key] = true
		switch v.Key {
		case "GOOS":
			osValue = v.Value
		case "GOARCH":
			archValue = v.Value
		case "-ldflags":
			flags = v.Value
		}
	}
	if osValue != "linux" || archValue != architecture {
		return false
	}
	parts, count := strings.Fields(flags), 0
	key := "github.com/FangcunMount/qs-server/pkg/version.GitCommit="
	for i := 0; i < len(parts); i++ {
		value := ""
		if parts[i] == "-X" {
			i++
			if i >= len(parts) {
				return false
			}
			value = parts[i]
		} else if strings.HasPrefix(parts[i], "-X=") {
			value = strings.TrimPrefix(parts[i], "-X=")
		}
		if strings.HasPrefix(value, key) {
			if strings.TrimPrefix(value, key) != source {
				return false
			}
			count++
		}
	}
	return count == 1
}

func lifecycleOriginalRuntimeVersionMatches(before, after, original lifecycleAPIInspection, image, source string, raw []byte) bool {
	var build struct {
		Code    *int         `json:"code"`
		Message string       `json:"message"`
		Data    version.Info `json:"data"`
	}
	return before.ID == original.ID && before.Image == image && before.State.Running && before.State.PID > 0 &&
		!before.State.Paused && !before.State.Restarting && !before.State.Dead && !before.State.OOMKilled &&
		before.State.StartedAt == original.State.StartedAt && before.State.PID == original.State.PID &&
		reflect.DeepEqual(before.spec(), original.spec()) && reflect.DeepEqual(before, after) &&
		rejectDuplicateJSON(raw) == nil && json.Unmarshal(raw, &build) == nil && build.Code != nil && *build.Code == 0 &&
		build.Message == "success" && build.Data.GitCommit == source && build.Data.Platform == "linux/"+runtime.GOARCH
}

func (v *lifecycleAPITransition) verifyOriginalRuntimeVersion(ctx context.Context, image, source string) error {
	before, e := lifecycleAPIInspect(ctx, v.engine, v.original.ID)
	if e != nil {
		return e
	}
	address, e := lifecycleAPIReadAddress(before)
	if e != nil {
		return e
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DialContext: func(q context.Context, network, addr string) (net.Conn, error) {
		if network != "tcp" || addr != address {
			return nil, lifecycleError("lifecycle_actual_api_fixed_read_route_unproven")
		}
		return (&net.Dialer{}).DialContext(q, "tcp", address)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	raw, e := lifecycleAPIReadEndpoint(ctx, client, address, "/version")
	if e != nil {
		return e
	}
	after, e := lifecycleAPIInspect(ctx, v.engine, v.original.ID)
	if e != nil || !lifecycleOriginalRuntimeVersionMatches(before, after, v.original, image, source, raw) {
		return lifecycleError("lifecycle_api_image_program_unproven")
	}
	return v.record("rollback-program-runtime-version", map[string]string{"id": before.ID, "image": image, "source_sha": source, "version_sha256": digestRaw(raw)})
}

// Docker inherits all image labels in a never-started probe. Bind that exact
// original set plus the five owner keys, rejecting conflicting inherited owners.
func lifecycleAPIProgramProbeLabels(image, owned map[string]string) (map[string]string, error) {
	labels := make(map[string]string, len(image)+len(owned))
	for key, value := range image {
		labels[key] = value
	}
	for key, value := range owned {
		if inherited, exists := image[key]; exists && inherited != value {
			return nil, lifecycleError("lifecycle_api_image_program_unproven")
		}
		labels[key] = value
	}
	return labels, nil
}

type lifecycleAPIInvocationIntent struct {
	FormatVersion       int    `json:"format_version"`
	Kind                string `json:"kind"`
	DispatcherSourceSHA string `json:"dispatcher_source_sha"`
	ToolSourceSHA       string `json:"tool_source_sha"`
	OriginalSourceSHA   string `json:"original_source_sha"`
	OperationID         string `json:"operation_id"`
	OriginalRunID       string `json:"original_run_id"`
	ActualRunID         string `json:"actual_run_id"`
	Stage               string `json:"stage"`
	TemplateSHA256      string `json:"approved_template_sha256"`
	DerivedSHA256       string `json:"derived_request_sha256"`
	ManifestSHA256      string `json:"manifest_sha256"`
	PackageSHA256       string `json:"package_sha256"`
	ToolDirectory       string `json:"tool_directory"`
	ToolProgramSHA256   string `json:"tool_program_sha256"`
	NativeSHA256        string `json:"native_sha256"`
	NativePath          string `json:"native_path"`
	BImageID            string `json:"b_image_id"`
	BProgramSHA256      string `json:"b_program_sha256"`
	SourceUID           uint32 `json:"source_uid"`
	DropAuthority       bool   `json:"drop_authority"`
}

func validateLifecycleAPIInvocation(r lifecycleRequest) error {
	// Actual root-written intent is expected input provenance, not an authority
	// producer. The host still demands concrete native transition/readback proof.
	path := filepath.Join(r.prepareRoot, "native-call.intent.private.json")
	raw, e := readLifecycleAPIRecord(path)
	i, decodeErr := decodeLifecycleAPIInvocationIntent(raw)
	if e != nil || decodeErr != nil {
		return lifecycleError("lifecycle_api_binding_rejected")
	}
	if i.FormatVersion != 1 || i.Kind != "independent_window_tool_native_invocation" || !shaRE.MatchString(i.DispatcherSourceSHA) || i.ToolSourceSHA != sourceSHA || i.ToolSourceSHA != r.ToolSourceSHA ||
		i.OriginalSourceSHA != r.OriginalSourceSHA || i.OperationID != r.OperationID || i.OriginalRunID != r.Recovery.OriginalRunID || i.ActualRunID != r.ActualRunID ||
		(i.Stage != "apply" && i.Stage != "recover") || i.ManifestSHA256 != r.ManifestSHA256 || i.DerivedSHA256 != r.requestSHA256 || !hashRE.MatchString(i.TemplateSHA256) ||
		!hashRE.MatchString(i.PackageSHA256) || !hashRE.MatchString(i.ToolProgramSHA256) || !hashRE.MatchString(i.NativeSHA256) || i.DropAuthority ||
		i.BImageID != r.DeploymentControl.BImageID || i.BProgramSHA256 != r.DeploymentControl.BProgramSHA256 {
		return lifecycleError("lifecycle_api_binding_rejected")
	}
	exe, e := os.Executable()
	if e != nil || exe != i.NativePath || exe != filepath.Join(lifecycleServicesRoot(r.OperationID, "server-a"), "qs-compatibility-retirement") {
		return lifecycleError("lifecycle_api_binding_rejected")
	}
	_, e = readLifecycleRootFile(exe, i.NativeSHA256, true)
	return e
}
func decodeLifecycleAPIInvocationIntent(raw []byte) (lifecycleAPIInvocationIntent, error) {
	var v lifecycleAPIInvocationIntent
	var fields map[string]json.RawMessage
	if rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(v)) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != reflect.TypeOf(v).NumField() {
		return v, lifecycleError("lifecycle_api_binding_rejected")
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return v, lifecycleError("lifecycle_api_binding_rejected")
		}
	}
	if !bytes.Equal(bytes.TrimSpace(fields["drop_authority"]), []byte("false")) || json.Unmarshal(raw, &v) != nil {
		return v, lifecycleError("lifecycle_api_binding_rejected")
	}
	return v, nil
}
func readLifecycleAPIRecord(path string) ([]byte, error) {
	if lifecycleProtectedServiceDirectory(filepath.Dir(path)) != nil {
		return nil, lifecycleError("lifecycle_api_journal_rejected")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, lifecycleError("lifecycle_api_journal_rejected")
	}
	before, be := f.Stat()
	st, ok := infoStat(before)
	if be != nil || !ok || !before.Mode().IsRegular() || before.Mode().Perm() != 0600 || st.Uid != 0 || st.Nlink != 1 || before.Size() < 1 || before.Size() > 256<<10 {
		_ = f.Close()
		return nil, lifecycleError("lifecycle_api_journal_rejected")
	}
	raw, re := io.ReadAll(io.LimitReader(f, (256<<10)+1))
	after, ae := f.Stat()
	ce := f.Close()
	named, ne := os.Lstat(path)
	if re != nil || ae != nil || ce != nil || ne != nil || len(raw) > 256<<10 || !sameLifecycleFile(before, after) || !sameLifecycleFile(before, named) {
		return nil, lifecycleError("lifecycle_api_journal_rejected")
	}
	return raw, nil
}
func lifecycleAPINoMigrationCommand(original []string) ([]string, error) {
	command := []string{}
	for i := 0; i < len(original); i++ {
		value := original[i]
		if value == "--migration.enabled" {
			if i+1 >= len(original) || original[i+1] != "true" && original[i+1] != "false" {
				return nil, lifecycleError("lifecycle_api_binding_rejected")
			}
			i++
			continue
		}
		if strings.HasPrefix(value, "--migration.enabled=") {
			if value != "--migration.enabled=true" && value != "--migration.enabled=false" {
				return nil, lifecycleError("lifecycle_api_binding_rejected")
			}
			continue
		}
		command = append(command, value)
	}
	return append(command, "--migration.enabled=false"), nil
}
func lifecycleAPICreateBody(original lifecycleAPIInspection, image string, rollback bool, r lifecycleRequest) ([]byte, error) {
	if !lifecycleAPIImageID(image) || len(original.Config) == 0 || len(original.HostConfig) == 0 || len(original.NetworkSettings.Networks) == 0 {
		return nil, lifecycleError("lifecycle_api_binding_rejected")
	}
	config := map[string]json.RawMessage{}
	for k, v := range original.Config {
		config[k] = append(json.RawMessage(nil), v...)
	}
	config["Image"], _ = json.Marshal(image)
	var command, entry []string
	var labels map[string]string
	if json.Unmarshal(config["Cmd"], &command) != nil || json.Unmarshal(config["Entrypoint"], &entry) != nil || !reflect.DeepEqual(entry, []string{"/app/qs-apiserver"}) || json.Unmarshal(config["Labels"], &labels) != nil || len(labels) == 0 {
		return nil, lifecycleError("lifecycle_api_binding_rejected")
	}
	normalized, e := lifecycleAPINoMigrationCommand(command)
	if e != nil {
		return nil, e
	}
	if !rollback {
		normalized[len(normalized)-1] = "--migration.enabled=true"
	}
	config["Cmd"], _ = json.Marshal(normalized)
	for k, v := range map[string]string{"codex.qs_retirement.operation": r.OperationID, "codex.qs_retirement.run": r.ActualRunID, "codex.qs_retirement.tool_source": r.ToolSourceSHA} {
		if old, ok := labels[k]; ok && old != v {
			return nil, lifecycleError("lifecycle_api_binding_rejected")
		}
		labels[k] = v
	}
	config["Labels"], _ = json.Marshal(labels)
	endpoints := map[string]map[string]json.RawMessage{}
	for name, raw := range original.NetworkSettings.Networks {
		var endpoint map[string]json.RawMessage
		if name == "" || json.Unmarshal(raw, &endpoint) != nil {
			return nil, lifecycleError("lifecycle_api_binding_rejected")
		}
		projection := map[string]json.RawMessage{}
		for _, key := range []string{"IPAMConfig", "Links", "Aliases", "DriverOpts", "GwPriority"} {
			if value, ok := endpoint[key]; ok {
				projection[key] = value
			}
		}
		endpoints[name] = projection
	}
	payload := map[string]any{"HostConfig": original.HostConfig, "NetworkingConfig": map[string]any{"EndpointsConfig": endpoints}}
	for key, value := range config {
		payload[key] = value
	}
	body, e := json.Marshal(payload)
	if e != nil || len(body) > 4<<20 {
		return nil, lifecycleError("lifecycle_api_binding_rejected")
	}
	return body, nil
}
func lifecycleAPIExpectedConfiguration(v lifecycleAPIInspection, original lifecycleAPIInspection, image, bodyHash string, body []byte) bool {
	if !hashRE.MatchString(v.ID) || v.Image != image || v.Name != original.Name || digestRaw(body) != bodyHash {
		return false
	}
	var expected map[string]json.RawMessage
	if json.Unmarshal(body, &expected) != nil {
		return false
	}
	for name := range original.Config {
		if !lifecycleAPIJSONEqual(v.Config[name], expected[name]) {
			return false
		}
	}
	var want map[string]json.RawMessage
	if json.Unmarshal(expected["HostConfig"], &want) != nil {
		return false
	}
	for name, value := range want {
		if !lifecycleAPIJSONEqual(value, v.HostConfig[name]) {
			return false
		}
	}
	if len(v.NetworkSettings.Networks) != len(original.NetworkSettings.Networks) {
		return false
	}
	var networking struct {
		EndpointsConfig map[string]map[string]json.RawMessage
	}
	if json.Unmarshal(expected["NetworkingConfig"], &networking) != nil {
		return false
	}
	for name, originalRaw := range original.NetworkSettings.Networks {
		var actualEndpoint, originalEndpoint map[string]json.RawMessage
		if json.Unmarshal(v.NetworkSettings.Networks[name], &actualEndpoint) != nil || json.Unmarshal(originalRaw, &originalEndpoint) != nil {
			return false
		}
		for field, value := range networking.EndpointsConfig[name] {
			if !lifecycleAPIJSONEqual(value, actualEndpoint[field]) {
				return false
			}
		}
		if value, ok := originalEndpoint["NetworkID"]; ok && !lifecycleAPIJSONEqual(value, actualEndpoint["NetworkID"]) {
			return false
		}
	}
	return true
}
func lifecycleAPIJSONEqual(a, b []byte) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	decode := func(raw []byte) (any, error) {
		var v any
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		e := d.Decode(&v)
		return v, e
	}
	x, e := decode(a)
	y, f := decode(b)
	return e == nil && f == nil && reflect.DeepEqual(x, y)
}
func lifecycleAPIExpectedRuntime(v lifecycleAPIInspection, original lifecycleAPIInspection, image, bodyHash string, body []byte) bool {
	return lifecycleAPIExpectedConfiguration(v, original, image, bodyHash, body) && v.State.Running && v.State.PID > 0 &&
		!v.State.Paused && !v.State.Restarting && !v.State.Dead && !v.State.OOMKilled
}
func lifecycleAPIStoppedRuntime(v lifecycleAPIInspection, original lifecycleAPIInspection, image, bodyHash string, body []byte) bool {
	var execs []string
	return lifecycleAPIExpectedConfiguration(v, original, image, bodyHash, body) && !v.State.Running && v.State.PID == 0 && v.State.ExitCode == 0 &&
		!v.State.Paused && !v.State.Restarting && !v.State.Dead && !v.State.OOMKilled && len(v.ExecIDs) != 0 && json.Unmarshal(v.ExecIDs, &execs) == nil && len(execs) == 0
}
func (v *lifecycleAPITransition) deploy(ctx context.Context, r lifecycleRequest, rollback bool) error {
	if v == nil || v.self != v || v.unknown || v.request.ActualRunID != r.ActualRunID || v.request.OperationID != r.OperationID || v.request.ToolSourceSHA != r.ToolSourceSHA || !r.DeploymentControl.valid() ||
		!v.bProgramVerified || !v.rollbackProgramVerified {
		return lifecycleError("lifecycle_api_existing_or_unknown")
	}
	kind, image, program := "b", r.DeploymentControl.BImageID, r.DeploymentControl.BProgramSHA256
	if rollback {
		kind, image, program = "rollback", r.DeploymentControl.RollbackImageID, r.DeploymentControl.RollbackProgramSHA256
	}
	if _, e := readLifecycleRootFile(filepath.Join(v.dir, kind+"-program"), program, true); e != nil {
		return e
	}
	if v.bCID != "" && !rollback || v.rollbackCID != "" {
		return lifecycleError("lifecycle_api_existing_or_unknown")
	}
	remove := v.approved.ID
	if rollback && v.bCID != "" {
		remove = v.bCID
	}
	if !v.removedOriginal || remove != v.approved.ID {
		current, e := lifecycleAPIInspect(ctx, v.engine, remove)
		if e != nil || current.State.Running || current.State.PID != 0 || current.State.Paused || current.State.Restarting || current.State.Dead || current.State.OOMKilled {
			return lifecycleError("lifecycle_api_original_stop_unproven")
		}
		if remove == v.approved.ID && !lifecycleAPIStaticMatches(current, v.approved, r.DeploymentControl.OriginalRuntimeSpecSHA256) {
			return lifecycleError("lifecycle_api_binding_rejected")
		}
		if remove != v.approved.ID {
			body, readErr := readLifecycleAPIRecord(filepath.Join(v.dir, "b-api-create-body.private.json"))
			if readErr != nil || !lifecycleAPIStoppedRuntime(current, v.original, r.DeploymentControl.BImageID, digestRaw(body), body) {
				return lifecycleError("lifecycle_api_binding_rejected")
			}
		}
		if e = v.record(kind+"-api-remove-intent", map[string]string{"id": remove}); e != nil {
			return e
		}
		if _, e = v.engine.call(ctx, http.MethodDelete, "/containers/"+remove+"?force=false&v=false", nil, 204); e != nil {
			v.unknown = true
			return e
		}
		if _, e = v.engine.call(ctx, http.MethodGet, "/containers/"+remove+"/json", nil, 404); e != nil {
			v.unknown = true
			return e
		}
		if e = v.record(kind+"-api-remove-result", map[string]string{"id": remove, "state": "absent"}); e != nil {
			v.unknown = true
			return e
		}
		v.removedOriginal = true
	}
	body, e := lifecycleAPICreateBody(v.original, image, rollback, r)
	if e != nil {
		return e
	}
	if e = v.writeMaterial(kind+"-api-create-body.private.json", body); e != nil {
		return e
	}
	if e = v.record(kind+"-api-create-intent", map[string]string{"name": strings.TrimPrefix(v.original.Name, "/"), "image": image, "body_sha256": digestRaw(body), "program_sha256": program}); e != nil {
		return e
	}
	raw, e := v.engine.call(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(strings.TrimPrefix(v.original.Name, "/")), body, 201)
	var created struct {
		ID       string `json:"Id"`
		Warnings []string
	}
	if e != nil || rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &created) != nil || !hashRE.MatchString(created.ID) || len(created.Warnings) != 0 {
		v.unknown = true
		return lifecycleError("lifecycle_api_operation_result_unknown")
	}
	if e = v.record(kind+"-api-create-result", map[string]string{"id": created.ID}); e != nil {
		v.unknown = true
		return e
	}
	if rollback {
		v.rollbackCID = created.ID
	} else {
		v.bCID = created.ID
	}
	// Startup must be one actual command, journalled before the daemon call.
	if e = v.record(kind+"-api-start-intent", map[string]string{"id": created.ID}); e != nil {
		return e
	}
	if _, e = v.engine.call(ctx, http.MethodPost, "/containers/"+created.ID+"/start", nil, 204); e != nil {
		v.unknown = true
		return e
	}
	actual, e := lifecycleAPIInspect(ctx, v.engine, created.ID)
	if e != nil || !lifecycleAPIExpectedRuntime(actual, v.original, image, digestRaw(body), body) {
		v.unknown = true
		return lifecycleError("lifecycle_api_runtime_binding_unproven")
	}
	if e = v.record(kind+"-api-start-result", map[string]string{"id": created.ID, "image": image, "program_sha256": program, "started_at": actual.State.StartedAt}); e != nil {
		v.unknown = true
		return e
	}
	// Runtime/business acceptance stays a separate missing host port. A bound
	// running replacement is deployment evidence only, not an acceptance receipt.
	return nil
}

func (v *lifecycleAPITransition) checkRecoveryAPIStopped(ctx context.Context, r lifecycleRequest) error {
	if v == nil || v.self != v || !v.reopenedRecovery || v.unknown || v.request.ActualRunID != r.ActualRunID {
		return lifecycleError("lifecycle_api_original_stop_unproven")
	}
	if v.bCID != "" {
		body, e := readLifecycleAPIRecord(filepath.Join(v.dir, "b-api-create-body.private.json"))
		if e != nil {
			return e
		}
		current, e := lifecycleAPIInspect(ctx, v.engine, v.bCID)
		if e != nil || !lifecycleAPIStoppedRuntime(current, v.original, r.DeploymentControl.BImageID, digestRaw(body), body) {
			return lifecycleError("lifecycle_api_original_stop_unproven")
		}
		return nil
	}
	if v.removedOriginal {
		_, e := v.engine.call(ctx, http.MethodGet, "/containers/"+v.approved.ID+"/json", nil, 404)
		return e
	}
	current, e := lifecycleAPIInspect(ctx, v.engine, v.approved.ID)
	if e != nil || !lifecycleAPIStaticMatches(current, v.approved, r.DeploymentControl.OriginalRuntimeSpecSHA256) || current.State.Running || current.State.PID != 0 || current.State.ExitCode != 0 || current.State.Paused || current.State.Dead || current.State.Restarting || current.State.OOMKilled {
		return lifecycleError("lifecycle_api_original_stop_unproven")
	}
	return nil
}
func (v *lifecycleAPITransition) stopBForRecovery(ctx context.Context, r lifecycleRequest) error {
	if v == nil || v.self != v || v.unknown || v.request.ActualRunID != r.ActualRunID || v.request.OperationID != r.OperationID {
		return lifecycleError("lifecycle_api_existing_or_unknown")
	}
	if v.bCID == "" {
		return nil
	} // Actual same-process record: no B start was issued.
	body, e := readLifecycleAPIRecord(filepath.Join(v.dir, "b-api-create-body.private.json"))
	if e != nil {
		return e
	}
	actual, e := lifecycleAPIInspect(ctx, v.engine, v.bCID)
	if e != nil || actual.Name != v.original.Name || actual.Image != r.DeploymentControl.BImageID || actual.State.Paused || actual.State.Restarting || actual.State.Dead || actual.State.OOMKilled {
		return lifecycleError("lifecycle_api_runtime_binding_unproven")
	}
	if !actual.State.Running {
		if !lifecycleAPIStoppedRuntime(actual, v.original, r.DeploymentControl.BImageID, digestRaw(body), body) {
			return lifecycleError("lifecycle_api_original_stop_unproven")
		}
		return nil
	}
	if !lifecycleAPIExpectedRuntime(actual, v.original, r.DeploymentControl.BImageID, digestRaw(body), body) {
		return lifecycleError("lifecycle_api_runtime_binding_unproven")
	}
	if e = v.record("b-api-recovery-stop-intent", map[string]string{"id": v.bCID}); e != nil {
		return e
	}
	if _, e = v.engine.call(ctx, http.MethodPost, "/containers/"+v.bCID+"/stop?t=30", nil, 204); e != nil {
		v.unknown = true
		return e
	}
	stopped, e := lifecycleAPIInspect(ctx, v.engine, v.bCID)
	if e != nil || !lifecycleAPIStoppedRuntime(stopped, v.original, r.DeploymentControl.BImageID, digestRaw(body), body) {
		v.unknown = true
		return lifecycleError("lifecycle_api_original_stop_unproven")
	}
	return v.record("b-api-recovery-stop-result", map[string]string{"id": v.bCID, "state": "stopped_gracefully"})
}
